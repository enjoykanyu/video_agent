package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"video_agent/internal/agent"
	"video_agent/internal/memory"
)

// XiaovHandler 小V助手Handler
type XiaovHandler struct {
	llm           *ollama.ChatModel
	intentAgent   *agent.IntentRecognitionAgent
	memoryManager *memory.MemoryManager
	sessionStore  map[string]*SessionContext
}

// SessionContext 会话上下文
type SessionContext struct {
	SessionID    string    `json:"session_id"`
	UserID       string    `json:"user_id"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

// NewXiaovHandler 创建小V助手Handler
func NewXiaovHandler() (*XiaovHandler, error) {
	ctx := context.Background()

	// 初始化Ollama模型
	chatModel, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen2.5:7b",
	})
	if err != nil {
		return nil, err
	}

	// 创建意图识别Agent
	intentAgent := agent.NewIntentRecognitionAgent(chatModel)

	// 创建记忆管理器
	memoryManager := memory.NewMemoryManager(
		memory.NewShortTermMemory(1000, 24*time.Hour),
		memory.NewLongTermMemory(nil, nil, nil),
		memory.NewWorkingMemory(100),
	)

	return &XiaovHandler{
		llm:           chatModel,
		intentAgent:   intentAgent,
		memoryManager: memoryManager,
		sessionStore:  make(map[string]*SessionContext),
	}, nil
}

// ChatRequest 聊天请求
type ChatRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	Message   string `json:"message" binding:"required"`
	SessionID string `json:"session_id,omitempty"`
}

// ChatResponse 聊天响应
type ChatResponse struct {
	Code      int32             `json:"code"`
	Message   string            `json:"message"`
	Reply     string            `json:"reply"`
	SessionID string            `json:"session_id"`
	Intent    string            `json:"intent"`
	Timestamp int64             `json:"timestamp"`
	Metadata  map[string]string `json:"metadata"`
}

// Chat 处理聊天请求
func (h *XiaovHandler) Chat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, ChatResponse{
			Code:    400,
			Message: "请求参数错误: " + err.Error(),
		})
		return
	}

	// 生成或获取会话ID
	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// 获取或创建会话上下文
	ctx := h.getOrCreateSession(sessionID, req.UserID)

	// 记录用户消息到记忆系统
	userMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Content:   req.Message,
		Type:      memory.MemoryTypeUser,
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"user_id": req.UserID,
		},
	}
	h.memoryManager.Store(context.Background(), userMemory)

	// 构建消息列表
	messages := h.buildMessages(ctx, req.Message)

	// 调用大模型生成回复
	startTime := time.Now()
	response, err := h.llm.Generate(context.Background(), messages)
	if err != nil {
		c.JSON(http.StatusOK, ChatResponse{
			Code:      500,
			Message:   "模型调用失败: " + err.Error(),
			SessionID: sessionID,
			Timestamp: time.Now().UnixMilli(),
		})
		return
	}

	latency := time.Since(startTime)

	// 识别用户意图
	intent, _ := h.intentAgent.Recognize(context.Background(), req.Message)

	// 记录助手回复到记忆系统
	assistantMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Content:   response.Content,
		Type:      memory.MemoryTypeAssistant,
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"user_id": req.UserID,
		},
	}
	h.memoryManager.Store(context.Background(), assistantMemory)

	// 更新会话历史
	ctx.LastActiveAt = time.Now()

	c.JSON(http.StatusOK, ChatResponse{
		Code:      0,
		Message:   "success",
		Reply:     response.Content,
		SessionID: sessionID,
		Intent:    string(intent.Type),
		Timestamp: time.Now().UnixMilli(),
		Metadata: map[string]string{
			"latency_ms":    fmt.Sprintf("%d", latency.Milliseconds()),
			"history_count": "0",
			"user_id":       req.UserID,
		},
	})
}

// ChatStream 处理流式聊天请求
func (h *XiaovHandler) ChatStream(c *gin.Context) {
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

	// 设置SSE响应头
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	// 识别意图
	intent, _ := h.intentAgent.Recognize(context.Background(), req.Message)

	// 构建消息列表
	ctx := h.getOrCreateSession(sessionID, req.UserID)
	messages := h.buildMessages(ctx, req.Message)

	// 调用流式生成
	streamReader, err := h.llm.Stream(context.Background(), messages)
	if err != nil {
		c.SSEvent("error", map[string]interface{}{
			"code":    500,
			"message": "模型流式调用失败: " + err.Error(),
		})
		return
	}
	defer streamReader.Close()

	// 收集完整回复
	var fullResponse string

	// 流式输出
	for {
		chunk, err := streamReader.Recv()
		if err == io.EOF {
			c.SSEvent("done", map[string]interface{}{
				"session_id": sessionID,
				"intent":     string(intent.Type),
				"timestamp":  time.Now().UnixMilli(),
			})
			break
		}
		if err != nil {
			c.SSEvent("error", map[string]interface{}{
				"code":    500,
				"message": err.Error(),
			})
			break
		}

		content := chunk.Content
		fullResponse += content

		c.SSEvent("message", map[string]interface{}{
			"type":       "content",
			"content":    content,
			"session_id": sessionID,
			"intent":     string(intent.Type),
		})
		c.Writer.Flush()
	}

	// 存储对话到记忆系统
	h.storeConversation(sessionID, req.UserID, req.Message, fullResponse)
}

// GetSessionHistory 获取会话历史
func (h *XiaovHandler) GetSessionHistory(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    400,
			"message": "session_id 不能为空",
		})
		return
	}

	history, err := h.memoryManager.GetSessionHistory(context.Background(), sessionID, 20)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"message": "获取历史记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "success",
		"data":    history,
		"total":   len(history),
	})
}

// ClearSession 清空会话
func (h *XiaovHandler) ClearSession(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    400,
			"message": "session_id 不能为空",
		})
		return
	}

	err := h.memoryManager.ClearSession(context.Background(), sessionID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    500,
			"message": "清空会话失败: " + err.Error(),
		})
		return
	}

	delete(h.sessionStore, sessionID)

	c.JSON(http.StatusOK, gin.H{
		"code":       0,
		"message":    "会话已清空",
		"cleared":    true,
		"session_id": sessionID,
	})
}

// HealthCheck 健康检查
func (h *XiaovHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code":      0,
		"status":    "healthy",
		"version":   "1.0.0",
		"timestamp": time.Now().UnixMilli(),
		"features": []string{
			"chat",
			"stream_chat",
			"session_history",
			"intent_recognition",
		},
	})
}

// getOrCreateSession 获取或创建会话上下文
func (h *XiaovHandler) getOrCreateSession(sessionID, userID string) *SessionContext {
	if ctx, exists := h.sessionStore[sessionID]; exists {
		return ctx
	}

	ctx := &SessionContext{
		SessionID:    sessionID,
		UserID:       userID,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	h.sessionStore[sessionID] = ctx
	return ctx
}

// buildMessages 构建消息列表
func (h *XiaovHandler) buildMessages(ctx *SessionContext, userMessage string) []*schema.Message {
	var messages []*schema.Message

	// 添加系统提示词
	systemPrompt := schema.SystemMessage("你是小V助手，一个智能AI助手。请根据用户的问题提供有帮助、准确且友好的回答。")
	messages = append(messages, systemPrompt)

	// 添加当前用户消息
	messages = append(messages, schema.UserMessage(userMessage))

	return messages
}

// storeConversation 存储对话到记忆系统
func (h *XiaovHandler) storeConversation(sessionID, userID, userMsg, assistantMsg string) {
	userMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Content:   userMsg,
		Type:      memory.MemoryTypeUser,
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"user_id": userID,
		},
	}
	h.memoryManager.Store(context.Background(), userMemory)

	assistantMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Content:   assistantMsg,
		Type:      memory.MemoryTypeAssistant,
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"user_id": userID,
		},
	}
	h.memoryManager.Store(context.Background(), assistantMemory)
}
