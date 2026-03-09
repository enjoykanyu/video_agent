package handler

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"video_agent/internal/agent"
	"video_agent/mcp_server"
)

type XiaovHandler struct {
	uc *agent.VideoAssistantUsecase
}

type SessionContext struct {
	SessionID    string    `json:"session_id"`
	UserID       string    `json:"user_id"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

func NewXiaovHandler(uc *agent.VideoAssistantUsecase) *XiaovHandler {
	return &XiaovHandler{uc: uc}
}

func (h *XiaovHandler) GetUsecase() *agent.VideoAssistantUsecase {
	return h.uc
}

type ChatRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	Message   string `json:"message" binding:"required"`
	SessionID string `json:"session_id,omitempty"`
}

type ChatResponse struct {
	Code      int32  `json:"code"`
	Message   string `json:"message"`
	Reply     string `json:"reply"`
	SessionID string `json:"session_id"`
	Intent    string `json:"intent"`
	Timestamp int64  `json:"timestamp"`
}

func (h *XiaovHandler) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, ChatResponse{
			Code:    400,
			Message: "请求参数错误: " + err.Error(),
		})
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	reply, err := h.uc.Chat(c.Request.Context(), sessionID, req.UserID, req.Message)
	if err != nil {
		c.JSON(http.StatusOK, ChatResponse{
			Code:      500,
			Message:   "处理失败: " + err.Error(),
			SessionID: sessionID,
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	c.JSON(http.StatusOK, ChatResponse{
		Code:      0,
		Message:   "success",
		Reply:     reply,
		SessionID: sessionID,
		Timestamp: time.Now().UnixMilli(),
	})
}

func (h *XiaovHandler) StreamChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, ChatResponse{
			Code:    400,
			Message: "请求参数错误: " + err.Error(),
		})
		return
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	stream, err := h.uc.StreamChat(c.Request.Context(), sessionID, req.UserID, req.Message)
	if err != nil {
		c.JSON(http.StatusOK, ChatResponse{
			Code:      500,
			Message:   "处理失败: " + err.Error(),
			SessionID: sessionID,
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	for {
		resp, err := stream.Recv()
		if err != nil {
			break
		}
		c.SSEvent("message", resp.Content)
		c.Writer.Flush()
	}
}

func InitHandler(ctx context.Context) (*XiaovHandler, error) {
	var repo agent.VideoAssistantRepo
	var ragRetriever agent.RAGDocsRetriever

	// 1. 启动 MCP Server (在后台运行)
	mcpSrv := mcp_server.NewVideoServer("http://localhost:50090")
	go func() {
		if err := mcpSrv.Start(":9090"); err != nil {
			fmt.Printf("⚠️ [Handler] MCP Server 启动失败: %v\n", err)
		}
	}()
	fmt.Println("✅ [Handler] MCP Server 启动中 :9090")

	// 等待 MCP Server 启动
	time.Sleep(500 * time.Millisecond)

	// 2. 配置 MCP Servers（连接到刚启动的 MCP Server）
	mcpServers := []agent.MCPServer{
		{
			UID:    "video-mcp-1",
			Name:   "video-mcp",
			URL:    "http://localhost:9090/mcp/sse",
			Status: 1,
		},
	}

	llm, err := newLLM(ctx)
	if err != nil {
		return nil, fmt.Errorf("create llm failed: %w", err)
	}

	uc, err := agent.NewVideoAssistantUsecase(repo, llm, ragRetriever)
	if err != nil {
		return nil, fmt.Errorf("create usecase failed: %w", err)
	}

	if err := uc.RefreshMCPTools(ctx, mcpServers); err != nil {
		fmt.Printf("refresh mcp tools failed: %v\n", err)
	}

	return NewXiaovHandler(uc), nil
}

func newLLM(ctx context.Context) (model.ChatModel, error) {
	llm, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3:0.6b",
	})
	if err != nil {
		return nil, fmt.Errorf("create ollama model failed: %w", err)
	}
	return llm, nil
}
