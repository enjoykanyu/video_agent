package rag

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Document struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	Embedding []float64              `json:"embedding"`
	CreatedAt time.Time              `json:"created_at"`
}

type RAGManager struct {
	documents    map[string]*Document
	vectorStore  string
	ragStore     string
	embeddingDim int
}

func NewRAGManager(vectorStorePath, ragStorePath string) (*RAGManager, error) {
	rm := &RAGManager{
		documents:    make(map[string]*Document),
		vectorStore:  vectorStorePath,
		ragStore:     ragStorePath,
		embeddingDim: 100, // 默认维度
	}

	// 加载现有文档
	if err := rm.loadDocuments(); err != nil {
		return nil, fmt.Errorf("failed to load documents: %w", err)
	}

	return rm, nil
}

func (rm *RAGManager) loadDocuments() error {
	// 加载向量存储
	if err := rm.loadFromStore(rm.vectorStore); err != nil {
		return err
	}

	// 加载RAG存储
	if err := rm.loadFromStore(rm.ragStore); err != nil {
		return err
	}

	return nil
}

func (rm *RAGManager) loadFromStore(storePath string) error {
	data, err := os.ReadFile(storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 文件不存在是正常的
		}
		return fmt.Errorf("failed to read store file: %w", err)
	}

	var storeData map[string]*Document
	if err := json.Unmarshal(data, &storeData); err != nil {
		return fmt.Errorf("failed to unmarshal store data: %w", err)
	}

	for id, doc := range storeData {
		rm.documents[id] = doc
	}

	return nil
}

func (rm *RAGManager) saveDocuments() error {
	// 保存到向量存储
	if err := rm.saveToStore(rm.vectorStore); err != nil {
		return err
	}

	return nil
}

func (rm *RAGManager) saveToStore(storePath string) error {
	// 确保目录存在
	dir := filepath.Dir(storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	data, err := json.MarshalIndent(rm.documents, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal documents: %w", err)
	}

	if err := os.WriteFile(storePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write store file: %w", err)
	}

	return nil
}

func (rm *RAGManager) AddDocument(content string, metadata map[string]interface{}) error {
	docID := fmt.Sprintf("doc-%d", time.Now().UnixNano())

	// 生成简单的嵌入向量（实际项目中应该使用真实的嵌入模型）
	embedding := rm.generateSimpleEmbedding(content)

	doc := &Document{
		ID:        docID,
		Content:   content,
		Metadata:  metadata,
		Embedding: embedding,
		CreatedAt: time.Now(),
	}

	rm.documents[docID] = doc

	return rm.saveDocuments()
}

func (rm *RAGManager) generateSimpleEmbedding(text string) []float64 {
	// 这是一个简化的嵌入生成函数
	// 实际项目中应该使用真实的嵌入模型，如OpenAI、Ollama等
	embedding := make([]float64, rm.embeddingDim)

	// 基于文本内容生成简单的特征向量
	words := strings.Fields(strings.ToLower(text))
	for i, word := range words {
		if i < rm.embeddingDim {
			// 简单的哈希函数来生成嵌入值
			hash := 0
			for _, char := range word {
				hash = (hash*31 + int(char)) % 100
			}
			embedding[i] = float64(hash) / 100.0
		}
	}

	return embedding
}

func (rm *RAGManager) SearchSimilarDocuments(query string, topK int) ([]*Document, error) {
	if len(rm.documents) == 0 {
		return []*Document{}, nil
	}

	// 生成查询嵌入
	queryEmbedding := rm.generateSimpleEmbedding(query)

	// 计算相似度并排序
	type docScore struct {
		doc   *Document
		score float64
	}

	var scores []docScore
	for _, doc := range rm.documents {
		score := rm.cosineSimilarity(queryEmbedding, doc.Embedding)
		scores = append(scores, docScore{doc: doc, score: score})
	}

	// 按分数降序排序
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].score > scores[j].score
	})

	// 返回前K个结果
	if topK > len(scores) {
		topK = len(scores)
	}

	var results []*Document
	for i := 0; i < topK; i++ {
		results = append(results, scores[i].doc)
	}

	return results, nil
}

func (rm *RAGManager) cosineSimilarity(vec1, vec2 []float64) float64 {
	if len(vec1) != len(vec2) {
		return 0.0
	}

	var dotProduct, norm1, norm2 float64
	for i := 0; i < len(vec1); i++ {
		dotProduct += vec1[i] * vec2[i]
		norm1 += vec1[i] * vec1[i]
		norm2 += vec2[i] * vec2[i]
	}

	if norm1 == 0 || norm2 == 0 {
		return 0.0
	}

	return dotProduct / (math.Sqrt(norm1) * math.Sqrt(norm2))
}

func (rm *RAGManager) GetDocument(id string) (*Document, bool) {
	doc, exists := rm.documents[id]
	return doc, exists
}

func (rm *RAGManager) GetAllDocuments() []*Document {
	var docs []*Document
	for _, doc := range rm.documents {
		docs = append(docs, doc)
	}
	return docs
}
