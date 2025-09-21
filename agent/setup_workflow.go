package agent

import (
	"context"

	"github.com/cloudwego/eino/compose"
)

// SetupCompleteWorkflow 设置完整的Graph工作流
func SetupCompleteWorkflow(g *compose.Graph[string, interface{}]) error {
	ctx := context.Background()
	
	// 创建意图识别Agent
	intentAgent, err := NewIntentAgent(ctx)
	if err != nil {
		return err
	}

	// 创建工具调度器
	toolDispatcher, err := NewToolDispatcher(ctx)
	if err != nil {
		return err
	}

	// 意图识别节点
	intentLambda := compose.InvokableLambda(func(ctx context.Context, input string) (output *IntentResult, err error) {
		return intentAgent.RecognizeIntent(ctx, input)
	})

	// 工具调度节点
	dispatchLambda := compose.InvokableLambda(func(ctx context.Context, intentResult *IntentResult) (output interface{}, err error) {
		return toolDispatcher.DispatchByIntent(ctx, intentResult.Type, intentResult.Input)
	})

	// 添加节点
	err = g.AddLambdaNode("intent_recognition", intentLambda)
	if err != nil {
		return err
	}

	err = g.AddLambdaNode("tool_dispatch", dispatchLambda)
	if err != nil {
		return err
	}

	// 添加分支：根据意图类型路由到不同的处理
	err = g.AddEdge(compose.START, "intent_recognition")
	if err != nil {
		return err
	}

	err = g.AddEdge("intent_recognition", "tool_dispatch")
	if err != nil {
		return err
	}

	err = g.AddEdge("tool_dispatch", compose.END)
	if err != nil {
		return err
	}

	return nil
}