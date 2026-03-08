package biz

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type MCPServer struct {
	ID            uint
	UID           string
	Name          string
	URL           string
	RequestHeader string
	Status        int32
	Phone         string
}

type RAGDocsRetriever interface {
	RetrieveDocuments(ctx context.Context, query, docType string) ([]RAGDocument, error)
}

type RAGDocument struct {
	Content string
	Score   float64
}

type VideoAssistantRepo interface {
	SaveConversation(ctx context.Context, sessionID, userID, message, reply string) error
	GetConversationHistory(ctx context.Context, sessionID string, limit int) ([]Conversation, error)
}

type Conversation struct {
	SessionID    string
	UserID       string
	UserMsg      string
	AssistantMsg string
	Timestamp    int64
}

type VideoAssistantUsecase struct {
	repo         VideoAssistantRepo
	llm          model.ChatModel
	mcpTools     []tool.BaseTool
	mcpToolsInfo []*schema.ToolInfo
	graph        *VideoGraph
	ragRetriever RAGDocsRetriever
}

func NewVideoAssistantUsecase(
	repo VideoAssistantRepo,
	llm model.ChatModel,
	ragRetriever RAGDocsRetriever,
) (*VideoAssistantUsecase, error) {
	usecase := &VideoAssistantUsecase{
		repo:         repo,
		llm:          llm,
		ragRetriever: ragRetriever,
	}

	usecase.initGraph()

	return usecase, nil
}

func (uc *VideoAssistantUsecase) initGraph() {
	graph, err := NewVideoGraph(uc.llm, uc.mcpTools)
	if err != nil {
		log.Printf("init graph failed: %v", err)
		return
	}
	uc.graph = graph
}

func (uc *VideoAssistantUsecase) Chat(ctx context.Context, sessionID, userID, message string) (string, error) {
	if uc.graph == nil {
		return "", ErrGraphNotInitialized
	}

	if uc.ragRetriever != nil {
		docs, err := uc.ragRetriever.RetrieveDocuments(ctx, message, "")
		if err != nil {
			log.Printf("retrieve documents failed: %v", err)
		} else if len(docs) > 0 {
			message = uc.buildMessageWithContext(message, docs)
		}
	}

	result, err := uc.graph.Chat(ctx, message)
	if err != nil {
		return "", err
	}

	if uc.repo != nil {
		_ = uc.repo.SaveConversation(ctx, sessionID, userID, message, result)
	}

	return result, nil
}

func (uc *VideoAssistantUsecase) StreamChat(ctx context.Context, sessionID, userID, message string) (*schema.StreamReader[*schema.Message], error) {
	if uc.graph == nil {
		return nil, ErrGraphNotInitialized
	}

	if uc.ragRetriever != nil {
		docs, err := uc.ragRetriever.RetrieveDocuments(ctx, message, "")
		if err != nil {
			log.Printf("retrieve documents failed: %v", err)
		} else if len(docs) > 0 {
			message = uc.buildMessageWithContext(message, docs)
		}
	}

	return uc.graph.StreamChat(ctx, message)
}

func (uc *VideoAssistantUsecase) buildMessageWithContext(message string, docs []RAGDocument) string {
	var sb strings.Builder
	sb.WriteString("【相关知识】\n")
	for i, doc := range docs {
		sb.WriteString(fmt.Sprintf("[%d] %s\n", i+1, doc.Content))
	}
	sb.WriteString("\n【用户问题】\n")
	sb.WriteString(message)
	return sb.String()
}

func (uc *VideoAssistantUsecase) RefreshMCPTools(ctx context.Context, mcpServers []MCPServer) error {
	tools, toolsInfo, err := GetHealthyMCPTools(ctx, mcpServers)
	if err != nil {
		return err
	}

	uc.mcpTools = tools
	uc.mcpToolsInfo = toolsInfo

	uc.graph, err = NewVideoGraph(uc.llm, uc.mcpTools)
	if err != nil {
		return err
	}

	return nil
}

var (
	ErrGraphNotInitialized = errors.New("graph not initialized")
)
