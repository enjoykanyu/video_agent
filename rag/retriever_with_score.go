package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// floatVectorConverter 将 float64 向量转换为 FloatVector
func floatVectorConverter(ctx context.Context, vectors [][]float64) ([]entity.Vector, error) {
	vec := make([]entity.Vector, 0, len(vectors))
	for _, vector := range vectors {
		// 转换为 float32
		float32Vec := make([]float32, len(vector))
		for i, v := range vector {
			float32Vec[i] = float32(v)
		}
		vec = append(vec, entity.FloatVector(float32Vec))
	}
	return vec, nil
}

// l2DocumentConverter 处理 L2 距离分数，将距离转换为相似度分数 (0-1)
func l2DocumentConverter(ctx context.Context, result client.SearchResult) ([]*schema.Document, error) {
	documents := make([]*schema.Document, 0, result.IDs.Len())

	// 获取内容字段
	var contentCol *entity.ColumnVarChar
	var metadataCol *entity.ColumnJSONBytes

	for _, field := range result.Fields {
		switch col := field.(type) {
		case *entity.ColumnVarChar:
			if col.Name() == "content" {
				contentCol = col
			}
		case *entity.ColumnJSONBytes:
			if col.Name() == "metadata" {
				metadataCol = col
			}
		}
	}

	// 获取ID列表
	idCol, ok := result.IDs.(*entity.ColumnVarChar)
	if !ok {
		return nil, fmt.Errorf("unsupported id column type")
	}

	for i := 0; i < idCol.Len(); i++ {
		id, err := idCol.GetAsString(i)
		if err != nil {
			return nil, fmt.Errorf("failed to get id: %w", err)
		}

		doc := &schema.Document{
			ID:       id,
			MetaData: make(map[string]any),
		}

		// 获取内容
		if contentCol != nil && i < contentCol.Len() {
			content, err := contentCol.GetAsString(i)
			if err == nil {
				doc.Content = content
			}
		}

		// 计算 L2 相似度分数 (距离越小越相似)
		// 使用公式: similarity = 1 / (1 + distance)
		var similarity float64
		if i < len(result.Scores) {
			l2Distance := float64(result.Scores[i])
			similarity = 1.0 / (1.0 + l2Distance)
		}

		// 获取元数据
		if metadataCol != nil && i < metadataCol.Len() {
			metadataBytes, err := metadataCol.Get(i)
			if err == nil {
				bytes, ok := metadataBytes.([]byte)
				if ok {
					json.Unmarshal(bytes, &doc.MetaData)
				}
			}
		}

		// 设置分数（确保不被 metadata 覆盖）
		doc.MetaData["score"] = similarity
		if i < len(result.Scores) {
			doc.MetaData["l2_distance"] = float64(result.Scores[i])
		}

		documents = append(documents, doc)
	}

	return documents, nil
}

// DocumentWithScore 带相似度分数的文档
type DocumentWithScore struct {
	*schema.Document
	Score float64
}

// 相似度阈值常量 (针对 L2 距离转换的相似度: similarity = 1/(1+L2))
const (
	ScoreVeryRelevant  = 0.45 // 非常相关 (L2距离约1.2)
	ScoreRelevant      = 0.40 // 相关 (L2距离约1.5)
	ScoreMaybeRelevant = 0.35 // 可能相关 (L2距离约1.9)
	ScoreWeakRelevant  = 0.30 // 弱相关 (L2距离约2.3)
)

// GetSimilarityLevel 根据分数返回相似度等级
func GetSimilarityLevel(score float64) string {
	switch {
	case score >= ScoreVeryRelevant:
		return "非常相关"
	case score >= ScoreRelevant:
		return "相关"
	case score >= ScoreMaybeRelevant:
		return "可能相关"
	case score >= ScoreWeakRelevant:
		return "弱相关"
	default:
		return "不相关"
	}
}

// RAGResult RAG 检索结果
type RAGResult struct {
	Query       string
	Documents   []*DocumentWithScore
	TopDocument *DocumentWithScore
	TotalFound  int
	Filtered    int
	HasResult   bool
}

