package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"video_agent/rag"
	"video_agent/tool"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ToolDispatcher 工具调度器
type ToolDispatcher struct {
	mcpTools   []einotool.BaseTool
	qaModel    *ollama.ChatModel
	ragTools   []einotool.BaseTool // RAG工具（后续实现）
}

// NewToolDispatcher 创建新的工具调度器
func NewToolDispatcher(ctx context.Context) (*ToolDispatcher, error) {
	// 初始化MCP工具
	mcpTools := []einotool.BaseTool{
		&tool.ListTodoTool{},
		&tool.AddTodoTool{},
		&tool.SearchRepoTool{},
	}

	// 初始化问答模型
	qaModel, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3:0.6b",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create QA model: %w", err)
	}

	// 初始化RAG工具
	ragTool, err := rag.CreateRAGTool(ctx, "./data/rag_store")
	if err != nil {
		return nil, fmt.Errorf("failed to create RAG tool: %w", err)
	}

	ragTools := []einotool.BaseTool{ragTool}

	return &ToolDispatcher{
		mcpTools: mcpTools,
		qaModel:  qaModel,
		ragTools: ragTools,
	}, nil
}

// DispatchByIntent 根据意图类型调度工具
func (td *ToolDispatcher) DispatchByIntent(ctx context.Context, intent IntentType, userInput string) (interface{}, error) {
	switch intent {
	case IntentMCP:
		return td.handleMCPIntent(ctx, userInput)
	case IntentQA:
		return td.handleQAIntent(ctx, userInput)
	case IntentRAG:
		return td.handleRAGIntent(ctx, userInput)
	default:
		return td.handleUnknownIntent(ctx, userInput)
	}
}

// handleMCPIntent 处理MCP工具意图
func (td *ToolDispatcher) handleMCPIntent(ctx context.Context, userInput string) (interface{}, error) {
	// 1. Build the prompt for the LLM to select a tool and extract arguments.
	prompt, err := td.buildToolSelectionPrompt(userInput)
	if err != nil {
		return nil, fmt.Errorf("failed to build tool selection prompt: %w", err)
	}

	// 2. Call the LLM. We can reuse the qaModel for this.
	messages := []*schema.Message{
		{Role: schema.System, Content: prompt},
	}
	response, err := td.qaModel.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("tool selection LLM call failed: %w", err)
	}

	// 3. Parse the LLM's response to get the tool name and arguments.
	var toolSelection struct {
		ToolName string          `json:"tool_name"`
		ToolArgs json.RawMessage `json:"tool_args"`
	}

	jsonStr := extractJSON(response.Content)
	if jsonStr == "" {
		// Fallback to QA if the model doesn't return valid JSON
		return td.handleQAIntent(ctx, userInput)
	}

	if err := json.Unmarshal([]byte(jsonStr), &toolSelection); err != nil {
		// Fallback to QA if JSON parsing fails
		return td.handleQAIntent(ctx, userInput)
	}

	if toolSelection.ToolName == "none" || toolSelection.ToolName == "" {
		// If LLM decides no tool is suitable, fallback to QA.
		return td.handleQAIntent(ctx, userInput)
	}

	// 4. Find the selected tool.
	var selectedTool einotool.BaseTool
	for _, t := range td.mcpTools {
		info, _ := t.Info(ctx)
		if info.Name == toolSelection.ToolName {
			selectedTool = t
			break
		}
	}

	if selectedTool == nil {
		// If tool is not found, maybe the model hallucinated. Fallback to QA.
		return td.handleQAIntent(ctx, userInput)
	}

	// 5. Invoke the tool with the extracted arguments.
	invokableTool, ok := selectedTool.(interface {
		InvokableRun(ctx context.Context, argumentsInJSON string, opts ...einotool.Option) (string, error)
	})
	if !ok {
		return nil, fmt.Errorf("tool %s is not invokable", toolSelection.ToolName)
	}
	result, err := invokableTool.InvokableRun(ctx, string(toolSelection.ToolArgs))
	if err != nil {
		return nil, fmt.Errorf("failed to invoke tool %s: %w", toolSelection.ToolName, err)
	}

	// 6. Return the result.
	return map[string]interface{}{
		"type":       "mcp",
		"result":     result,
		"tool_used":  toolSelection.ToolName,
		"user_input": userInput,
	}, nil
}

// handleQAIntent 处理普通问答意图
func (td *ToolDispatcher) handleQAIntent(ctx context.Context, userInput string) (interface{}, error) {
	// 直接使用问答模型回答
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个有帮助的AI助手，请用中文回答用户的问题。",
		},
		{
			Role:    schema.User,
			Content: userInput,
		},
	}

	response, err := td.qaModel.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke QA model: %w", err)
	}

	return map[string]interface{}{
		"type":   "qa",
		"answer": response.Content,
		"model":  "qwen3:0.6b",
	}, nil
}

// handleRAGIntent 处理RAG知识库意图
func (td *ToolDispatcher) handleRAGIntent(ctx context.Context, userInput string) (interface{}, error) {
	// 直接调用RAG模块进行检索
	result, err := rag.SearchDocuments(ctx, userInput, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to search documents: %w", err)
	}

	// 格式化结果
	var formattedResults []map[string]interface{}
	for _, doc := range result {
		formattedResults = append(formattedResults, map[string]interface{}{
			"id":       doc.ID,
			"content":  doc.Content,
			"metadata": doc.Metadata,
		})
	}

	return map[string]interface{}{
		"type":       "rag",
		"result":     formattedResults,
		"query":      userInput,
		"total_hits": len(result),
		"tools_used": []string{"rag_knowledge_base"},
	}, nil
}

