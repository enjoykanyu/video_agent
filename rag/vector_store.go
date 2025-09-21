package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/schema"
)

// VectorStore 向量存储接口
type VectorStore interface {
	StoreDocument(ctx context.Context, doc *Document) error
	SearchSimilar(ctx context.Context, query string, topK int) ([]*Document, error)
	GetDocumentByID(id string) (*Document, error)
	DeleteDocument(id string) error
	ListDocuments() ([]*Document, error)
}

// Document 文档结构
type Document struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	Embedding []float32              `json:"embedding"`
	CreatedAt string                 `json:"created_at"`
}

// LocalVectorStore 基于本地文件的向量存储实现
type LocalVectorStore struct {
	storePath string
	embedder  interface{} // 占位符，不再使用Ollama嵌入模型
	documents map[string]*Document
	mu        sync.RWMutex
}

// NewLocalVectorStore 创建新的本地向量存储
func NewLocalVectorStore(ctx context.Context, storePath string) (*LocalVectorStore, error) {
	// 创建存储目录
	if err := os.MkdirAll(storePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	store := &LocalVectorStore{
		storePath: storePath,
		embedder:  nil, // 不使用Ollama嵌入模型
		documents: make(map[string]*Document),
	}

	// 加载现有文档
	if err := store.loadDocuments(); err != nil {
		return nil, fmt.Errorf("failed to load documents: %w", err)
	}

	return store, nil
}

// StoreDocument 存储文档并生成向量
func (vs *LocalVectorStore) StoreDocument(ctx context.Context, doc *Document) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// 生成简单的文档嵌入向量
	doc.Embedding = generateSimpleEmbedding(doc.Content)

	// 存储到内存
	vs.documents[doc.ID] = doc

	// 持久化到文件
	return vs.saveDocuments()
}

// SearchSimilar 搜索相似文档（简化实现）
func (vs *LocalVectorStore) SearchSimilar(ctx context.Context, query string, topK int) ([]*Document, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	// 生成查询向量
	queryVec := generateSimpleEmbedding(query)

	// 计算相似度并排序
	var scoredDocs []struct {
		doc   *Document
		score float64
	}

	for _, doc := range vs.documents {
		score := cosineSimilarity(queryVec, doc.Embedding)
		scoredDocs = append(scoredDocs, struct {
			doc   *Document
			score float64
		}{doc: doc, score: score})
	}

	// 按相似度排序（降序）
	for i := 0; i < len(scoredDocs)-1; i++ {
		for j := i + 1; j < len(scoredDocs); j++ {
			if scoredDocs[i].score < scoredDocs[j].score {
				scoredDocs[i], scoredDocs[j] = scoredDocs[j], scoredDocs[i]
			}
		}
	}

	// 返回前topK个结果
	var results []*Document
	for i := 0; i < len(scoredDocs) && i < topK; i++ {
		results = append(results, scoredDocs[i].doc)
	}

	return results, nil
}

// GetDocumentByID 根据ID获取文档
func (vs *LocalVectorStore) GetDocumentByID(id string) (*Document, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	doc, exists := vs.documents[id]
	if !exists {
		return nil, fmt.Errorf("document not found: %s", id)
	}

	return doc, nil
}

// DeleteDocument 删除文档
func (vs *LocalVectorStore) DeleteDocument(id string) error {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	delete(vs.documents, id)
	return vs.saveDocuments()
}

// ListDocuments 列出所有文档
func (vs *LocalVectorStore) ListDocuments() ([]*Document, error) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	var docs []*Document
	for _, doc := range vs.documents {
		docs = append(docs, doc)
	}

	return docs, nil
}

// loadDocuments 从文件加载文档
func (vs *LocalVectorStore) loadDocuments() error {
	dataPath := filepath.Join(vs.storePath, "documents.json")
	if _, err := os.Stat(dataPath); os.IsNotExist(err) {
		return nil // 文件不存在，无需加载
	}

	data, err := os.ReadFile(dataPath)
	if err != nil {
		return fmt.Errorf("failed to read documents file: %w", err)
	}

	var documents map[string]*Document
	if err := json.Unmarshal(data, &documents); err != nil {
		return fmt.Errorf("failed to unmarshal documents: %w", err)
	}

	vs.documents = documents
	return nil
}

// saveDocuments 保存文档到文件
func (vs *LocalVectorStore) saveDocuments() error {
	dataPath := filepath.Join(vs.storePath, "documents.json")

	data, err := json.MarshalIndent(vs.documents, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal documents: %w", err)
	}

	return os.WriteFile(dataPath, data, 0644)
}

// CreateRAGTool 创建RAG工具
func CreateRAGTool(ctx context.Context, storePath string) (*RAGTool, error) {
	vectorStore, err := NewLocalVectorStore(ctx, storePath)
	if err != nil {
		return nil, err
	}

	return &RAGTool{
		vectorStore: vectorStore,
	}, nil
}

