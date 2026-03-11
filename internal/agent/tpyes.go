package agent

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type AgentType string

const (
	AgentTypeSupervisor   AgentType = "supervisor"
	AgentTypeToolSelect   AgentType = "tool_select"
	AgentTypeToolExecutor AgentType = "tool_executor"
	AgentTypeVideo        AgentType = "video"
	AgentTypeAnalysis     AgentType = "analysis"
	AgentTypeCreation     AgentType = "creation"
	AgentTypeReport       AgentType = "report"
	AgentTypeProfile      AgentType = "profile"
	AgentTypeRecommend    AgentType = "recommend"
	AgentTypeSummary      AgentType = "summary"
	AgentTypeEnd          AgentType = "end"
)

var AllAgentTypes = []AgentType{
	AgentTypeVideo,
	AgentTypeAnalysis,
	AgentTypeCreation,
	AgentTypeReport,
	AgentTypeProfile,
	AgentTypeRecommend,
}

func (a AgentType) NodeName() string {
	return string(a) + "_agent"
}

type AgentResult struct {
	AgentType AgentType         `json:"agent_type"`
	Content   string            `json:"content"`
	ToolsUsed []string          `json:"tools_used,omitempty"`
	NextAgent AgentType         `json:"next_agent,omitempty"`
	Error     string            `json:"error,omitempty"`
	ToolCalls []schema.ToolCall `json:"tool_calls,omitempty"`
}

type Agent interface {
	Name() AgentType
	Execute(ctx context.Context, state *GraphState) (*AgentResult, error)
	Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error)
}

type AgentConfig struct {
	LLM          model.ChatModel
	Tools        []tool.BaseTool
	MaxToolRound int
}

type MCPServer struct {
	ID            uint
	UID           string
	Name          string
	URL           string
	RequestHeader string
	Status        int32
	Phone         string
}

func (s MCPServer) ToServerConfig() (map[string]string, error) {
	if s.RequestHeader == "" {
		return nil, nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(s.RequestHeader), &headers); err != nil {
		return nil, err
	}
	return headers, nil
}

type RAGDocument struct {
	ID       string  `json:"id"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
	Metadata map[string]any
}

type RAGDocsRetriever interface {
	RetrieveDocuments(ctx context.Context, query, sessionID string) ([]RAGDocument, error)
}

type VideoAssistantRepo interface {
	SaveConversation(ctx context.Context, sessionID, userID, question, answer string) error
	GetConversations(ctx context.Context, sessionID string) ([]string, error)
}
