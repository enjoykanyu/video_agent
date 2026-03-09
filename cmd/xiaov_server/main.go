package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"video_agent/internal/agent"
	"video_agent/internal/handler"
	pb "video_agent/proto_gen/proto"
)

type XiaovGRPCServer struct {
	pb.UnimplementedXiaovServiceServer
	usecase    *agent.VideoAssistantUsecase
	sessionMap map[string]*SessionContext
}

type SessionContext struct {
	SessionID string
	UserID    string
	CreatedAt time.Time
}

func NewXiaovGRPCServer(uc *agent.VideoAssistantUsecase) *XiaovGRPCServer {
	return &XiaovGRPCServer{
		usecase:    uc,
		sessionMap: make(map[string]*SessionContext),
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

	streamResult, err := s.usecase.StreamChat(stream.Context(), sessionID, req.UserId, req.Message)
	if err != nil {
		return status.Errorf(codes.Internal, "stream chat failed: %v", err)
	}

	for {
		resp, err := streamResult.Recv()
		if err != nil {
			break
		}
		if err := stream.Send(&pb.ChatStreamResponse{
			Payload: &pb.ChatStreamResponse_Content{
				Content: &pb.StreamContent{
					Content: resp.Content,
				},
			},
		}); err != nil {
			break
		}
	}

	return nil
}

func main() {
	ctx := context.Background()

	h, err := handler.InitHandler(ctx)
	if err != nil {
		log.Fatalf("init handler failed: %v", err)
	}

	uc := h.GetUsecase()

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
