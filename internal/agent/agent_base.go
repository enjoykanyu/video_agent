package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// BaseAgent 所有Agent的基础实现
type BaseAgent struct {
	name         AgentType
	llm          model.ChatModel
	toolExecutor *ToolExecutor
	systemPrompt string
	maxToolRound int
}

func NewBaseAgent(name AgentType, llm model.ChatModel, te *ToolExecutor, systemPrompt string) *BaseAgent {
	return &BaseAgent{
		name:         name,
		llm:          llm,
		toolExecutor: te,
		systemPrompt: systemPrompt,
		maxToolRound: 5,
	}
}

func (b *BaseAgent) Name() AgentType {
	return b.name
}

// ExecuteWithToolLoop 通用的带Tool循环的执行逻辑
func (b *BaseAgent) ExecuteWithToolLoop(ctx context.Context, state *GraphState) (*AgentResult, error) {
	log.Printf("[%s] starting execution", b.name)

	// 构建消息
	messages := state.BuildMessagesForAgent(b.systemPrompt, b.name)

	// 执行带Tool循环的LLM调用
	resp, toolsUsed, err := b.toolExecutor.ExecuteWithTools(ctx, b.llm, messages)
	if err != nil {
		return &AgentResult{
			AgentType: b.name,
			Error:     err.Error(),
		}, err
	}

	result := &AgentResult{
		AgentType: b.name,
		Content:   resp.Content,
		ToolsUsed: toolsUsed,
	}

	log.Printf("[%s] execution done, tools used: %v, content length: %d",
		b.name, toolsUsed, len(resp.Content))

	return result, nil
}

// DefaultRoute 默认路由逻辑：按计划顺序继续
func (b *BaseAgent) DefaultRoute(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	// 先检查是否有动态路由需求
	dynamicNext, err := b.checkDynamicRoute(ctx, state, result)
	if err == nil && dynamicNext != "" {
		log.Printf("[%s] dynamic route to: %s", b.name, dynamicNext)
		return dynamicNext, nil
	}

	// 按计划推进
	state.AdvanceAgent()
	next, hasMore := state.GetNextAgent()
	if !hasMore {
		log.Printf("[%s] no more agents, route to summary", b.name)
		return AgentTypeSummary, nil
	}

	log.Printf("[%s] route to next: %s", b.name, next)
	return next, nil
}

// checkDynamicRoute 检查Agent是否需要动态路由到计划外的Agent
func (b *BaseAgent) checkDynamicRoute(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	// 用LLM判断是否需要动态路由
	routeMessages := []*schema.Message{
		schema.SystemMessage(AgentRoutePrompt),
		schema.UserMessage(fmt.Sprintf("用户原始问题: %s\n\n你的分析结果: %s",
			state.OriginalQuery, result.Content)),
	}

	resp, err := b.toolExecutor.ExecuteWithoutTools(ctx, b.llm, routeMessages)
	if err != nil {
		return "", err
	}

	// 解析路由决策
	jsonStr := extractJSON(resp.Content)
	var route struct {
		Next   string `json:"next"`
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &route); err != nil {
		return "", err
	}

	if route.Next == "continue" || route.Next == "" {
		return "", fmt.Errorf("continue with plan")
	}

	if at := parseAgentType(route.Next); at != "" {
		// 确认不是已经执行过的Agent
		if _, exists := state.GetAgentResult(at); !exists {
			return at, nil
		}
	}

	if strings.ToLower(route.Next) == "summary" {
		return AgentTypeSummary, nil
	}

	return "", fmt.Errorf("continue with plan")
}
