package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type AgentType string

type Supervisor struct {
	llm      model.ChatModel
	mcpTools []tool.BaseTool
	system   string
}

type SupervisorDecision struct {
	SelectedAgents []AgentType
	Reasoning      string
}

func NewSupervisor(llm model.ChatModel, mcpTools []tool.BaseTool) *Supervisor {
	return &Supervisor{
		llm:      llm,
		mcpTools: mcpTools,
		system:   SupervisorPrompt,
	}
}

const SupervisorPrompt = `# Role: 视频助手系统 - Supervisor（调度者）

## Profile
- language: 中文
- description: 负责分析用户输入，智能调度合适的Agent处理请求
- expertise: 意图识别、任务分解、Agent调度

## Available Agents
- video: 处理视频相关操作（获取视频信息、数据查询等）
- analysis: 处理数据分析（视频分析、趋势追踪、竞品分析等）
- creation: 处理内容创作（文案生成、脚本编写等）
- report: 处理报表生成（周报、月报等）

## Decision Rules
1. 分析用户输入，识别核心意图
2. 选择最合适的Agent处理任务
3. 复杂任务可组合多个Agent
4. 输出结构化决策

## Intent Patterns
- video_query: 视频查询、获取视频信息
- video_analysis: 视频数据分析、趋势分析
- content_creation: 文案创作、脚本编写
- report_generation: 报表生成、周报月报
- knowledge_qa: 知识问答
- general_chat: 通用对话

## OutputFormat
请以JSON格式输出：
{"selected_agents": ["agent1", "agent2"], "reasoning": "决策理由"}
`

func (s *Supervisor) Decide(ctx context.Context, message string) (*SupervisorDecision, error) {
	messages := []*schema.Message{
		schema.SystemMessage(s.system),
		schema.UserMessage(message),
	}

	resp, err := s.llm.Generate(ctx, messages)
	if err != nil {
		return s.fallbackDecision(message), nil
	}

	decision, err := s.parseDecision(resp.Content)
	if err != nil {
		return s.fallbackDecision(message), nil
	}

	return decision, nil
}

func (s *Supervisor) parseDecision(content string) (*SupervisorDecision, error) {
	content = strings.TrimSpace(content)
	content = strings.Trim(content, "```json")
	content = strings.Trim(content, "```")
	content = strings.TrimSpace(content)

	var decision struct {
		SelectedAgents []string `json:"selected_agents"`
		Reasoning      string   `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return nil, fmt.Errorf("parse decision failed: %w", err)
	}

	var agents []AgentType
	for _, a := range decision.SelectedAgents {
		switch a {
		case "video":
			agents = append(agents, AgentTypeVideo)
		case "analysis":
			agents = append(agents, AgentTypeAnalysis)
		case "creation":
			agents = append(agents, AgentTypeCreation)
		case "report":
			agents = append(agents, AgentTypeReport)
		}
	}

	return &SupervisorDecision{
		SelectedAgents: agents,
		Reasoning:      decision.Reasoning,
	}, nil
}

func (s *Supervisor) fallbackDecision(message string) *SupervisorDecision {
	lower := strings.ToLower(message)

	var agents []AgentType

	patterns := map[string][]string{
		string(AgentTypeVideo):    {"视频", "播放", "下载", "获取视频", "video"},
		string(AgentTypeAnalysis): {"分析", "数据", "趋势", "热点", "竞品", "analysis"},
		string(AgentTypeCreation): {"创作", "写", "文案", "脚本", "标题", "creation"},
		string(AgentTypeReport):   {"报表", "周报", "月报", "报告", "report"},
	}

	for agentType, keywords := range patterns {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				agents = append(agents, AgentType(agentType))
				break
			}
		}
	}

	if len(agents) == 0 {
		agents = []AgentType{AgentTypeGeneral}
	}

	return &SupervisorDecision{
		SelectedAgents: agents,
		Reasoning:      "基于关键词检测的默认决策",
	}
}
