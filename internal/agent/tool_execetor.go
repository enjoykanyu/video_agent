package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// ToolExecutor 封装LLM + Tool的执行循环
// 核心逻辑：LLM生成 -> 检查tool_calls -> 执行tool -> 将结果回传LLM -> 循环直到无tool_calls
type ToolExecutor struct {
	mcpManager *MCPClientManager
	mcpServers []MCPServer // MCP Server 配置列表
	maxRounds  int         // 最大tool调用轮数
}

func NewToolExecutor(mcpManager *MCPClientManager, mcpServers []MCPServer, maxRounds int) *ToolExecutor {
	if maxRounds <= 0 {
		maxRounds = 5
	}
	return &ToolExecutor{
		mcpManager: mcpManager,
		mcpServers: mcpServers,
		maxRounds:  maxRounds,
	}
}

// ExecuteWithTools 执行带Tool调用循环的LLM调用
// 返回最终的LLM回复和使用过的工具名列表
func (te *ToolExecutor) ExecuteWithTools(
	ctx context.Context,
	llm model.ChatModel,
	messages []*schema.Message,
) (finalResponse *schema.Message, toolsUsed []string, err error) {

	// 绑定工具
	toolInfos := te.mcpManager.GetToolInfos()
	if len(toolInfos) > 0 {
		if err := llm.BindTools(toolInfos); err != nil {
			log.Printf("[ToolExecutor] bind tools warning: %v", err)
			// 继续执行，可能不需要工具
		}
	}

	currentMessages := make([]*schema.Message, len(messages))
	copy(currentMessages, messages)

	for round := 0; round < te.maxRounds; round++ {
		log.Printf("[ToolExecutor] round %d, messages count: %d", round+1, len(currentMessages))

		// 调用LLM
		resp, err := llm.Generate(ctx, currentMessages)
		if err != nil {
			return nil, toolsUsed, fmt.Errorf("LLM generate failed (round %d): %w", round+1, err)
		}

		// 检查是否有tool_calls
		if len(resp.ToolCalls) == 0 {
			// 没有工具调用，返回最终结果
			log.Printf("[ToolExecutor] no tool calls, final response ready")
			return resp, toolsUsed, nil
		}

		// 将assistant的回复（含tool_calls）加入消息历史
		currentMessages = append(currentMessages, resp)

		// 执行每个tool call
		for _, toolCall := range resp.ToolCalls {
			log.Printf("[ToolExecutor] executing tool: %s (id: %s)",
				toolCall.Function.Name, toolCall.ID)

			toolResult, execErr := te.executeSingleTool(ctx, toolCall)
			toolsUsed = append(toolsUsed, toolCall.Function.Name)

			// 构建tool结果消息
			var resultContent string
			if execErr != nil {
				resultContent = fmt.Sprintf(`{"error": "%s"}`, execErr.Error())
				log.Printf("[ToolExecutor] tool %s failed: %v",
					toolCall.Function.Name, execErr)
			} else {
				resultContent = toolResult
				log.Printf("[ToolExecutor] tool %s success, result length: %d",
					toolCall.Function.Name, len(toolResult))
			}

			// 添加tool结果消息
			toolMsg := &schema.Message{
				Role:       schema.Tool,
				Content:    resultContent,
				ToolCallID: toolCall.ID,
			}
			currentMessages = append(currentMessages, toolMsg)
		}
	}

	// 超过最大轮数，做最后一次不带工具的调用
	log.Printf("[ToolExecutor] max rounds reached, final call without tools")

	// 解绑工具，最后一次调用不允许再使用工具
	llm.BindTools(nil)

	resp, err := llm.Generate(ctx, currentMessages)
	if err != nil {
		return nil, toolsUsed, fmt.Errorf("final LLM generate failed: %w", err)
	}

	return resp, toolsUsed, nil
}

// executeSingleTool 执行单个工具调用
func (te *ToolExecutor) executeSingleTool(ctx context.Context, toolCall schema.ToolCall) (string, error) {
	toolName := toolCall.Function.Name

	t, ok := te.mcpManager.GetToolByName(toolName)
	if !ok {
		return "", fmt.Errorf("tool not found: %s", toolName)
	}

	// 解析参数
	args := toolCall.Function.Arguments
	log.Printf("[ToolExecutor] tool %s args: %s", toolName, args)

	// 调用MCP工具
	invokable, ok := t.(tool.InvokableTool)
	if !ok {
		return "", fmt.Errorf("tool %s does not support invokable call", toolName)
	}

	log.Printf("[ToolExecutor] 准备调用 MCP 工具: %s", toolName)

	result, err := invokable.InvokableRun(ctx, args)
	if err != nil {
		log.Printf("[ToolExecutor] MCP 工具调用失败: %s, error: %v", toolName, err)
		return "", fmt.Errorf("tool %s execution failed: %w", toolName, err)
	}

	log.Printf("[ToolExecutor] tool %s success, result length: %d",
		toolName, len(result))

	return result, nil
}

// ExecuteWithoutTools 执行不带Tool的LLM调用
func (te *ToolExecutor) ExecuteWithoutTools(
	ctx context.Context,
	llm model.ChatModel,
	messages []*schema.Message,
) (*schema.Message, error) {
	resp, err := llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM generate failed: %w", err)
	}
	return resp, nil
}
