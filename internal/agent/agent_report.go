package agent

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
)

type ReportAgentNode struct {
	*BaseAgent
}

func NewReportAgentNode(llm model.ChatModel, te *ToolExecutor) *ReportAgentNode {
	return &ReportAgentNode{
		BaseAgent: NewBaseAgent(AgentTypeReport, llm, te, ReportAgentPrompt),
	}
}

func (a *ReportAgentNode) Execute(ctx context.Context, state *GraphState) (*AgentResult, error) {
	log.Printf("[ReportAgent] executing for query: %s", state.OriginalQuery)

	// 报表Agent通常需要分析数据
	if analysisResult, ok := state.GetAgentResult(AgentTypeAnalysis); ok {
		log.Printf("[ReportAgent] using analysis data for report, length: %d",
			len(analysisResult.Content))
	}

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *ReportAgentNode) Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *ReportAgentNode) postProcess(result *AgentResult) *AgentResult {
	// Report Agent特定后处理
	// 例如：添加报表格式标记，生成目录
	return result
}
