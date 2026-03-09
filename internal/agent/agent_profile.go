package agent

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
)

type ProfileAgentNode struct {
	*BaseAgent
}

func NewProfileAgentNode(llm model.ChatModel, te *ToolExecutor) *ProfileAgentNode {
	return &ProfileAgentNode{
		BaseAgent: NewBaseAgent(AgentTypeProfile, llm, te, ProfileAgentPrompt),
	}
}

func (a *ProfileAgentNode) Execute(ctx context.Context, state *GraphState) (*AgentResult, error) {
	log.Printf("[ProfileAgent] executing for query: %s", state.OriginalQuery)

	// 用户画像Agent可以利用视频观看数据
	if videoResult, ok := state.GetAgentResult(AgentTypeVideo); ok {
		log.Printf("[ProfileAgent] using video watching data, length: %d",
			len(videoResult.Content))
	}

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *ProfileAgentNode) Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *ProfileAgentNode) postProcess(result *AgentResult) *AgentResult {
	// Profile Agent特定后处理
	return result
}
