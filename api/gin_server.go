package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino/compose"
	"github.com/gin-gonic/gin"

	"video_agent/agent"
)

// GinServer Gin HTTP服务器
type GinServer struct {
	router *gin.Engine
	graph  *compose.Graph[string, interface{}]
}

// NewGinServer 创建新的Gin服务器
func NewGinServer() *GinServer {
	g := compose.NewGraph[string, interface{}]()
	
	// 创建完整工作流
	agent.SetupCompleteWorkflow(g)
	
	// 创建Gin路由
	router := gin.Default()
	
	// 添加中间件
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(CORSMiddleware())
	
	return &GinServer{
		router: router,
		graph:  g,
	}
}

// CORSMiddleware CORS中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		
		c.Next()
	}
}

// RequestPayload API请求结构体
type RequestPayload struct {
	Input     string `json:"input" binding:"required"`
	SessionID string `json:"session_id,omitempty"`
}

// ResponsePayload API响应结构体
type ResponsePayload struct {
	Success   bool        `json:"success"`
	Output    string      `json:"output,omitempty"`
	Error     string      `json:"error,omitempty"`
	SessionID string      `json:"session_id,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	Metadata  interface{} `json:"metadata,omitempty"`
}

// ProcessInput 处理用户输入
func (s *GinServer) ProcessInput(c *gin.Context) {
	var req RequestPayload
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ResponsePayload{
			Success:   false,
			Error:     "无效的请求格式: " + err.Error(),
			Timestamp: time.Now(),
		})
		return
	}
	
	// 执行Graph工作流
	ctx := context.Background()
	compiledGraph, err := s.graph.Compile(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ResponsePayload{
			Success:   false,
			Error:     "编译Graph失败: " + err.Error(),
			Timestamp: time.Now(),
		})
		return
	}
	
	result, err := compiledGraph.Invoke(ctx, req.Input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ResponsePayload{
			Success:   false,
			Error:     "处理失败: " + err.Error(),
			Timestamp: time.Now(),
		})
		return
	}
	
	// 将结果转换为字符串
	var outputStr string
	switch v := result.(type) {
	case string:
		outputStr = v
	default:
		outputStr = fmt.Sprintf("%v", v)
	}
	
	c.JSON(http.StatusOK, ResponsePayload{
		Success:   true,
		Output:    outputStr,
		SessionID: req.SessionID,
		Timestamp: time.Now(),
		Metadata: map[string]interface{}{
			"input_length": len(req.Input),
			"processed_at": time.Now().Format(time.RFC3339),
		},
	})
}

// HealthCheck 健康检查接口
func (s *GinServer) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"version":   "1.0.0",
		"services": []string{
			"intent_agent",
			"tool_dispatcher", 
			"rag_knowledge_base",
			"graph_workflow",
		},
	})
}

// GetGraphInfo 获取Graph信息接口
func (s *GinServer) GetGraphInfo(c *gin.Context) {
	graphInfo := map[string]interface{}{
		"nodes": []string{
			"start",
			"intent_recognition", 
			"mcp_tool",
			"qa_tool",
			"rag_tool",
			"end",
		},
		"edges": []map[string]string{
			{"from": "start", "to": "intent_recognition"},
			{"from": "intent_recognition", "to": "mcp_tool", "condition": "intent=mcp"},
			{"from": "intent_recognition", "to": "qa_tool", "condition": "intent=qa"},
			{"from": "intent_recognition", "to": "rag_tool", "condition": "intent=rag"},
			{"from": "mcp_tool", "to": "end"},
			{"from": "qa_tool", "to": "end"},
			{"from": "rag_tool", "to": "end"},
		},
		"workflow": "用户输入 → 意图识别 → 工具分流 → 结果输出",
	}
	
	c.JSON(http.StatusOK, graphInfo)
}

// SetupRoutes 设置路由
func (s *GinServer) SetupRoutes() {
	// API路由组
	api := s.router.Group("/api/v1")
	{
		api.POST("/process", s.ProcessInput)
		api.GET("/health", s.HealthCheck)
		api.GET("/graph/info", s.GetGraphInfo)
	}
	
	// 文档路由
	s.router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":        "多智能体系统API",
			"version":     "1.0.0",
			"description": "基于Eino框架的多智能体系统架构",
			"endpoints": map[string]string{
				"process":    "POST /api/v1/process - 处理用户输入",
				"health":     "GET /api/v1/health - 健康检查",
				"graph_info": "GET /api/v1/graph/info - 获取Graph信息",
			},
		})
	})
}

// Start 启动服务器
func (s *GinServer) Start(addr string) error {
	fmt.Printf("🚀 多智能体系统API服务器启动中...\n")
	fmt.Printf("📍 监听地址: %s\n", addr)
	fmt.Printf("📊 健康检查: http://%s/api/v1/health\n", addr)
	fmt.Printf("🔧 Graph信息: http://%s/api/v1/graph/info\n", addr)
	fmt.Printf("🎯 处理接口: POST http://%s/api/v1/process\n", addr)
	
	return s.router.Run(addr)
}