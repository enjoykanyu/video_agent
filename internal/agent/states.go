package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/cloudwego/eino/schema"
)

// GraphState 图全局状态，在所有节点间共享
type GraphState struct {
	mu sync.RWMutex

	// ========== 用户输入 ==========
	OriginalQuery string // 原始用户输入
	SessionID     string // 会话ID
	UserID        string // 用户ID

	// ========== 执行计划 ==========
	Plan         *SupervisorPlan // Supervisor生成的执行计划
	CurrentIndex int             // 当前执行到的Agent索引
	CurrentAgent AgentType       // 当前正在执行的Agent

	// ========== Agent结果 ==========
	AgentResults map[AgentType]*AgentResult // 各Agent的执行结果

	// ========== 消息历史 ==========
	Messages []*schema.Message // 完整消息历史

	// ========== RAG知识 ==========
	RAGDocuments []RAGDocument // RAG检索到的文档
	RAGContext   string        // 格式化后的RAG上下文

	// ========== 最终输出 ==========
	FinalAnswer string // 最终回复
}

func NewGraphState(query, sessionID, userID string) *GraphState {
	return &GraphState{
		OriginalQuery: query,
		SessionID:     sessionID,
		UserID:        userID,
		AgentResults:  make(map[AgentType]*AgentResult),
		Messages: []*schema.Message{
			schema.UserMessage(query),
		},
		CurrentAgent: AgentTypeSupervisor,
	}
}

// ========== 线程安全的状态操作 ==========

func (s *GraphState) SetPlan(plan *SupervisorPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Plan = plan
	s.CurrentIndex = 0
}

func (s *GraphState) GetNextAgent() (AgentType, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.Plan == nil || s.CurrentIndex >= len(s.Plan.ExecutionOrder) {
		return AgentTypeSummary, false
	}
	return s.Plan.ExecutionOrder[s.CurrentIndex], true
}

func (s *GraphState) AdvanceAgent() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.CurrentIndex++
}

func (s *GraphState) SetAgentResult(agentType AgentType, result *AgentResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.AgentResults[agentType] = result
}

func (s *GraphState) GetAgentResult(agentType AgentType) (*AgentResult, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.AgentResults[agentType]
	return r, ok
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

func (s *GraphState) SetRAGDocuments(docs []RAGDocument) {
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

// BuildAgentContext 为特定Agent构建上下文信息
// 包含之前Agent的结果摘要和RAG文档
func (s *GraphState) BuildAgentContext(targetAgent AgentType) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var sb strings.Builder

	// 1. RAG知识
	if s.RAGContext != "" {
		sb.WriteString("## 相关知识库资料\n")
		sb.WriteString(s.RAGContext)
		sb.WriteString("\n")
	}

	// 2. 前置Agent结果
	if s.Plan != nil {
		for _, agentType := range s.Plan.ExecutionOrder {
			if agentType == targetAgent {
				break // 只包含当前Agent之前的结果
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

// BuildMessagesForAgent 为特定Agent构建完整的消息列表
func (s *GraphState) BuildMessagesForAgent(systemPrompt string, targetAgent AgentType) []*schema.Message {
	context := s.BuildAgentContext(targetAgent)

	fullSystemPrompt := systemPrompt
	if context != "" {
		fullSystemPrompt += "\n\n## 上下文信息\n" + context
	}

	msgs := []*schema.Message{
		schema.SystemMessage(fullSystemPrompt),
	}

	// 添加原始用户问题
	msgs = append(msgs, schema.UserMessage(s.OriginalQuery))

	return msgs
}
