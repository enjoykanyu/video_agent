package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/eino/compose"
)

// RAGAgent RAG智能体
type RAGAgent struct {
	vectorStore VectorStore
	baseURL     string
	model       string
}

// NewRAGAgent 创建新的RAG智能体
func NewRAGAgent(ctx context.Context, storePath string) (*RAGAgent, error) {
	vectorStore, err := NewLocalVectorStore(ctx, storePath)
	if err != nil {
		return nil, err
	}

	return &RAGAgent{
		vectorStore: vectorStore,
		baseURL:     "http://localhost:11434",
		model:       "qwen3:0.6b",
	}, nil
}

// ProcessQuery 处理RAG查询
func (ra *RAGAgent) ProcessQuery(ctx context.Context, query string) (string, error) {
	// 1. 检索相关文档
	results, err := ra.vectorStore.SearchSimilar(ctx, query, 3)
	if err != nil {
		return "", fmt.Errorf("failed to search documents: %w", err)
	}

	// 2. 构建上下文
	context := buildContextFromResults(results, query)

	// 3. 使用LLM生成回答
	return ra.callOllamaAPI(ctx, context)
}

// buildContextFromResults 从检索结果构建上下文
func buildContextFromResults(results []*Document, query string) string {
	if len(results) == 0 {
		return fmt.Sprintf("用户问题: %s\n\n没有找到相关的上下文信息。", query)
	}

	context := fmt.Sprintf("用户问题: %s\n\n相关上下文信息:\n", query)
	for i, doc := range results {
		context += fmt.Sprintf("[文档%d]: %s\n", i+1, doc.Content)
		if len(doc.Metadata) > 0 {
			context += fmt.Sprintf("  元数据: %v\n", doc.Metadata)
		}
		context += "\n"
	}

	return context
}

// callOllamaAPI 调用Ollama API生成回答
func (ra *RAGAgent) callOllamaAPI(ctx context.Context, prompt string) (string, error) {
	requestBody := map[string]interface{}{
		"model": ra.model,
		"prompt": prompt,
		"stream": false,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "POST", ra.baseURL+"/api/generate", strings.NewReader(string(jsonData)))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Ollama API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Ollama API returned status: %d", resp.StatusCode)
	}

	var response struct {
		Response string `json:"response"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return response.Response, nil
}

// CreateRAGGraph 创建RAG处理Graph
func CreateRAGGraph(ctx context.Context, storePath string) (*compose.Graph[string, string], error) {
	g := compose.NewGraph[string, string]()

	// 创建RAG智能体
	ragAgent, err := NewRAGAgent(ctx, storePath)
	if err != nil {
		return nil, err
	}

	// RAG处理节点
	ragLambda := compose.InvokableLambda(func(ctx context.Context, query string) (output string, err error) {
		return ragAgent.ProcessQuery(ctx, query)
	})

	// 添加节点
	err = g.AddLambdaNode("rag_processing", ragLambda)
	if err != nil {
		return nil, err
	}

	// 连接节点
	err = g.AddEdge(compose.START, "rag_processing")
	if err != nil {
		return nil, err
	}

	err = g.AddEdge("rag_processing", compose.END)
	if err != nil {
		return nil, err
	}

	return g, nil
}

// TestRAGAgent 测试RAG智能体
func TestRAGAgent() {
	ctx := context.Background()

	// 创建RAG智能体
	ragAgent, err := NewRAGAgent(ctx, "./data/rag_store")
	if err != nil {
		fmt.Printf("Failed to create RAG agent: %v\n", err)
		return
	}

	// 测试查询
	query := "Go语言的主要特点是什么？"
	result, err := ragAgent.ProcessQuery(ctx, query)
	if err != nil {
		fmt.Printf("Failed to process query: %v\n", err)
		return
	}

	fmt.Printf("查询: %s\n", query)
	fmt.Printf("回答: %s\n", result)
}

// InitKnowledgeBase 初始化知识库
func InitKnowledgeBase() error {
	ctx := context.Background()
	
	// 创建向量存储目录
	if err := os.MkdirAll("./data/rag_store", 0755); err != nil {
		return fmt.Errorf("failed to create rag store directory: %w", err)
	}

	// 创建向量存储实例
	store, err := NewLocalVectorStore(ctx, "./data/rag_store")
	if err != nil {
		return fmt.Errorf("failed to create vector store: %w", err)
	}

	// 添加一些示例文档
	exampleDocs := []*Document{
		{
			ID:        "doc_go_intro",
			Content:   "Go语言是一种静态强类型、编译型、并发型编程语言，由Google开发。主要特点包括：垃圾回收、内存安全、结构类型和CSP风格的并发编程。",
			Metadata:  map[string]interface{}{"category": "programming", "language": "go", "type": "introduction"},
			CreatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:        "doc_go_concurrency",
			Content:   "Go语言的并发模型基于goroutine和channel。goroutine是轻量级线程，channel用于goroutine之间的通信。这种模型使得并发编程更加简单和安全。",
			Metadata:  map[string]interface{}{"category": "programming", "language": "go", "type": "concurrency"},
			CreatedAt: time.Now().Format(time.RFC3339),
		},
		{
			ID:        "doc_python_intro",
			Content:   "Python是一种解释型、面向对象、动态数据类型的高级程序设计语言。它具有简洁的语法和强大的标准库，广泛应用于Web开发、数据分析、人工智能等领域。",
			Metadata:  map[string]interface{}{"category": "programming", "language": "python", "type": "introduction"},
			CreatedAt: time.Now().Format(time.RFC3339),
		},
	}

	for _, doc := range exampleDocs {
		if err := store.StoreDocument(ctx, doc); err != nil {
			return fmt.Errorf("failed to store document %s: %w", doc.ID, err)
		}
	}

	fmt.Println("✅ 知识库初始化完成，添加了", len(exampleDocs), "个示例文档")
	return nil
}

// StoreDocument 存储文档到知识库
func StoreDocument(ctx context.Context, id string, content string) error {
	store, err := NewLocalVectorStore(ctx, "./data/rag_store")
	if err != nil {
		return fmt.Errorf("failed to create vector store: %w", err)
	}

	doc := &Document{
		ID:        id,
		Content:   content,
		Metadata:  map[string]interface{}{"source": "manual", "timestamp": time.Now().Format(time.RFC3339)},
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	return store.StoreDocument(ctx, doc)
}

// SearchDocuments 搜索文档
func SearchDocuments(ctx context.Context, query string, topK int) ([]*Document, error) {
	store, err := NewLocalVectorStore(ctx, "./data/rag_store")
	if err != nil {
		return nil, fmt.Errorf("failed to create vector store: %w", err)
	}

	return store.SearchSimilar(ctx, query, topK)
}