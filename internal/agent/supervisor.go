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

type SupervisorNode struct {
	llm model.ChatModel
}

func NewSupervisorNode(llm model.ChatModel) *SupervisorNode {
	return &SupervisorNode{llm: llm}
}

// Execute supervisor不需要tool，直接调用LLM做意图识别
func (s *SupervisorNode) Execute(ctx context.Context, state *GraphState) (*SupervisorPlan, error) {
	messages := []*schema.Message{
		schema.SystemMessage(SupervisorPrompt),
		schema.UserMessage(state.OriginalQuery),
	}

	// 如果有RAG上下文，追加参考信息
	if rag := state.GetRAGContext(); rag != "" {
		messages = append(messages,
			schema.SystemMessage("以下是相关的知识库资料，可辅助你做意图判断：\n"+rag))
	}

	resp, err := s.llm.Generate(ctx, messages)
	if err != nil {
		log.Printf("[Supervisor] LLM generate failed: %v, using fallback", err)
		return s.fallbackPlan(state.OriginalQuery), nil
	}

	plan, err := s.parsePlan(resp.Content)
	if err != nil {
		log.Printf("[Supervisor] parse plan failed: %v, content: %s", err, resp.Content)
		return s.fallbackPlan(state.OriginalQuery), nil
	}

	log.Printf("[Supervisor] plan: agents=%v, order=%v, reasoning=%s",
		plan.SelectedAgents, plan.ExecutionOrder, plan.Reasoning)

	return plan, nil
}

func (s *SupervisorNode) parsePlan(content string) (*SupervisorPlan, error) {
	content = extractJSON(content)

	var raw struct {
		TaskAnalysis   string   `json:"task_analysis"`
		SelectedAgents []string `json:"selected_agents"`
		ExecutionOrder []string `json:"execution_order"`
		Reasoning      string   `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w, content: %s", err, content)
	}

	plan := &SupervisorPlan{
		TaskAnalysis: raw.TaskAnalysis,
		Reasoning:    raw.Reasoning,
	}

	for _, a := range raw.SelectedAgents {
		if at := parseAgentType(a); at != "" {
			plan.SelectedAgents = append(plan.SelectedAgents, at)
		}
	}

	for _, a := range raw.ExecutionOrder {
		if at := parseAgentType(a); at != "" {
			plan.ExecutionOrder = append(plan.ExecutionOrder, at)
		}
	}

	// 如果execution_order为空但selected_agents不为空，用selected_agents作为顺序
	if len(plan.ExecutionOrder) == 0 && len(plan.SelectedAgents) > 0 {
		plan.ExecutionOrder = make([]AgentType, len(plan.SelectedAgents))
		copy(plan.ExecutionOrder, plan.SelectedAgents)
	}

	return plan, nil
}

func (s *SupervisorNode) fallbackPlan(message string) *SupervisorPlan {
	lower := strings.ToLower(message)

	var agents []AgentType

	type pattern struct {
		agentType AgentType
		keywords  []string
	}

	patterns := []pattern{
		{AgentTypeVideo, []string{"视频", "播放", "下载", "video", "播放量", "获取视频"}},
		{AgentTypeAnalysis, []string{"分析", "数据", "趋势", "热点", "竞品", "对比"}},
		{AgentTypeCreation, []string{"创作", "写", "文案", "脚本", "标题", "选题", "策划"}},
		{AgentTypeReport, []string{"报表", "周报", "月报", "报告", "汇总"}},
		{AgentTypeProfile, []string{"画像", "用户分析", "粉丝", "偏好", "行为"}},
		{AgentTypeRecommend, []string{"推荐", "类似", "相似", "发现"}},
	}

	for _, p := range patterns {
		for _, kw := range p.keywords {
			if strings.Contains(lower, kw) {
				agents = append(agents, p.agentType)
				break
			}
		}
	}

	// 去重
	agents = uniqueAgents(agents)

	return &SupervisorPlan{
		TaskAnalysis:   "基于关键词匹配的默认决策",
		SelectedAgents: agents,
		ExecutionOrder: agents,
		Reasoning:      "LLM解析失败，使用关键词匹配回退策略",
	}
}

func parseAgentType(s string) AgentType {
	switch strings.TrimSpace(strings.ToLower(s)) {
	case "video":
		return AgentTypeVideo
	case "analysis":
		return AgentTypeAnalysis
	case "creation":
		return AgentTypeCreation
	case "report":
		return AgentTypeReport
	case "profile":
		return AgentTypeProfile
	case "recommend":
		return AgentTypeRecommend
	default:
		return ""
	}
}

func uniqueAgents(agents []AgentType) []AgentType {
	seen := make(map[AgentType]bool)
	var result []AgentType
	for _, a := range agents {
		if !seen[a] {
			seen[a] = true
			result = append(result, a)
		}
	}
	return result
}

func extractJSON(content string) string {
	content = strings.TrimSpace(content)

	// 处理 ```json ... ``` 包裹
	if idx := strings.Index(content, "```json"); idx != -1 {
		content = content[idx+7:]
		if endIdx := strings.Index(content, "```"); endIdx != -1 {
			content = content[:endIdx]
		}
	} else if idx := strings.Index(content, "```"); idx != -1 {
		content = content[idx+3:]
		if endIdx := strings.Index(content, "```"); endIdx != -1 {
			content = content[:endIdx]
		}
	}

	// 尝试找到JSON对象
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start != -1 && end != -1 && end > start {
		content = content[start : end+1]
	}

	return strings.TrimSpace(content)
}
