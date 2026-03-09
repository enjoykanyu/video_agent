package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type SummaryNode struct {
	llm model.ChatModel
}

func NewSummaryNode(llm model.ChatModel) *SummaryNode {
	return &SummaryNode{llm: llm}
}

func (s *SummaryNode) Execute(ctx context.Context, state *GraphState) (string, error) {
	log.Printf("[Summary] starting, agent results count: %d", len(state.AgentResults))

	// 如果没有Agent执行过（通用对话），直接用LLM回答
	if len(state.AgentResults) == 0 {
		return s.directAnswer(ctx, state)
	}

	// 如果只有一个Agent的结果，且没有错误，直接返回
	if len(state.AgentResults) == 1 {
		for _, result := range state.AgentResults {
			if result.Error == "" {
				return result.Content, nil
			}
		}
	}

	// 多Agent结果整合
	return s.integrateResults(ctx, state)
}

func (s *SummaryNode) directAnswer(ctx context.Context, state *GraphState) (string, error) {
	messages := []*schema.Message{
		schema.SystemMessage("你是一个专业的视频助手。请直接回答用户的问题。"),
	}

	if rag := state.GetRAGContext(); rag != "" {
		messages = append(messages,
			schema.SystemMessage("参考知识：\n"+rag))
	}

	messages = append(messages, schema.UserMessage(state.OriginalQuery))

	resp, err := s.llm.Generate(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("direct answer failed: %w", err)
	}

	return resp.Content, nil
}

func (s *SummaryNode) integrateResults(ctx context.Context, state *GraphState) (string, error) {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("用户原始问题: %s\n\n", state.OriginalQuery))
	sb.WriteString("以下是各Agent的处理结果:\n\n")

	if state.Plan != nil {
		for _, agentType := range state.Plan.ExecutionOrder {
			if result, ok := state.AgentResults[agentType]; ok {
				sb.WriteString(fmt.Sprintf("### %s Agent 结果\n", agentType))
				if result.Error != "" {
					sb.WriteString(fmt.Sprintf("执行出错: %s\n", result.Error))
				} else {
					sb.WriteString(result.Content)
					if len(result.ToolsUsed) > 0 {
						sb.WriteString(fmt.Sprintf("\n(使用的工具: %s)", strings.Join(result.ToolsUsed, ", ")))
					}
				}
				sb.WriteString("\n\n")
			}
		}
	} else {
		for agentType, result := range state.AgentResults {
			sb.WriteString(fmt.Sprintf("### %s Agent 结果\n", agentType))
			if result.Error != "" {
				sb.WriteString(fmt.Sprintf("执行出错: %s\n", result.Error))
			} else {
				sb.WriteString(result.Content)
			}
			sb.WriteString("\n\n")
		}
	}

	messages := []*schema.Message{
		schema.SystemMessage(SummaryPrompt),
		schema.UserMessage(sb.String()),
	}

	resp, err := s.llm.Generate(ctx, messages)
	if err != nil {
		// 降级：拼接各Agent结果
		log.Printf("[Summary] LLM integration failed: %v, using concatenation", err)
		return s.fallbackIntegration(state), nil
	}

	return resp.Content, nil
}

func (s *SummaryNode) fallbackIntegration(state *GraphState) string {
	var sb strings.Builder

	if state.Plan != nil {
		for _, agentType := range state.Plan.ExecutionOrder {
			if result, ok := state.AgentResults[agentType]; ok && result.Error == "" {
				sb.WriteString(result.Content)
				sb.WriteString("\n\n")
			}
		}
	}

	if sb.Len() == 0 {
		return "抱歉，处理过程中出现了问题，请重试。"
	}

	return sb.String()
}
