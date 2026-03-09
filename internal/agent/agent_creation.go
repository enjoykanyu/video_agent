package agent

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
)

type CreationAgentNode struct {
	*BaseAgent
}

func NewCreationAgentNode(llm model.ChatModel, te *ToolExecutor) *CreationAgentNode {
	return &CreationAgentNode{
		BaseAgent: NewBaseAgent(AgentTypeCreation, llm, te, CreationAgentPrompt),
	}
}

func (a *CreationAgentNode) Execute(ctx context.Context, state *GraphState) (*AgentResult, error) {
	log.Printf("[CreationAgent] executing for query: %s", state.OriginalQuery)

	// 创作Agent可以参考分析结果
	if analysisResult, ok := state.GetAgentResult(AgentTypeAnalysis); ok {
		log.Printf("[CreationAgent] using analysis result as reference, length: %d",
			len(analysisResult.Content))
	}

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	result = a.postProcess(result)
	return result, nil
}

func (a *CreationAgentNode) Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *CreationAgentNode) postProcess(result *AgentResult) *AgentResult {
	// Creation Agent特定后处理
	// 例如：格式化创作内容，添加结构化标签
	return result
}
