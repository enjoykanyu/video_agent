package comment_analysis

import (
	"context"
	"log"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

type CommentAnalysisAgentNode struct {
	*base.BaseAgent
}

func NewCommentAnalysisAgentNode(llm model.ChatModel, te *base.ToolExecutor) *CommentAnalysisAgentNode {
	return &CommentAnalysisAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeCommentAnalysis, llm, te, prompt.CommentAnalysisAgentPrompt),
	}
}

func (a *CommentAnalysisAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[CommentAnalysisAgent] executing for query: %s", state.OriginalQuery)

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *CommentAnalysisAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *CommentAnalysisAgentNode) postProcess(result *types.AgentResult) *types.AgentResult {
	return result
}
