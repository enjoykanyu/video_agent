package agent

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var (
	ErrGraphNotInitialized = errors.New("graph not initialized")
)

type VideoAssistantUsecase struct {
	repo         VideoAssistantRepo
	llm          model.ChatModel
	mcpManager   *MCPClientManager
	graph        *VideoGraph
	ragRetriever RAGDocsRetriever
}

func NewVideoAssistantUsecase(
	repo VideoAssistantRepo,
	llm model.ChatModel,
	ragRetriever RAGDocsRetriever,
) (*VideoAssistantUsecase, error) {
	mcpManager := NewMCPClientManager()

	usecase := &VideoAssistantUsecase{
		repo:         repo,
		llm:          llm,
		mcpManager:   mcpManager,
		ragRetriever: ragRetriever,
	}

	if err := usecase.initGraph(); err != nil {
		log.Printf("[Usecase] init graph warning: %v", err)
		// 不返回错误，后续可以通过RefreshMCPTools重试
	}

	return usecase, nil
}

func (uc *VideoAssistantUsecase) initGraph() error {
	graph, err := NewVideoGraph(uc.llm, uc.mcpManager)
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

	// 1. RAG检索
	if uc.ragRetriever != nil {
		docs, err := uc.ragRetriever.RetrieveDocuments(ctx, message, "")
		if err != nil {
			log.Printf("[Usecase] RAG retrieve warning: %v", err)
		} else if len(docs) > 0 {
			log.Printf("[Usecase] RAG retrieved %d documents", len(docs))
			// RAG文档通过state传递，而不是拼接到message中
			// 在graph执行时通过state.RAGDocuments获取
			ctx = context.WithValue(ctx, ragDocsKey{}, docs)
		}
	}

	// 2. 执行图
	result, err := uc.graph.Chat(ctx, message, sessionID, userID)
	if err != nil {
		return "", fmt.Errorf("graph chat: %w", err)
	}

	// 3. 保存对话
	if uc.repo != nil {
		if saveErr := uc.repo.SaveConversation(ctx, sessionID, userID, message, result); saveErr != nil {
			log.Printf("[Usecase] save conversation warning: %v", saveErr)
		}
	}

	return result, nil
}

func (uc *VideoAssistantUsecase) StreamChat(ctx context.Context, sessionID, userID, message string) (*schema.StreamReader[*schema.Message], error) {
	if uc.graph == nil {
		return nil, ErrGraphNotInitialized
	}

	// RAG检索
	if uc.ragRetriever != nil {
		docs, err := uc.ragRetriever.RetrieveDocuments(ctx, message, "")
		if err != nil {
			log.Printf("[Usecase] RAG retrieve warning: %v", err)
		} else if len(docs) > 0 {
			ctx = context.WithValue(ctx, ragDocsKey{}, docs)
		}
	}

	return uc.graph.StreamChat(ctx, message, sessionID, userID)
}

// RefreshMCPTools 刷新MCP工具连接
func (uc *VideoAssistantUsecase) RefreshMCPTools(ctx context.Context, mcpServers []MCPServer) error {
	// 1. 刷新MCP连接
	if err := uc.mcpManager.RefreshConnections(ctx, mcpServers); err != nil {
		return fmt.Errorf("refresh MCP connections: %w", err)
	}

	// 2. 重建图（因为工具变了）
	if err := uc.initGraph(); err != nil {
		return fmt.Errorf("reinit graph: %w", err)
	}

	log.Printf("[Usecase] MCP tools refreshed, tools count: %d",
		len(uc.mcpManager.GetTools()))

	return nil
}

// MCPHealthCheck MCP服务健康检查
func (uc *VideoAssistantUsecase) MCPHealthCheck(ctx context.Context) map[string]bool {
	return uc.mcpManager.HealthCheck(ctx)
}

// Close 释放资源
func (uc *VideoAssistantUsecase) Close() {
	if uc.mcpManager != nil {
		uc.mcpManager.Close()
	}
}

// ragDocsKey context key for RAG documents
type ragDocsKey struct{}

// GetRAGDocsFromContext 从context获取RAG文档
func GetRAGDocsFromContext(ctx context.Context) []RAGDocument {
	docs, ok := ctx.Value(ragDocsKey{}).([]RAGDocument)
	if !ok {
		return nil
	}
	return docs
}
