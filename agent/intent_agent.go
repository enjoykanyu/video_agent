package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// IntentType 定义意图类型
type IntentType string

const (
	IntentMCP    IntentType = "mcp"      // MCP处理工具意图
	IntentQA     IntentType = "qa"       // 普通问答意图  
	IntentRAG    IntentType = "rag"      // RAG知识库意图
	IntentUnknown IntentType = "unknown" // 未知意图
)

// IntentResult 意图识别结果
type IntentResult struct {
	Type        IntentType `json:"type"`
	Confidence  float64    `json:"confidence"`
	Explanation string     `json:"explanation"`
	Input       string     `json:"input"` // 原始用户输入
}

// IntentAgent 意图识别Agent
type IntentAgent struct {
	chatModel *ollama.ChatModel
}

// NewIntentAgent 创建新的意图识别Agent
func NewIntentAgent(ctx context.Context) (*IntentAgent, error) {
	chatModel, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434", // Ollama 服务地址
		Model:   "qwen3:0.6b",             // 使用千问模型
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create chat model: %w", err)
	}

	return &IntentAgent{
		chatModel: chatModel,
	}, nil
}

// RecognizeIntent 识别用户输入的意图
func (ia *IntentAgent) RecognizeIntent(ctx context.Context, userInput string) (*IntentResult, error) {
	// 构建意图识别提示词
	systemPrompt := `You are a professional intent classifier. Analyze the user's input and determine the intent type. The user input is in Chinese.

Here are the definitions for each intent type:
1.  **mcp**: Use this for tasks that require executing a tool to perform an action or a complex query. Examples: "add a to-do item", "list my tasks", "search the eino repository", "generate go code for an http server", "delete the temp file".
2.  **rag**: Use this for when the user is asking to find or search for information within a knowledge base or documents. Examples: "search for documents about machine learning", "what does the Eino framework documentation say about agents?".
3.  **qa**: Use this for general questions, conversations, or queries that can be answered directly without needing specific tools or document retrieval. Examples: "what is AI?", "what is the weather like today?", "tell me a joke".

Analyze the following user input and respond strictly in the JSON format below, with no other text or explanations.

User Input: ` + userInput + `

JSON Response format:
{
  "type": "mcp|qa|rag",
  "confidence": 0.0-1.0,
  "explanation": "A brief explanation for the classification."
}`

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: systemPrompt,
		},
		{
			Role:    schema.User,
			Content: userInput,
		},
	}

	// 调用模型进行意图识别
	response, err := ia.chatModel.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke chat model: %w", err)
	}

	// 解析模型响应
	result, err := parseIntentResponse(response.Content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse intent response: %w", err)
	}

	result.Input = userInput // 确保返回原始输入

	return result, nil
}

// parseIntentResponse 解析模型返回的意图响应
func parseIntentResponse(content string) (*IntentResult, error) {
	// 首先尝试提取JSON部分
	jsonStart := strings.Index(content, "{")
	jsonEnd := strings.LastIndex(content, "}")
	if jsonStart == -1 || jsonEnd == -1 || jsonEnd <= jsonStart {
		// 如果没有找到完整JSON，使用简单匹配作为fallback
		return parseIntentFallback(content)
	}

	jsonStr := content[jsonStart : jsonEnd+1]
	
	// 使用简单的JSON解析（实际项目中应该使用encoding/json）
	if strings.Contains(jsonStr, `"type": "mcp"`) {
		return &IntentResult{
			Type:        IntentMCP,
			Confidence:  0.85,
			Explanation: "识别为MCP工具处理意图",
		}, nil
	} else if strings.Contains(jsonStr, `"type": "rag"`) {
		return &IntentResult{
			Type:        IntentRAG,
			Confidence:  0.85,
			Explanation: "识别为RAG知识库检索意图",
		}, nil
	} else if strings.Contains(jsonStr, `"type": "qa"`) {
		return &IntentResult{
			Type:        IntentQA,
			Confidence:  0.85,
			Explanation: "识别为普通问答意图",
		}, nil
	}

	// 如果JSON中没有明确类型，使用fallback
	return parseIntentFallback(content)
}

