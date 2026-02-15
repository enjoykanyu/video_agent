package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"video_agent/agent"
	"video_agent/rag"
)

// RAGServer RAG API服务器
type RAGServer struct {
	ragManager *rag.RAGManager
	router     *gin.Engine
}

// NewRAGServer 创建新的RAG服务器
func NewRAGServer(ragManager *rag.RAGManager) *RAGServer {
	server := &RAGServer{
		ragManager: ragManager,
		router:     gin.Default(),
	}

	server.setupRoutes()
	return server
}

// setupRoutes 设置路由
func (s *RAGServer) setupRoutes() {
	// 健康检查
	s.router.GET("/health", s.healthCheck)

	// RAG相关API
	ragGroup := s.router.Group("/api/rag")
	{
		ragGroup.POST("/search", s.searchDocuments)
		ragGroup.POST("/add", s.addDocument)
		ragGroup.GET("/documents", s.getAllDocuments)
		ragGroup.GET("/documents/:id", s.getDocument)
	}

	// 聊天API
	chatGroup := s.router.Group("/api/chat")
	{
		chatGroup.POST("/rag", s.chatWithRAG)
		chatGroup.POST("/simple", s.simpleChat)
	}
}

// healthCheck 健康检查
func (s *RAGServer) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"service":   "rag-api",
		"timestamp": gin.H{"$date": "now"},
	})
}

// SearchRequest 搜索请求
type SearchRequest struct {
	Query string `json:"query" binding:"required"`
	TopK  int    `json:"top_k"`
}

// SearchResponse 搜索响应
type SearchResponse struct {
	Documents []DocumentResponse `json:"documents"`
	Count     int                `json:"count"`
}

// DocumentResponse 文档响应
type DocumentResponse struct {
	ID        string                 `json:"id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata"`
	Score     float64                `json:"score,omitempty"`
	CreatedAt string                 `json:"created_at"`
}

// searchDocuments 搜索文档
func (s *RAGServer) searchDocuments(c *gin.Context) {
	var req SearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TopK <= 0 {
		req.TopK = 3
	}

	ctx := context.Background()
	documents, err := s.ragManager.SearchSimilarDocuments(req.Query, req.TopK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := SearchResponse{
		Documents: make([]DocumentResponse, len(documents)),
		Count:     len(documents),
	}

	for i, doc := range documents {
		response.Documents[i] = DocumentResponse{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  doc.Metadata,
			CreatedAt: doc.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, response)
}

// AddDocumentRequest 添加文档请求
type AddDocumentRequest struct {
	Content  string                 `json:"content" binding:"required"`
	Metadata map[string]interface{} `json:"metadata"`
}

// AddDocumentResponse 添加文档响应
type AddDocumentResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// addDocument 添加文档
func (s *RAGServer) addDocument(c *gin.Context) {
	var req AddDocumentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := context.Background()
	if err := s.ragManager.AddDocument(req.Content, req.Metadata); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, AddDocumentResponse{
		Message: "文档添加成功",
	})
}

// getAllDocuments 获取所有文档
func (s *RAGServer) getAllDocuments(c *gin.Context) {
	documents := s.ragManager.GetAllDocuments()

	response := SearchResponse{
		Documents: make([]DocumentResponse, len(documents)),
		Count:     len(documents),
	}

	for i, doc := range documents {
		response.Documents[i] = DocumentResponse{
			ID:        doc.ID,
			Content:   doc.Content,
			Metadata:  doc.Metadata,
			CreatedAt: doc.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	c.JSON(http.StatusOK, response)
}

// getDocument 获取单个文档
func (s *RAGServer) getDocument(c *gin.Context) {
	docID := c.Param("id")
	doc, exists := s.ragManager.GetDocument(docID)
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "文档不存在"})
		return
	}

	response := DocumentResponse{
		ID:        doc.ID,
		Content:   doc.Content,
		Metadata:  doc.Metadata,
		CreatedAt: doc.CreatedAt.Format("2006-01-02 15:04:05"),
	}

	c.JSON(http.StatusOK, response)
}

// ChatRequest 聊天请求
type ChatRequest struct {
	Query   string `json:"query" binding:"required"`
	TopK    int    `json:"top_k"`
	Context string `json:"context"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Answer   string `json:"answer"`
	Context  string `json:"context,omitempty"`
	Model    string `json:"model"`
	Duration int64  `json:"duration_ms"`
}

// chatWithRAG 带RAG的聊天
func (s *RAGServer) chatWithRAG(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.TopK <= 0 {
		req.TopK = 3
	}

	// 这里应该调用RAG图代理来处理请求
	// 为了简化，这里返回一个模拟的响应
	ctx := context.Background()
	documents, err := s.ragManager.SearchSimilarDocuments(req.Query, req.TopK)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 构建上下文
	var contextStr string
	for i, doc := range documents {
		contextStr += fmt.Sprintf("文档 %d: %s\n", i+1, doc.Content)
	}

	response := ChatResponse{
		Answer:   fmt.Sprintf("基于检索到的%d个文档，我可以回答你的问题：%s", len(documents), req.Query),
		Context:  contextStr,
		Model:    "qwen3:0.6b",
		Duration: 100, // 模拟处理时间
	}

	c.JSON(http.StatusOK, response)
}

// simpleChat 简单聊天（不带RAG）
func (s *RAGServer) simpleChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response := ChatResponse{
		Answer:   fmt.Sprintf("这是一个简单的回答：%s", req.Query),
		Model:    "qwen3:0.6b",
		Duration: 50,
	}

	c.JSON(http.StatusOK, response)
}

// Start 启动服务器
func (s *RAGServer) Start(addr string) error {
	return s.router.Run(addr)
}