// RetrieverRAGTop1 只返回最匹配的单篇文档
// 使用余弦相似度（Cosine Similarity），范围 [0, 1]
// 参数说明：
//   - query: 用户查询
//   - threshold: 相似度阈值，默认 0.75
//   - topK: 召回数量，默认 3
func RetrieverRAGTop1(query string, threshold float64) *RAGResult {
	ctx := context.Background()

	if err := EnsureMilvusConnected(); err != nil {
		log.Printf("[RAG] Milvus client not available, skipping retrieval")
		return &RAGResult{Query: query, HasResult: false}
	}
	if MilvusCli == nil {
		log.Printf("[RAG] Milvus client not initialized, skipping retrieval")
		return &RAGResult{Query: query, HasResult: false}
	}

	// 初始化自定义嵌入器（确保返回 Float64 向量）
	embedder, err := NewOllamaEmbedder(&OllamaEmbedderConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3-embedding:0.6b",
		Timeout: 10 * time.Second,
	})
	if err != nil {
		log.Printf("[RAG] Failed to create embedder: %v", err)
		return &RAGResult{Query: query, HasResult: false}
	}

	// 创建检索器 - TopK=3，减少无效召回
	retriever, err := milvus.NewRetriever(ctx, &milvus.RetrieverConfig{
		Client:      MilvusCli,
		Collection:  "website_kb",
		Partition:   nil,
		VectorField: "vector",
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		TopK:              3,
		Embedding:         embedder,
		VectorConverter:   floatVectorConverter,
		DocumentConverter: l2DocumentConverter,
		MetricType:        entity.L2,
	})
	if err != nil {
		log.Printf("[RAG] Failed to create retriever: %v", err)
		return &RAGResult{Query: query, HasResult: false}
	}

	results, err := retriever.Retrieve(ctx, query)
	if err != nil {
		log.Printf("[RAG] Retrieval failed: %v", err)
		return &RAGResult{Query: query, HasResult: false}
	}

	// 转换为带分数的结果
	var docsWithScore []*DocumentWithScore
	for _, doc := range results {
		score := 0.0
		if doc.MetaData != nil {
			// 尝试 float64
			if s, ok := doc.MetaData["score"].(float64); ok {
				score = s
			} else if s, ok := doc.MetaData["score"].(float32); ok {
				// 尝试 float32
				score = float64(s)
			}
		}
		docsWithScore = append(docsWithScore, &DocumentWithScore{
			Document: doc,
			Score:    score,
		})
	}

	// 按分数降序排序
	sort.Slice(docsWithScore, func(i, j int) bool {
		return docsWithScore[i].Score > docsWithScore[j].Score
	})

	// 根据阈值过滤
	var filtered []*DocumentWithScore
	for _, d := range docsWithScore {
		if d.Score >= threshold {
			filtered = append(filtered, d)
		}
	}

	// 构建结果
	result := &RAGResult{
		Query:      query,
		Documents:  filtered,
		TotalFound: len(results),
		Filtered:   len(filtered),
		HasResult:  len(filtered) > 0,
	}

	// 取 Top-1 最相关文档
	if len(filtered) > 0 {
		result.TopDocument = filtered[0]
	}

	// 打印检索结果
	log.Printf("[RAG] 查询: %s", query)
	log.Printf("[RAG] 原始召回: %d 条, 阈值过滤后: %d 条 (threshold=%.2f)",
		result.TotalFound, result.Filtered, threshold)
	if result.TopDocument != nil {
		log.Printf("[RAG] Top-1: 分数=%.4f, 等级=%s, 内容=%s",
			result.TopDocument.Score,
			GetSimilarityLevel(result.TopDocument.Score),
			truncateContent(result.TopDocument.Content, 100))
	}

	return result
}

// RetrieverRAGWithScore 带相似度分数的RAG检索（保留兼容）
func RetrieverRAGWithScore(query string, threshold float64) []*DocumentWithScore {
	result := RetrieverRAGTop1(query, threshold)
	return result.Documents
}

// truncateContent 截断内容用于日志显示
func truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen] + "..."
}
