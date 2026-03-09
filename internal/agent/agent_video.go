package agent

import (
	"context"
	"log"

	"github.com/cloudwego/eino/components/model"
)

type VideoAgentNode struct {
	*BaseAgent
}

func NewVideoAgentNode(llm model.ChatModel, te *ToolExecutor) *VideoAgentNode {
	return &VideoAgentNode{
		BaseAgent: NewBaseAgent(AgentTypeVideo, llm, te, VideoAgentPrompt),
	}
}

func (a *VideoAgentNode) Execute(ctx context.Context, state *GraphState) (*AgentResult, error) {
	log.Printf("[VideoAgent] executing for query: %s", state.OriginalQuery)

	// 使用基础的Tool循环执行
	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	// Video Agent特有的后处理逻辑
	// 比如：从tool结果中提取视频ID列表，存入state供后续Agent使用
	result = a.postProcess(result)

	return result, nil
}

func (a *VideoAgentNode) Route(ctx context.Context, state *GraphState, result *AgentResult) (AgentType, error) {
	return a.DefaultRoute(ctx, state, result)
}

func (a *VideoAgentNode) postProcess(result *AgentResult) *AgentResult {
	// Video Agent的特定后处理
	// 例如：解析视频信息，标准化数据格式
	return result
}
