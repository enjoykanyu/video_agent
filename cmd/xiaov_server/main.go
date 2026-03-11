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

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/components/model"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"video_agent/internal/agent"
	"video_agent/mcp"
	pb "video_agent/proto_gen/proto"
)

func main() {
	ctx := context.Background()

	mcpConfig := &mcp.MCPConfig{
		Transport: "sse",
		Server: mcp.ServerConfig{
			URL: "http://localhost:8081/mcp/sse",
		},
	}

	fmt.Println("⏳ 初始化 MCP 工具...")
	if err := mcp.InitMCP(ctx, mcpConfig); err != nil {
		log.Printf("init MCP warning: %v", err)
	}

	fmt.Println("⏳ 初始化 Ollama 大模型...")
	llm, err := getChatModel(ctx)
	if err != nil {
		log.Fatalf("get chat model failed: %v", err)
	}

	mcpServers := []agent.MCPServer{
		{
			UID:    "video-mcp-1",
			Name:   "video-mcp",
			URL:    "http://localhost:8081/mcp/sse",
			Status: 1,
		},
	}

	fmt.Println("⏳ 初始化 Agent...")
	uc, err := agent.NewVideoAssistantUsecase(nil, llm, nil, mcpServers)
	if err != nil {
		log.Fatalf("create usecase failed: %v", err)
	}

	lis, err := net.Listen("tcp", ":50090")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterXiaovServiceServer(grpcServer, NewXiaovGRPCServer(uc))

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()

	log.Println("Server started on :50090")

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")
	grpcServer.GracefulStop()
}

type XiaovGRPCServer struct {
	pb.UnimplementedXiaovServiceServer
	usecase *agent.VideoAssistantUsecase
}

func NewXiaovGRPCServer(uc *agent.VideoAssistantUsecase) *XiaovGRPCServer {
	return &XiaovGRPCServer{
		usecase: uc,
	}
}

func (s *XiaovGRPCServer) Chat(ctx context.Context, req *pb.ChatRequest) (*pb.ChatResponse, error) {
	sessionID := req.SessionId
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	reply, err := s.usecase.Chat(ctx, sessionID, req.UserId, req.Message)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "chat failed: %v", err)
	}

	return &pb.ChatResponse{
		Code:      0,
		Message:   "success",
		Reply:     reply,
		SessionId: sessionID,
		Timestamp: time.Now().UnixMilli(),
	}, nil
}

func (s *XiaovGRPCServer) ChatStream(req *pb.ChatRequest, stream pb.XiaovService_ChatStreamServer) error {
	sessionID := req.SessionId
	if sessionID == "" {
		sessionID = uuid.New().String()
	}

	result, err := s.usecase.StreamChat(stream.Context(), sessionID, req.UserId, req.Message)
	if err != nil {
		return status.Errorf(codes.Internal, "stream chat failed: %v", err)
	}

	return stream.Send(&pb.ChatStreamResponse{
		Payload: &pb.ChatStreamResponse_Content{
			Content: &pb.StreamContent{
				Content: result,
			},
		},
	})
}

func getChatModel(ctx context.Context) (model.ChatModel, error) {
	llm, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3:0.6b",
	})
	if err != nil {
		return nil, fmt.Errorf("create ollama chat model failed: %w", err)
	}
	return llm, nil
}
