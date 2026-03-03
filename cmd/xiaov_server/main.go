package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	rag "video_agent/rag"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"video_agent/internal/agent"
	"video_agent/internal/mcp"
	"video_agent/internal/memory"
	"video_agent/internal/orchestrator"
	"video_agent/mcp_client"
	pb "video_agent/proto_gen/proto"
)

// XiaovGRPCServer 小V助手gRPC服务器
type XiaovGRPCServer struct {
	pb.UnimplementedXiaovServiceServer
	xiaovGraph    *orchestrator.XiaovGraph
	intentAgent   *agent.IntentRecognitionAgent
	memoryManager *memory.MemoryManager
	sessionStore  map[string]*SessionContext
	ragManager    *rag.RAGManager
}

// SessionContext 会话上下文
type SessionContext struct {
	SessionID    string    `json:"session_id"`
	UserID       string    `json:"user_id"`
	CreatedAt    time.Time `json:"created_at"`
	LastActiveAt time.Time `json:"last_active_at"`
}

func main() {
	// 创建gRPC服务器
	server, err := NewXiaovGRPCServer()
	if err != nil {
		log.Fatalf("❌ 初始化服务器失败: %v", err)
	}

	// 启动gRPC服务器
	grpcServer := grpc.NewServer()
	pb.RegisterXiaovServiceServer(grpcServer, server)

	// 监听端口
	addr := ":50090"
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("❌ 监听端口失败: %v", err)
	}

	// 非阻塞启动服务器
	go func() {
		fmt.Printf("🚀 gRPC服务器启动成功！\n")
		fmt.Printf("📍 监听地址: localhost%s\n", addr)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("❌ gRPC服务器运行失败: %v", err)
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	fmt.Println("\n🛑 正在关闭服务器...")
	grpcServer.GracefulStop()
	fmt.Println("✅ 服务器已安全退出")
}

// NewXiaovGRPCServer 创建小V助手gRPC服务器
func NewXiaovGRPCServer() (*XiaovGRPCServer, error) {
	ctx := context.Background()

	// 初始化Ollama模型
	// 可用模型: qwen3:0.6b(最快), deepseek-r1:1.5b(不支持工具), deepseek-r1:8b, llama2-chinese, llama3(支持工具)
	// ReAct Agent 本地无法支持
	chatModel, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3:0.6b",    // 使用0.6b模型，速度最快
		Timeout: 6 * time.Minute, // 设置6分钟超时，给模型足够时间处理
	})
	if err != nil {
		return nil, fmt.Errorf("初始化Ollama模型失败: %w", err)
	}

	// 创建意图识别Agent
	intentAgent := agent.NewIntentRecognitionAgent(chatModel)

	// 创建记忆管理器
	memoryManager := memory.NewMemoryManager(
		memory.NewShortTermMemory(1000, 24*time.Hour),
		memory.NewLongTermMemory(nil, nil, nil),
		memory.NewWorkingMemory(100),
	)
	// 初始化 RAG
	ragManager, err := rag.NewRAGManager(
		"./data/vector_store/documents.json",
		"./data/rag_store/documents.json",
	)
	if err != nil {
		log.Printf("⚠️ RAG 初始化失败: %v", err)
		ragManager = nil
	}
	// 创建图编排器（MCP模式）
	// 配置MCP Server连接
	// 注意：MCP Server需要单独启动，建议监听 :8081（避免与Gateway :8080冲突）
	// MCP Server SSE端点路径是 /mcp/sse
	mcpConfig := &mcp.ManagerConfig{
		RemoteConfig: &mcp_client.Config{
			Transport: "sse", // 使用SSE传输
			Server: mcp_client.ServerConfig{
				URL: "http://localhost:8081/mcp/sse", // MCP Server SSE端点（端口8081）
			},
		},
	}

	xiaovGraph, err := orchestrator.NewXiaovGraph(chatModel, intentAgent, memoryManager, mcpConfig, ragManager)
	if err != nil {
		return nil, fmt.Errorf("初始化图编排器失败: %w", err)
	}

	return &XiaovGRPCServer{
		xiaovGraph:    xiaovGraph,
		intentAgent:   intentAgent,
		memoryManager: memoryManager,
		sessionStore:  make(map[string]*SessionContext),
		ragManager:    ragManager,
	}, nil
}

