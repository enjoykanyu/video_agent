package agent

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
)

// FullWorkflowGraph 完整的Graph工作流：用户输入→意图识别→工具分流
func FullWorkflowGraph() {
	ctx := context.Background()
	
	// 创建Graph，输入为string类型，输出为interface{}类型以兼容不同工具的结果
	g := compose.NewGraph[string, interface{}]()
	
	// 节点1: 意图识别
	intentNode := compose.InvokableLambda(func(ctx context.Context, input string) (output interface{}, err error) {
		fmt.Printf("接收到用户输入: %s\n", input)
		
		// 调用意图识别Agent
		result, err := RecognizeIntentAPI(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("意图识别失败: %v", err)
		}
		
		fmt.Printf("识别到意图: %s\n", result.Type)
		return string(result.Type), nil
	})
	
	// 节点2: MCP工具处理
	mcpToolNode := compose.InvokableLambda(func(ctx context.Context, input interface{}) (output interface{}, err error) {
		inputStr := input.(string)
		fmt.Printf("MCP工具处理输入: %s\n", inputStr)
		
		// 创建工具调度器并调用MCP工具
		dispatcher, err := NewToolDispatcher(ctx)
		if err != nil {
			return nil, fmt.Errorf("创建工具调度器失败: %v", err)
		}
		
		result, err := dispatcher.DispatchByIntent(ctx, IntentMCP, inputStr)
		if err != nil {
			return nil, fmt.Errorf("MCP工具处理失败: %v", err)
		}
		
		fmt.Printf("MCP工具结果: %+v\n", result)
		return result, nil
	})
	
	// 节点3: 普通问答工具
	qaToolNode := compose.InvokableLambda(func(ctx context.Context, input interface{}) (output interface{}, err error) {
		inputStr := input.(string)
		fmt.Printf("问答工具处理输入: %s\n", inputStr)
		
		// 创建工具调度器并调用问答工具
		dispatcher, err := NewToolDispatcher(ctx)
		if err != nil {
			return nil, fmt.Errorf("创建工具调度器失败: %v", err)
		}
		
		result, err := dispatcher.DispatchByIntent(ctx, IntentQA, inputStr)
		if err != nil {
			return nil, fmt.Errorf("问答工具处理失败: %v", err)
		}
		
		fmt.Printf("问答工具结果: %+v\n", result)
		return result, nil
	})
	
	// 节点4: RAG知识库工具
	ragToolNode := compose.InvokableLambda(func(ctx context.Context, input interface{}) (output interface{}, err error) {
		inputStr := input.(string)
		fmt.Printf("RAG工具处理输入: %s\n", inputStr)
		
		// 创建工具调度器并调用RAG工具
		dispatcher, err := NewToolDispatcher(ctx)
		if err != nil {
			return nil, fmt.Errorf("创建工具调度器失败: %v", err)
		}
		
		result, err := dispatcher.DispatchByIntent(ctx, IntentRAG, inputStr)
		if err != nil {
			return nil, fmt.Errorf("RAG工具处理失败: %v", err)
		}
		
		fmt.Printf("RAG工具结果: %+v\n", result)
		return result, nil
	})
	
	// 注册所有节点
	nodes := map[string]*compose.Lambda{
		"intent": intentNode,
		"mcp":    mcpToolNode,
		"qa":     qaToolNode,
		"rag":    ragToolNode,
	}
	
	for name, node := range nodes {
		if err := g.AddLambdaNode(name, node); err != nil {
			panic(fmt.Errorf("添加节点 %s 失败: %v", name, err))
		}
	}
	
	// 添加分支：根据意图选择不同的工具
	err := g.AddBranch("intent", compose.NewGraphBranch(func(ctx context.Context, intent interface{}) (endNode string, err error) {
		intentStr := intent.(string)
		switch intentStr {
		case "mcp":
			return "mcp", nil
		case "qa":
			return "qa", nil
		case "rag":
			return "rag", nil
		default:
			// 默认使用问答工具
			return "qa", nil
		}
	}, map[string]bool{"mcp": true, "qa": true, "rag": true}))
	if err != nil {
		panic(fmt.Errorf("添加分支失败: %v", err))
	}
	
	// 连接节点：START → intent → [mcp|qa|rag] → END
	if err := g.AddEdge(compose.START, "intent"); err != nil {
		panic(fmt.Errorf("连接START到intent失败: %v", err))
	}
	
	// 连接所有工具节点到END
	toolNodes := []string{"mcp", "qa", "rag"}
	for _, tool := range toolNodes {
		if err := g.AddEdge(tool, compose.END); err != nil {
			panic(fmt.Errorf("连接%s到END失败: %v", tool, err))
		}
	}
	
	// 编译Graph
	r, err := g.Compile(ctx)
	if err != nil {
		panic(fmt.Errorf("编译Graph失败: %v", err))
	}
	
	// 测试不同的输入
	testInputs := []string{
		"帮我添加一个待办事项",
		"什么是人工智能",
		"搜索关于机器学习的资料",
		"随便问点什么",
	}
	
	for _, input := range testInputs {
		fmt.Printf("\n=== 测试输入: %s ===\n", input)
		
		result, err := r.Invoke(ctx, input)
		if err != nil {
			fmt.Printf("执行失败: %v\n", err)
			continue
		}
		
		fmt.Printf("最终结果: %s\n", result)
	}
}

// TestFullWorkflow 测试完整工作流
func TestFullWorkflow() {
	fmt.Println("=== 测试完整Graph工作流 ===")
	FullWorkflowGraph()
}