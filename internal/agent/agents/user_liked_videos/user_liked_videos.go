package user_liked_videos

import (
	"context"
	"log"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

type UserLikedVideosAgentNode struct {
	*base.BaseAgent
}

func NewUserLikedVideosAgentNode(llm model.ChatModel, te *base.ToolExecutor) *UserLikedVideosAgentNode {
	return &UserLikedVideosAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeUserLikedVideos, llm, te, prompt.UserLikedVideosAgentPrompt),
	}
}

func (a *UserLikedVideosAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[UserLikedVideosAgent] executing for query: %s", state.OriginalQuery)

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *UserLikedVideosAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *UserLikedVideosAgentNode) postProcess(result *types.AgentResult) *types.AgentResult {
	return result
}
