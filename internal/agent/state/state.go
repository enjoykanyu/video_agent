package state

import (
	"fmt"
	"strings"
	"sync"

	types "video_agent/internal/agent/types"

	"github.com/cloudwego/eino/schema"
)

type ExecutionBranch string

const (
	BranchRAG       ExecutionBranch = "rag"
	BranchDirectLLM ExecutionBranch = "direct_llm"
	BranchAgent     ExecutionBranch = "agent"
)

type SupervisorPlan struct {
	TaskAnalysis   string            `json:"task_analysis"`
	SelectedAgents []types.AgentType `json:"selected_agents"`
	ExecutionOrder []types.AgentType `json:"execution_order"`
	Branch         ExecutionBranch   `json:"branch"`
	Reasoning      string            `json:"reasoning"`
}

type GraphState struct {
	mu sync.RWMutex

	OriginalQuery string
	SessionID     string
	UserID        string

	Plan         *SupervisorPlan
	CurrentIndex int
	CurrentAgent types.AgentType

	AgentResults map[types.AgentType]*types.AgentResult

	SelectedTools []string

	Messages []*schema.Message

	RAGDocuments []types.RAGDocument
	RAGContext   string

	// RAGSelection RAG知识库选择结果
	RAGSelection interface{}

	// OptimizedQuery RAG优化后的查询
	OptimizedQuery string

	FinalAnswer string
}

func NewGraphState(query, sessionID, userID string) *GraphState {
	return &GraphState{
		OriginalQuery: query,
		SessionID:     sessionID,
		UserID:        userID,
		AgentResults:  make(map[types.AgentType]*types.AgentResult),
		Messages: []*schema.Message{
			schema.UserMessage(query),
		},
		CurrentAgent: types.AgentTypeSupervisor,
	}
}

func (s *GraphState) SetPlan(plan *SupervisorPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Plan = plan
	s.CurrentIndex = 0
}

func (s *GraphState) GetNextAgent() (types.AgentType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Plan == nil || s.CurrentIndex >= len(s.Plan.ExecutionOrder) {
		return types.AgentTypeSummary, false
	}
	return s.Plan.ExecutionOrder[s.CurrentIndex], true
}

func (s *GraphState) AdvanceAgent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentIndex++
}

func (s *GraphState) SetAgentResult(agentType types.AgentType, result *types.AgentResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AgentResults[agentType] = result
}

func (s *GraphState) GetAgentResult(agentType types.AgentType) (*types.AgentResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.AgentResults[agentType]
	return r, ok
}

func (s *GraphState) SetSelectedTools(tools []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SelectedTools = tools
}

func (s *GraphState) GetSelectedTools() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]string, len(s.SelectedTools))
	copy(cp, s.SelectedTools)
	return cp
}

func (s *GraphState) AppendMessage(msg *schema.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Messages = append(s.Messages, msg)
}

func (s *GraphState) GetMessages() []*schema.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]*schema.Message, len(s.Messages))
	copy(cp, s.Messages)
	return cp
}

func (s *GraphState) SetRAGDocuments(docs []types.RAGDocument) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RAGDocuments = docs

	var sb strings.Builder
	for i, doc := range docs {
		sb.WriteString(fmt.Sprintf("[参考资料%d] (相关度:%.2f) %s\n", i+1, doc.Score, doc.Content))
	}
	s.RAGContext = sb.String()
}

func (s *GraphState) GetRAGContext() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RAGContext
}

func (s *GraphState) BuildAgentContext(targetAgent types.AgentType) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sb strings.Builder

	if s.RAGContext != "" {
		sb.WriteString("## 相关知识库资料\n")
		sb.WriteString(s.RAGContext)
		sb.WriteString("\n")
	}

	if s.Plan != nil {
		for _, agentType := range s.Plan.ExecutionOrder {
			if agentType == targetAgent {
				break
			}
			if result, ok := s.AgentResults[agentType]; ok && result.Content != "" {
				sb.WriteString(fmt.Sprintf("## %s Agent 的分析结果\n", agentType))
				sb.WriteString(result.Content)
				sb.WriteString("\n\n")
			}
		}
	}

	return sb.String()
}

func (s *GraphState) BuildMessagesForAgent(systemPrompt string, targetAgent types.AgentType) []*schema.Message {
	context := s.BuildAgentContext(targetAgent)

	fullSystemPrompt := systemPrompt
	if context != "" {
		fullSystemPrompt += "\n\n## 上下文信息\n" + context
	}

	msgs := []*schema.Message{
		schema.SystemMessage(fullSystemPrompt),
	}

	toolInstruction := "请记住：如果用户的问题需要通过工具获取实时数据或执行特定操作，你必须使用可用的工具函数，不要直接编造回答。"
	if len(s.SelectedTools) > 0 {
		toolInstruction += fmt.Sprintf("\n当前可用的工具: %v", s.SelectedTools)
	}
	msgs = append(msgs, schema.SystemMessage(toolInstruction))

	msgs = append(msgs, schema.UserMessage(s.OriginalQuery))

	return msgs
}

func (s *GraphState) HasAgentResult(agentType types.AgentType) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.AgentResults[agentType]
	return ok
}

func (s *GraphState) GetLastAgentResult() (*types.AgentResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Plan == nil || len(s.Plan.ExecutionOrder) == 0 {
		return nil, false
	}

	for i := len(s.Plan.ExecutionOrder) - 1; i >= 0; i-- {
		if result, ok := s.AgentResults[s.Plan.ExecutionOrder[i]]; ok {
			return result, true
		}
	}
	return nil, false
}

func (s *GraphState) ShouldUseRAG() bool {
	queryLower := strings.ToLower(s.OriginalQuery)
	ragKeywords := []string{"资料", "文档", "知识", "参考", "查找"}
	for _, kw := range ragKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}
	return false
}

func (s *GraphState) ShouldUseDirectLLM() bool {
	queryLower := strings.ToLower(s.OriginalQuery)
	directKeywords := []string{"你好", "在吗", "谢谢", "你好啊", "嘿", "嗨"}
	for _, kw := range directKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}
	return false
}

// SetRAGSelection 设置RAG知识库选择结果
func (s *GraphState) SetRAGSelection(selection interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.RAGSelection = selection
}

// GetRAGSelection 获取RAG知识库选择结果
func (s *GraphState) GetRAGSelection() interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.RAGSelection
}

// SetOptimizedQuery 设置优化后的查询
func (s *GraphState) SetOptimizedQuery(query string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.OptimizedQuery = query
}

// GetOptimizedQuery 获取优化后的查询
func (s *GraphState) GetOptimizedQuery() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.OptimizedQuery
}
