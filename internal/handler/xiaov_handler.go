package handler

import (
	"net/http"
	"time"
	agent_biz "video_agent/internal/agent/biz"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ChatRequest struct {
	Message   string `json:"message" binding:"required"`
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
}

type ChatResponse struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	SessionID string `json:"session_id"`
	Timestamp int64  `json:"timestamp"`
}

type XiaovHandler struct {
	uc *agent_biz.VideoAssistantUsecase
}

func NewXiaovHandler(uc *agent_biz.VideoAssistantUsecase) *XiaovHandler {
	return &XiaovHandler{
		uc: uc,
	}
}

func (h *XiaovHandler) GetUsecase() *agent_biz.VideoAssistantUsecase {
	return h.uc
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

	result, err := h.uc.Chat(c.Request.Context(), sessionID, req.UserID, req.Message)
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
		Code:      200,
		Message:   result,
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

	result, err := h.uc.StreamChat(c.Request.Context(), sessionID, req.UserID, req.Message)
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

	c.SSEvent("message", result)
	c.Writer.Flush()
}
