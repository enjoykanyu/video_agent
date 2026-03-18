package hot_live

import (
	"context"
	"log"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

type HotLiveAgentNode struct {
	*base.BaseAgent
}

func NewHotLiveAgentNode(llm model.ChatModel, te *base.ToolExecutor) *HotLiveAgentNode {
	return &HotLiveAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeHotLive, llm, te, prompt.HotLiveAgentPrompt),
	}
}

func (a *HotLiveAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[HotLiveAgent] executing for query: %s", state.OriginalQuery)

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *HotLiveAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *HotLiveAgentNode) postProcess(result *types.AgentResult) *types.AgentResult {
	return result
}
