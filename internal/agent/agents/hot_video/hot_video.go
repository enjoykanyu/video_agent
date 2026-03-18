package hot_video

import (
	"context"
	"log"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

type HotVideoAgentNode struct {
	*base.BaseAgent
}

func NewHotVideoAgentNode(llm model.ChatModel, te *base.ToolExecutor) *HotVideoAgentNode {
	return &HotVideoAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeHotVideo, llm, te, prompt.HotVideoAgentPrompt),
	}
}

func (a *HotVideoAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[HotVideoAgent] executing for query: %s", state.OriginalQuery)

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *HotVideoAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *HotVideoAgentNode) postProcess(result *types.AgentResult) *types.AgentResult {
	return result
}