// Chat 实现普通对话接口（使用图编排）
func (s *XiaovGRPCServer) Chat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
	// 参数校验
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id不能为空")
	}
	if req.Message == "" {
		return nil, status.Error(codes.InvalidArgument, "message不能为空")
	}

	// 生成或获取会话ID
	sessionID := req.SessionId
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// 记录会话上下文
	s.getOrCreateSession(sessionID, req.UserId)

	// 构建图编排输入
	input := orchestrator.XiaovInput{
		SessionID: sessionID,
		UserID:    req.UserId,
		Message:   req.Message,
	}

	// 创建新的上下文，设置合理的超时时间（3分钟）
	// ReAct Agent 需要调用工具和生成回复，给足够时间但不要无限等待
	execCtx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// 执行图编排
	startTime := time.Now()
	output, err := s.xiaovGraph.Execute(execCtx, input)
	if err != nil {
		return nil, status.Error(codes.Internal, "图编排执行失败: "+err.Error())
	}

	latency := time.Since(startTime)

	return &pb.ChatResponse{
		Code:      0,
		Message:   "success",
		Reply:     output.Reply,
		SessionId: output.SessionID,
		Intent:    output.Intent,
		Timestamp: time.Now().UnixMilli(),
		Metadata: map[string]string{
			"latency_ms": fmt.Sprintf("%d", latency.Milliseconds()),
			"agent":      output.Agent,
			"user_id":    req.UserId,
		},
	}, nil
}

// ChatStream 实现流式对话接口
func (s *XiaovGRPCServer) ChatStream(req *pb.ChatRequest, stream pb.XiaovService_ChatStreamServer) error {
	// 参数校验
	if req.UserId == "" {
		return status.Error(codes.InvalidArgument, "user_id不能为空")
	}
	if req.Message == "" {
		return status.Error(codes.InvalidArgument, "message不能为空")
	}

	sessionID := req.SessionId
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	// 记录会话上下文
	s.getOrCreateSession(sessionID, req.UserId)

	// 对于流式响应，先执行意图识别
	intent, err := s.intentAgent.Recognize(stream.Context(), req.Message)
	if err != nil {
		intent = &agent.Intent{Type: agent.IntentGeneralChat, Confidence: 1.0}
	}

	// 发送开始消息
	contentMsg := &pb.ChatStreamResponse{
		Payload: &pb.ChatStreamResponse_Content{
			Content: &pb.StreamContent{
				Content:   "收到您的消息：" + req.Message + "\n意图识别：" + string(intent.Type) + "\n正在处理...",
				SessionId: sessionID,
				Intent:    string(intent.Type),
			},
		},
	}
	if err := stream.Send(contentMsg); err != nil {
		return err
	}

	// 构建输入
	input := orchestrator.XiaovInput{
		SessionID: sessionID,
		UserID:    req.UserId,
		Message:   req.Message,
	}

	// 所有意图都使用同步处理（避免流式超时问题）
	return s.handleStreamGeneralChat(stream, input, sessionID)
}