// parseIntentFallback 使用字符串匹配作为fallback解析
func parseIntentFallback(content string) (*IntentResult, error) {
	content = strings.ToLower(content)
	
	var intentType IntentType
	confidence := 0.7
	explanation := "基于关键词匹配的意图分类"

	// 关键词匹配逻辑
	if strings.Contains(content, "待办") || strings.Contains(content, "任务") || strings.Contains(content, "添加") ||
	   strings.Contains(content, "生成") || strings.Contains(content, "代码") ||
	   strings.Contains(content, "文件") || strings.Contains(content, "命令") ||
	   strings.Contains(content, "mcp") {
		intentType = IntentMCP
		explanation = "包含工具调用或任务管理关键词"
	} else if strings.Contains(content, "知识库") || strings.Contains(content, "文档") ||
	           strings.Contains(content, "查找") || strings.Contains(content, "检索") ||
	           strings.Contains(content, "rag") {
		intentType = IntentRAG  
		explanation = "包含知识库检索关键词"
	} else {
		intentType = IntentQA
		explanation = "普通问答内容"
	}

	return &IntentResult{
		Type:        intentType,
		Confidence:  confidence,
		Explanation:  explanation,
	}, nil
}

// CreateIntentGraph 创建意图识别的Graph工作流
func CreateIntentGraph(ctx context.Context) (*compose.Graph[string, *IntentResult], error) {
	g := compose.NewGraph[string, *IntentResult]()

	// 创建意图识别Agent
	intentAgent, err := NewIntentAgent(ctx)
	if err != nil {
		return nil, err
	}

	// 定义意图识别Lambda节点
	intentLambda := compose.InvokableLambda(func(ctx context.Context, input string) (output *IntentResult, err error) {
		return intentAgent.RecognizeIntent(ctx, input)
	})

	// 添加节点
	err = g.AddLambdaNode("intent_recognition", intentLambda)
	if err != nil {
		return nil, err
	}

	// 连接节点
	err = g.AddEdge(compose.START, "intent_recognition")
	if err != nil {
		return nil, err
	}

	err = g.AddEdge("intent_recognition", compose.END)
	if err != nil {
		return nil, err
	}

	return g, nil
}

// RecognizeIntentAPI API接口使用的意图识别函数
func RecognizeIntentAPI(ctx context.Context, userInput string) (*IntentResult, error) {
	intentAgent, err := NewIntentAgent(ctx)
	if err != nil {
		return nil, err
	}
	return intentAgent.RecognizeIntent(ctx, userInput)
}

// ProcessCompleteFlow 完整处理流程
func ProcessCompleteFlow(ctx context.Context, userInput string) (map[string]interface{}, error) {
	// 创建完整的Graph
	graph, err := CreateCompleteGraph(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create complete graph: %w", err)
	}

	// 编译Graph
	compiledGraph, err := graph.Compile(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to compile graph: %w", err)
	}

	// 执行完整流程
	result, err := compiledGraph.Invoke(ctx, userInput)
	if err != nil {
		return nil, fmt.Errorf("failed to invoke complete flow: %w", err)
	}

	return map[string]interface{}{
		"result":      result,
		"user_input":  userInput,
		"status":      "completed",
		"processing_flow": "user_input → intent_recognition → tool_dispatch",
	}, nil
}

// TestIntentRecognition 测试意图识别功能
func TestIntentRecognition() {
	ctx := context.Background()
	
	// 创建意图识别Graph
	graph, err := CreateIntentGraph(ctx)
	if err != nil {
		fmt.Printf("Failed to create intent graph: %v\n", err)
		return
	}

	// 编译Graph
	compiledGraph, err := graph.Compile(ctx)
	if err != nil {
		fmt.Printf("Failed to compile graph: %v\n", err)
		return
	}

	// 测试用例
	testInputs := []string{
		"请帮我生成一个Go语言的HTTP服务器代码",
		"今天的天气怎么样？",
		"从知识库中查找关于Eino框架的文档",
		"列出当前目录的文件",
	}

	for _, input := range testInputs {
		fmt.Printf("输入: %s\n", input)
		
		result, err := compiledGraph.Invoke(ctx, input)
		if err != nil {
			fmt.Printf("识别失败: %v\n", err)
			continue
		}

		fmt.Printf("意图类型: %s\n", result.Type)
		fmt.Printf("置信度: %.2f\n", result.Confidence)
		fmt.Printf("说明: %s\n", result.Explanation)
		fmt.Println("---")
	}
}