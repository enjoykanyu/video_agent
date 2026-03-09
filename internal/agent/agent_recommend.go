package agent

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
)

type RecommendAgentNode struct {
	*BaseAgent
}

func NewRecommendAgentNode(llm model.ChatModel, te *ToolExecutor) *RecommendAgentNode {
	return &RecommendAgentNode{
		BaseAgent: NewBaseAgent(AgentTypeRecommend, llm, te, RecommendAgentPrompt),
	}
}

func (a *RecommendAgentNode) Execute(ctx context.Context, state *GraphState) (*AgentResult, error) {
	log.Printf("[RecommendAgent] executing for query: %s", state.OriginalQuery)

	// 推荐Agent可以参考用户画像和分析结果
	if profileResult, ok := state.GetAgentResult(AgentTypeProfile); ok {
		log.Printf("[RecommendAgent] using user profile for recommendation, length: %d",
			len(profileResult.Content))
	}
	if analysisResult, ok := state.GetAgentResult(AgentTypeAnalysis); ok {
		log.Printf("[RecommendAgent] using analysis for recommendation, length: %d",
			len(analysisResult.Content))
	}

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *RecommendAgentNode) Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *RecommendAgentNode) postProcess(result *AgentResult) *AgentResult {
	// Recommend Agent特定后处理
	return result
}
