package types

import (
	"context"
	"encoding/json"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// AgentType 定义Agent类型
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

// AllAgentTypes 所有可用的Agent类型
var AllAgentTypes = []AgentType{
	AgentTypeVideo,
	AgentTypeAnalysis,
	AgentTypeCreation,
	AgentTypeReport,
	AgentTypeProfile,
	AgentTypeRecommend,
}

// NodeName 返回Agent对应的节点名称
func (a AgentType) NodeName() string {
	return string(a) + "_agent"
}

// AgentResult Agent执行结果
type AgentResult struct {
	AgentType AgentType         `json:"agent_type"`
	Content   string            `json:"content"`
	ToolsUsed []string          `json:"tools_used,omitempty"`
	NextAgent AgentType         `json:"next_agent,omitempty"`
	Error     string            `json:"error,omitempty"`
	ToolCalls []schema.ToolCall `json:"tool_calls,omitempty"`
}

// AgentConfig Agent配置
type AgentConfig struct {
	LLM          model.ChatModel
	Tools        []tool.BaseTool
	MaxToolRound int
}

// MCPServer MCP服务器配置
type MCPServer struct {
	ID            uint
	UID           string
	Name          string
	URL           string
	RequestHeader string
	Status        int32
	Phone         string
}

// ToServerConfig 解析请求头配置
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

// RAGDocument RAG检索文档
type RAGDocument struct {
	ID       string  `json:"id"`
	Content  string  `json:"content"`
	Score    float64 `json:"score"`
	Metadata map[string]any
}

// RAGDocsRetriever RAG文档检索接口
type RAGDocsRetriever interface {
	RetrieveDocuments(ctx context.Context, query, sessionID string) ([]RAGDocument, error)
}

// VideoAssistantRepo 对话存储接口
type VideoAssistantRepo interface {
	SaveConversation(ctx context.Context, sessionID, userID, question, answer string) error
	GetConversations(ctx context.Context, sessionID string) ([]string, error)
}
