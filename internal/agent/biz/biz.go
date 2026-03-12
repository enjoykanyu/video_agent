package agent_biz

import (
	"context"
	"errors"
	"fmt"
	"log"
	"video_agent/internal/agent/graph"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	ErrGraphNotInitialized = errors.New("graph not initialized")
)

type VideoAssistantUsecase struct {
	repo         types.VideoAssistantRepo
	llm          model.ChatModel
	mcpServers   []types.MCPServer
	graph        *graph.VideoGraph
	ragRetriever types.RAGDocsRetriever
}

func NewVideoAssistantUsecase(
	repo types.VideoAssistantRepo,
	llm model.ChatModel,
	ragRetriever types.RAGDocsRetriever,
	mcpServers []types.MCPServer,
) (*VideoAssistantUsecase, error) {
	if llm == nil {
		return nil, errors.New("llm is required")
	}

	usecase := &VideoAssistantUsecase{
		repo:         repo,
		llm:          llm,
		mcpServers:   mcpServers,
		ragRetriever: ragRetriever,
	}

	if err := usecase.initGraph(); err != nil {
		return nil, fmt.Errorf("init graph: %w", err)
	}

	return usecase, nil
}

func (uc *VideoAssistantUsecase) initGraph() error {
	graph, err := graph.NewVideoGraph(uc.llm, uc.mcpServers)
	if err != nil {
		return fmt.Errorf("create video graph: %w", err)
	}
	uc.graph = graph
	log.Printf("[Usecase] graph initialized successfully")
	return nil
}

func (uc *VideoAssistantUsecase) Chat(ctx context.Context, sessionID, userID, message string) (string, error) {
	if uc.graph == nil {
		return "", ErrGraphNotInitialized
	}

	messages := []*schema.Message{
		schema.UserMessage(message),
	}

	result, err := uc.graph.Run(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("graph chat: %w", err)
	}

	var content string
	if len(result) > 0 {
		content = result[len(result)-1].Content
	}

	if uc.repo != nil {
		if saveErr := uc.repo.SaveConversation(ctx, sessionID, userID, message, content); saveErr != nil {
			log.Printf("[Usecase] save conversation warning: %v", saveErr)
		}
	}

	return content, nil
}

type streamResult struct {
	content string
	done    bool
}

func (s *streamResult) Recv() (string, error) {
	if s.done {
		return "", fmt.Errorf("EOF")
	}
	s.done = true
	return s.content, nil
}

func (uc *VideoAssistantUsecase) StreamChat(ctx context.Context, sessionID, userID, message string) (string, error) {
	return uc.Chat(ctx, sessionID, userID, message)
}

func (uc *VideoAssistantUsecase) RefreshMCPTools(ctx context.Context, mcpServers []types.MCPServer) error {
	uc.mcpServers = mcpServers

	if err := uc.initGraph(); err != nil {
		return fmt.Errorf("reinit graph: %w", err)
	}

	log.Printf("[Usecase] MCP tools refreshed")

	return nil
}

func (uc *VideoAssistantUsecase) Close() {
	log.Printf("[Usecase] resources closed")
}
