package report

import (
	"context"
	"log"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

type ReportAgentNode struct {
	*base.BaseAgent
}

func NewReportAgentNode(llm model.ChatModel, te *base.ToolExecutor) *ReportAgentNode {
	return &ReportAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeReport, llm, te, prompt.ReportAgentPrompt),
	}
}

func (a *ReportAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[ReportAgent] executing for query: %s", state.OriginalQuery)

	// 报表Agent通常需要分析数据
	if analysisResult, ok := state.GetAgentResult(types.AgentTypeAnalysis); ok {
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

func (a *ReportAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	log.Printf("进入了新目录结构")
	return a.DefaultRoute(ctx, state, result)
}

func (a *ReportAgentNode) postProcess(result *types.AgentResult) *types.AgentResult {
	// Report Agent特定后处理
	// 例如：添加报表格式标记，生成目录
	return result
}
