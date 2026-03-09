package agent

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
)

type AnalysisAgentNode struct {
	*BaseAgent
}

func NewAnalysisAgentNode(llm model.ChatModel, te *ToolExecutor) *AnalysisAgentNode {
	return &AnalysisAgentNode{
		BaseAgent: NewBaseAgent(AgentTypeAnalysis, llm, te, AnalysisAgentPrompt),
	}
}

func (a *AnalysisAgentNode) Execute(ctx context.Context, state *GraphState) (*AgentResult, error) {
	log.Printf("[AnalysisAgent] executing for query: %s", state.OriginalQuery)

	// 检查是否有前置Video Agent的结果可用
	if videoResult, ok := state.GetAgentResult(AgentTypeVideo); ok {
		log.Printf("[AnalysisAgent] using video agent result as context, length: %d",
			len(videoResult.Content))
	}

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	// 分析Agent特有后处理：提取关键指标
	result = a.postProcess(result)

	return result, nil
}

func (a *AnalysisAgentNode) Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *AnalysisAgentNode) postProcess(result *AgentResult) *AgentResult {
	// Analysis Agent特定后处理
	// 例如：标准化分析数据格式，提取KPI指标
	return result
}
