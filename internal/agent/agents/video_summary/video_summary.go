package video_summary

import (
	"context"
	"log"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

type VideoSummaryAgentNode struct {
	*base.BaseAgent
}

func NewVideoSummaryAgentNode(llm model.ChatModel, te *base.ToolExecutor) *VideoSummaryAgentNode {
	return &VideoSummaryAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeVideoSummary, llm, te, prompt.VideoSummaryAgentPrompt),
	}
}

func (a *VideoSummaryAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[VideoSummaryAgent] executing for query: %s", state.OriginalQuery)

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *VideoSummaryAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *VideoSummaryAgentNode) postProcess(result *types.AgentResult) *types.AgentResult {
	return result
}