// handleStreamVideoAnalysis 处理视频分析（使用同步调用避免流式超时问题）
func (s *XiaovGRPCServer) handleStreamVideoAnalysis(stream pb.XiaovService_ChatStreamServer, input orchestrator.XiaovInput, sessionID string) error {
	log.Printf("📡 [视频分析] 开始同步分析，SessionID: %s", sessionID)

	// 发送开始处理的状态消息
	startMsg := &pb.ChatStreamResponse{
		Payload: &pb.ChatStreamResponse_Content{
			Content: &pb.StreamContent{
				Content:   "正在分析视频，请稍候...",
				SessionId: sessionID,
				Intent:    "video_analysis",
			},
		},
	}
	if err := stream.Send(startMsg); err != nil {
		log.Printf("❌ [视频分析] 发送开始消息失败: %v", err)
		return err
	}

	// 使用同步分析方法（避免流式处理的复杂问题）
	output, err := s.xiaovGraph.Execute(stream.Context(), input)
	if err != nil {
		log.Printf("❌ [视频分析] 分析失败: %v", err)
		errorMsg := &pb.ChatStreamResponse{
			Payload: &pb.ChatStreamResponse_Error{
				Error: &pb.StreamError{
					Code:      500,
					Message:   err.Error(),
					SessionId: sessionID,
				},
			},
		}
		return stream.Send(errorMsg)
	}

	log.Printf("✅ [视频分析] 分析完成，回复长度: %d", len(output.Reply))

	// 发送分析结果
	contentMsg := &pb.ChatStreamResponse{
		Payload: &pb.ChatStreamResponse_Content{
			Content: &pb.StreamContent{
				Content:   output.Reply,
				SessionId: sessionID,
				Intent:    "video_analysis",
			},
		},
	}
	if err := stream.Send(contentMsg); err != nil {
		log.Printf("❌ [视频分析] 发送结果失败: %v", err)
		return err
	}

	// 存储到记忆
	assistantMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: sessionID,
		Content:   output.Reply,
		Type:      memory.MemoryTypeAssistant,
		CreatedAt: time.Now(),
	}
	s.memoryManager.Store(stream.Context(), assistantMemory)

	// 发送完成标记
	doneMsg := &pb.ChatStreamResponse{
		Payload: &pb.ChatStreamResponse_Done{
			Done: &pb.StreamDone{
				SessionId: sessionID,
				Intent:    "video_analysis",
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}
	log.Printf("✅ [视频分析] 处理完成")
	return stream.Send(doneMsg)
}

// handleStreamGeneralChat 流式处理通用对话
func (s *XiaovGRPCServer) handleStreamGeneralChat(stream pb.XiaovService_ChatStreamServer, input orchestrator.XiaovInput, sessionID string) error {
	// 执行图编排获取完整回复
	output, err := s.xiaovGraph.Execute(stream.Context(), input)
	if err != nil {
		errorMsg := &pb.ChatStreamResponse{
			Payload: &pb.ChatStreamResponse_Error{
				Error: &pb.StreamError{
					Code:      500,
					Message:   err.Error(),
					SessionId: sessionID,
				},
			},
		}
		return stream.Send(errorMsg)
	}

	// 发送实际回复内容
	contentMsg := &pb.ChatStreamResponse{
		Payload: &pb.ChatStreamResponse_Content{
			Content: &pb.StreamContent{
				Content:   output.Reply,
				SessionId: sessionID,
				Intent:    output.Intent,
			},
		},
	}
	if err := stream.Send(contentMsg); err != nil {
		return err
	}

	// 发送完成标记
	doneMsg := &pb.ChatStreamResponse{
		Payload: &pb.ChatStreamResponse_Done{
			Done: &pb.StreamDone{
				SessionId: sessionID,
				Intent:    output.Intent,
				Timestamp: time.Now().UnixMilli(),
			},
		},
	}
	return stream.Send(doneMsg)
}

// GetSessionHistory 实现获取会话历史接口
func (s *XiaovGRPCServer) GetSessionHistory(ctx context.Context, req *pb.GetSessionHistoryRequest) (*pb.GetSessionHistoryResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id不能为空")
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	history, err := s.memoryManager.GetSessionHistory(ctx, req.SessionId, int(limit))
	if err != nil {
		return nil, status.Error(codes.Internal, "获取历史记录失败: "+err.Error())
	}

	// 转换为protobuf消息
	var messages []*pb.ChatMessage
	for _, mem := range history {
		role := "user"
		if mem.Type == memory.MemoryTypeAssistant {
			role = "assistant"
		}

		userID := ""
		if mem.Metadata != nil {
			if uid, ok := mem.Metadata["user_id"].(string); ok {
				userID = uid
			}
		}

		messages = append(messages, &pb.ChatMessage{
			Id:        mem.ID,
			SessionId: mem.SessionID,
			UserId:    userID,
			Role:      role,
			Content:   mem.Content,
			Timestamp: mem.CreatedAt.UnixMilli(),
		})
	}

	return &pb.GetSessionHistoryResponse{
		Code:     0,
		Message:  "success",
		Messages: messages,
		Total:    int32(len(messages)),
	}, nil
}

// ClearSession 实现清空会话接口
func (s *XiaovGRPCServer) ClearSession(ctx context.Context, req *pb.ClearSessionRequest) (*pb.ClearSessionResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id不能为空")
	}

	err := s.memoryManager.ClearSession(ctx, req.SessionId)
	if err != nil {
		return nil, status.Error(codes.Internal, "清空会话失败: "+err.Error())
	}

	// 清空本地会话缓存
	delete(s.sessionStore, req.SessionId)

	return &pb.ClearSessionResponse{
		Code:    0,
		Message: "会话已清空",
		Cleared: true,
	}, nil
}

// HealthCheck 实现健康检查接口
func (s *XiaovGRPCServer) HealthCheck(ctx context.Context, req *pb.HealthCheckRequest) (*pb.HealthCheckResponse, error) {
	return &pb.HealthCheckResponse{
		Code:      0,
		Status:    "healthy",
		Version:   "1.0.0",
		Timestamp: time.Now().UnixMilli(),
		Features: []string{
			"chat",
			"stream_chat",
			"session_history",
			"intent_recognition",
			"graph_orchestration",
			"branch_routing",
		},
	}, nil
}

// getOrCreateSession 获取或创建会话上下文
func (s *XiaovGRPCServer) getOrCreateSession(sessionID, userID string) *SessionContext {
	if ctx, exists := s.sessionStore[sessionID]; exists {
		return ctx
	}

	ctx := &SessionContext{
		SessionID:    sessionID,
		UserID:       userID,
		CreatedAt:    time.Now(),
		LastActiveAt: time.Now(),
	}
	s.sessionStore[sessionID] = ctx
	return ctx
}