// handleUnknownIntent 处理未知意图
func (td *ToolDispatcher) handleUnknownIntent(ctx context.Context, userInput string) (interface{}, error) {
	// 默认使用问答模型
	return td.handleQAIntent(ctx, userInput)
}

// getToolNames 获取工具名称列表
func getToolNames(tools []einotool.BaseTool) []string {
	var names []string
	for _, t := range tools {
		// 简化处理，实际应该调用Info方法获取名称
		switch t.(type) {
		case *tool.ListTodoTool:
			names = append(names, "list_todo")
		case *tool.AddTodoTool:
			names = append(names, "add_todo")
		case *tool.SearchRepoTool:
			names = append(names, "search_repo")
		default:
			// 处理RAG工具
			if _, ok := t.(*rag.RAGTool); ok {
				names = append(names, "rag_knowledge_base")
			}
		}
	}
	return names
}

// CreateCompleteGraph 创建完整的处理Graph（用户输入→意图识别→工具分流）
func CreateCompleteGraph(ctx context.Context) (*compose.Graph[string, interface{}], error) {
	g := compose.NewGraph[string, interface{}]()

	// 创建意图识别Agent
	intentAgent, err := NewIntentAgent(ctx)
	if err != nil {
		return nil, err
	}

	// 创建工具调度器
	toolDispatcher, err := NewToolDispatcher(ctx)
	if err != nil {
		return nil, err
	}

	// 完整处理节点：意图识别 + 工具调度
	completeLambda := compose.InvokableLambda(func(ctx context.Context, input string) (output interface{}, err error) {
		// 1. 意图识别
		intentResult, err := intentAgent.RecognizeIntent(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("意图识别失败: %w", err)
		}

		// 2. 工具调度
		result, err := toolDispatcher.DispatchByIntent(ctx, intentResult.Type, input)
		if err != nil {
			return nil, fmt.Errorf("工具调度失败: %w", err)
		}

		// 3. 返回完整结果
		return map[string]interface{}{
			"intent":     intentResult,
			"tool_result": result,
			"user_input": input,
		}, nil
	})

	// 添加节点
	err = g.AddLambdaNode("complete_processing", completeLambda)
	if err != nil {
		return nil, err
	}

	// 连接节点
	err = g.AddEdge(compose.START, "complete_processing")
	if err != nil {
		return nil, err
	}

	err = g.AddEdge("complete_processing", compose.END)
	if err != nil {
		return nil, err
	}

	return g, nil
}

// buildToolSelectionPrompt builds the prompt for the LLM.
func (td *ToolDispatcher) buildToolSelectionPrompt(userInput string) (string, error) {
	toolInfos := []string{}
	for _, t := range td.mcpTools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue // Or handle error
		}
		argsBytes, err := json.Marshal(info.ParamsOneOf)
		if err != nil {
			continue // Or handle error
		}
		toolInfos = append(toolInfos, fmt.Sprintf("- Tool Name: %s\n  Description: %s\n  Arguments (JSON Schema): %s", info.Name, info.Desc, string(argsBytes)))
	}

	prompt := fmt.Sprintf(`You are an expert at selecting the right tool and extracting its arguments from a user's request.
Given the user's input, choose the most appropriate tool from the list below and provide the arguments as a JSON object.

Available Tools:
%s

User Input: "%s"

Respond ONLY with a single JSON object in the following format, without any other text or explanation.
{
  "tool_name": "the_name_of_the_tool",
  "tool_args": { "arg1": "value1", "arg2": "value2" }
}
If no tool is suitable, respond with:
{
  "tool_name": "none",
  "tool_args": {}
}
`, strings.Join(toolInfos, "\n\n"), userInput)

	return prompt, nil
}

// extractJSON finds and returns the first valid JSON object from a string.
func extractJSON(s string) string {
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
}

// formatMessagesContent 格式化消息内容
func formatMessagesContent(messages []*schema.Message) string {
	if len(messages) == 0 {
		return "无响应内容"
	}
	
	// 获取最后一条消息的内容
	lastMessage := messages[len(messages)-1]
	return lastMessage.Content
}



// TestToolDispatcher 测试工具调度功能
func TestToolDispatcher() {
	ctx := context.Background()

	// 创建工具调度器
	dispatcher, err := NewToolDispatcher(ctx)
	if err != nil {
		fmt.Printf("Failed to create tool dispatcher: %v\n", err)
		return
	}

	// 测试用例
	testCases := []struct {
		input    string
		intent   IntentType
		expected string
	}{
		{"请帮我添加一个学习任务", IntentMCP, "mcp"},
		{"今天的天气怎么样", IntentQA, "qa"},
		{"查找文档资料", IntentRAG, "rag"},
	}

	for _, tc := range testCases {
		fmt.Printf("测试输入: %s\n", tc.input)
		fmt.Printf("预期意图: %s\n", tc.expected)

		result, err := dispatcher.DispatchByIntent(ctx, tc.intent, tc.input)
		if err != nil {
			fmt.Printf("调度失败: %v\n", err)
			continue
		}

		fmt.Printf("处理结果: %+v\n", result)
		fmt.Println("---")
	}
}