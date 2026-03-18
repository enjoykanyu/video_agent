package rag

import (
	"context"
	"log"
	"sort"
	"time"

	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/schema"
)

// DocumentWithScore 带相似度分数的文档
type DocumentWithScore struct {
	*schema.Document
	Score float64
}

// 相似度阈值常量
const (
	ScoreVeryRelevant = 0.85  // 非常相关
	ScoreRelevant     = 0.75  // 相关
	ScoreMaybeRelevant = 0.65 // 可能相关
	ScoreWeakRelevant = 0.50  // 弱相关
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

	// 检查 Milvus 客户端是否可用
	if MilvusCli == nil {
		log.Printf("[RAG] Milvus client not initialized, skipping retrieval")
		return &RAGResult{Query: query, HasResult: false}
	}

	// 初始化嵌入器
	embedder, err := ollama.NewEmbedder(ctx, &ollama.EmbeddingConfig{
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
		Collection:  "test_index",
		Partition:   nil,
		VectorField: "vector",
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		TopK:      3,
		Embedding: embedder,
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
			if s, ok := doc.MetaData["score"].(float64); ok {
				score = s
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