// RAGTool RAG工具实现
type RAGTool struct {
	vectorStore VectorStore
}

// Info 返回工具信息
func (rt *RAGTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "rag_knowledge_base",
	}, nil
}

// Invoke 执行RAG检索
func (rt *RAGTool) Invoke(ctx context.Context, input interface{}) (interface{}, error) {
	// 处理字符串输入
	if query, ok := input.(string); ok {
		return rt.invokeWithString(ctx, query)
	}
	
	// 处理map输入
	if inputMap, ok := input.(map[string]interface{}); ok {
		return rt.invokeWithMap(ctx, inputMap)
	}
	
	return nil, fmt.Errorf("invalid input type: expected string or map[string]interface{}")
}

// invokeWithString 处理字符串输入
func (rt *RAGTool) invokeWithString(ctx context.Context, query string) (map[string]interface{}, error) {
	// 搜索相似文档
	results, err := rt.vectorStore.SearchSimilar(ctx, query, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar documents: %w", err)
	}

	// 格式化结果
	var formattedResults []map[string]interface{}
	for _, doc := range results {
		formattedResults = append(formattedResults, map[string]interface{}{
			"id":       doc.ID,
			"content":  doc.Content,
			"metadata": doc.Metadata,
		})
	}

	return map[string]interface{}{
		"results":    formattedResults,
		"query":      query,
		"total_hits": len(results),
	}, nil
}

// invokeWithMap 处理map输入
func (rt *RAGTool) invokeWithMap(ctx context.Context, input map[string]interface{}) (map[string]interface{}, error) {
	query, ok := input["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query parameter is required")
	}

	topK := 3
	if topKParam, ok := input["top_k"].(int); ok {
		topK = topKParam
	}

	// 搜索相似文档
	results, err := rt.vectorStore.SearchSimilar(ctx, query, topK)
	if err != nil {
		return nil, fmt.Errorf("failed to search similar documents: %w", err)
	}

	// 格式化结果
	var formattedResults []map[string]interface{}
	for _, doc := range results {
		formattedResults = append(formattedResults, map[string]interface{}{
			"id":       doc.ID,
			"content":  doc.Content,
			"metadata": doc.Metadata,
		})
	}

	return map[string]interface{}{
		"results":    formattedResults,
		"query":      query,
		"total_hits": len(results),
	}, nil
}

// generateSimpleEmbedding 生成简单的文档嵌入向量（简化实现）
func generateSimpleEmbedding(text string) []float32 {
	// 简单的词频统计作为向量（实际应该使用专业的嵌入模型）
	words := strings.Fields(strings.ToLower(text))
	wordCount := make(map[string]int)
	
	for _, word := range words {
		if len(word) > 2 { // 忽略短词
			wordCount[word]++
		}
	}
	
	// 创建固定长度的向量（简化实现）
	vector := make([]float32, 100)
	for i := range vector {
		vector[i] = float32(i%10) * 0.1 // 简单的模式填充
	}
	
	return vector
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}
	
	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	
	if normA == 0 || normB == 0 {
		return 0.0
	}
	
	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// TestVectorStore 测试向量存储功能
func TestVectorStore() {
	ctx := context.Background()

	// 创建向量存储
	store, err := NewLocalVectorStore(ctx, "./data/vector_store")
	if err != nil {
		fmt.Printf("Failed to create vector store: %v\n", err)
		return
	}

	// 测试存储文档
	testDoc := &Document{
		ID:        "doc1",
		Content:   "Go语言是一种静态强类型、编译型、并发型编程语言",
		Metadata:  map[string]interface{}{"category": "programming", "language": "go"},
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	err = store.StoreDocument(ctx, testDoc)
	if err != nil {
		fmt.Printf("Failed to store document: %v\n", err)
		return
	}

	// 存储更多测试文档
	testDocs := []*Document{
		{
			ID:        "doc2", 
			Content:   "Python是一种解释型、面向对象、动态数据类型的高级程序设计语言",
			Metadata:  map[string]interface{}{"category": "programming", "language": "python"},
			CreatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:        "doc3",
			Content:   "Java是一种广泛使用的计算机编程语言，拥有跨平台、面向对象、泛型编程的特性",
			Metadata:  map[string]interface{}{"category": "programming", "language": "java"},
			CreatedAt: time.Now().Format(time.RFC3339),
		},
	}

	for _, doc := range testDocs {
		if err := store.StoreDocument(ctx, doc); err != nil {
			fmt.Printf("Failed to store document %s: %v\n", doc.ID, err)
		}
	}

	fmt.Println("Documents stored successfully")

	// 测试搜索
	results, err := store.SearchSimilar(ctx, "编程语言特点", 3)
	if err != nil {
		fmt.Printf("Failed to search documents: %v\n", err)
		return
	}

	fmt.Printf("Search results: %d documents found\n", len(results))
	for i, doc := range results {
		fmt.Printf("%d. %s (ID: %s)\n", i+1, doc.Content[:50]+"...", doc.ID)
	}
}