package agent

import (
	"context"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
)

// ==================== Agent类型 ====================

type AgentType string

const (
	AgentTypeSupervisor AgentType = "supervisor"
	AgentTypeVideo      AgentType = "video"
	AgentTypeAnalysis   AgentType = "analysis"
	AgentTypeCreation   AgentType = "creation"
	AgentTypeReport     AgentType = "report"
	AgentTypeProfile    AgentType = "profile"
	AgentTypeRecommend  AgentType = "recommend"
	AgentTypeSummary    AgentType = "summary"
	AgentTypeEnd        AgentType = "end"
)

// AllAgentTypes 所有可调度的Agent类型
var AllAgentTypes = []AgentType{
	AgentTypeVideo,
	AgentTypeAnalysis,
	AgentTypeCreation,
	AgentTypeReport,
	AgentTypeProfile,
	AgentTypeRecommend,
}

// AgentNodeName 返回图节点名
func (a AgentType) NodeName() string {
	return string(a) + "_agent"
}

// ==================== Agent执行结果 ====================

type AgentResult struct {
	AgentType AgentType `json:"agent_type"`
	Content   string    `json:"content"`
	ToolsUsed []string  `json:"tools_used,omitempty"`
	NextAgent AgentType `json:"next_agent,omitempty"` // 路由决策
	Error     string    `json:"error,omitempty"`
}

// ==================== Supervisor决策 ====================

type SupervisorPlan struct {
	TaskAnalysis   string      `json:"task_analysis"`
	SelectedAgents []AgentType `json:"selected_agents"`
	ExecutionOrder []AgentType `json:"execution_order"`
	Reasoning      string      `json:"reasoning"`
}

// ==================== Agent接口 ====================

// Agent 每个Agent的统一接口
type Agent interface {
	// Name 返回Agent名称
	Name() AgentType

	// Execute 执行Agent逻辑，包含完整的tool调用循环
	Execute(ctx context.Context, state *GraphState) (*AgentResult, error)

	// Route 根据执行结果决定下一个节点
	Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error)
}

// ==================== Agent基础配置 ====================

type AgentConfig struct {
	LLM          model.ChatModel
	Tools        []tool.BaseTool
	ToolExecutor *ToolExecutor
	MaxToolRound int // 最大tool调用轮数，防止无限循环
}

// ==================== MCP Server配置 ====================

type MCPServer struct {
	ID            uint
	UID           string
	Name          string
	URL           string
	RequestHeader string
	Status        int32
	Phone         string
}

// ==================== RAG ====================

type RAGDocsRetriever interface {
	RetrieveDocuments(ctx context.Context, query, docType string) ([]RAGDocument, error)
}

type RAGDocument struct {
	Content  string
	Score    float64
	DocType  string
	Metadata map[string]string
}

// ==================== 持久化 ====================

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
