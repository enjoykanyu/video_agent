package video_recommend

import (
	"context"
	"log"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

type VideoRecommendAgentNode struct {
	*base.BaseAgent
}

func NewVideoRecommendAgentNode(llm model.ChatModel, te *base.ToolExecutor) *VideoRecommendAgentNode {
	return &VideoRecommendAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeVideoRecommend, llm, te, prompt.VideoRecommendAgentPrompt),
	}
}

func (a *VideoRecommendAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[VideoRecommendAgent] executing for query: %s", state.OriginalQuery)

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *VideoRecommendAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *VideoRecommendAgentNode) postProcess(result *types.AgentResult) *types.AgentResult {
	return result
}
