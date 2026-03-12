package base

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ToolExecutor struct {
	tools []tool.BaseTool
	llm   model.ChatModel
}

func NewToolExecutor(tools []tool.BaseTool, llm model.ChatModel) *ToolExecutor {
	return &ToolExecutor{
		tools: tools,
		llm:   llm,
	}
}

func (te *ToolExecutor) ExecuteWithTools(
	ctx context.Context,
	messages []*schema.Message,
) (*schema.Message, []string, error) {
	var toolsUsed []string

	if len(te.tools) > 0 {
		toolInfos := make([]*schema.ToolInfo, len(te.tools))
		for i, t := range te.tools {
			info, _ := t.Info(ctx)
			toolInfos[i] = info
		}
		log.Printf("[ToolExecutor] binding %d tools: %v", len(toolInfos), func() []string {
			names := make([]string, len(toolInfos))
			for i, info := range toolInfos {
				names[i] = info.Name
			}
			return names
		}())
		if err := te.llm.BindTools(toolInfos); err != nil {
			log.Printf("[ToolExecutor] bind tools warning: %v", err)
		} else {
			log.Printf("[ToolExecutor] tools bound successfully")
		}
	}

	resp, err := te.llm.Generate(ctx, messages)
	if err != nil {
		return nil, nil, fmt.Errorf("LLM generate failed: %w", err)
	}

	log.Printf("[ToolExecutor] LLM response: content=%q, tool_calls=%d", resp.Content, len(resp.ToolCalls))

	if len(resp.ToolCalls) > 0 && len(te.tools) > 0 {
		log.Printf("[ToolExecutor] executing tool calls...")
		toolResultMsgs := make([]*schema.Message, 0, len(resp.ToolCalls)+1)
		toolResultMsgs = append(toolResultMsgs, resp)

		for _, tc := range resp.ToolCalls {
			toolsUsed = append(toolsUsed, tc.Function.Name)

			var result string
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
				log.Printf("[ToolExecutor] unmarshal args error: %v", err)
				result = fmt.Sprintf("参数解析失败: %v", err)
			} else {
				for _, t := range te.tools {
					info, _ := t.Info(ctx)
					if info.Name == tc.Function.Name {
						invokable, ok := t.(tool.InvokableTool)
						if !ok {
							result = fmt.Sprintf("tool %s is not invokable", tc.Function.Name)
							break
						}
						argsJSON, _ := json.Marshal(args)
						log.Println("调用工具之前+v%", argsJSON)
						output, err := invokable.InvokableRun(ctx, string(argsJSON))
						log.Printf("[ToolExecutor] 工具调用返回 %+v", output)

						if err != nil {
							result = fmt.Sprintf("tool execution failed: %v", err)
						} else {
							result = extractMCPToolResult(fmt.Sprintf("%v", output))
							log.Printf("工具格式转换后返回: %s", result)
						}
						break
					}
				}
			}

			toolMsg := &schema.Message{
				Role:       schema.Tool,
				Content:    result,
				ToolCallID: tc.ID,
			}
			toolResultMsgs = append(toolResultMsgs, toolMsg)
			log.Printf("[ToolExecutor] tool %s result: %s", tc.Function.Name, result)
		}
		//到这里工具调用完成 拼接工具返回和agent的系统提示词
		log.Printf("[ToolExecutor] sending %d messages to LLM for final generation", len(toolResultMsgs))
		resp, err = te.llm.Generate(ctx, toolResultMsgs)
		//到这里会调用agent 综合工具数据返回给出了分析
		if err != nil {
			log.Printf("[ToolExecutor] LLM generate after tool warning: %v, returning tool result directly", err)
			toolResultContent := ""
			for i := 1; i < len(toolResultMsgs); i++ {
				toolResultContent += toolResultMsgs[i].Content + "\n"
			}
			return &schema.Message{
				Role:    schema.Assistant,
				Content: toolResultContent,
			}, toolsUsed, nil
		}
		log.Printf("[ToolExecutor] final response: %s", resp.Content)
	}

	return resp, toolsUsed, nil
}

type BaseAgent struct {
	name         types.AgentType
	llm          model.ChatModel
	toolExecutor *ToolExecutor
	systemPrompt string
	maxToolRound int
}

func NewBaseAgent(name types.AgentType, llm model.ChatModel, te *ToolExecutor, systemPrompt string) *BaseAgent {
	return &BaseAgent{
		name:         name,
		llm:          llm,
		toolExecutor: te,
		systemPrompt: systemPrompt,
		maxToolRound: 5,
	}
}

func (b *BaseAgent) Name() types.AgentType {
	return b.name
}

func (b *BaseAgent) ExecuteWithToolLoop(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[%s] starting execution", b.name)

	messages := state.BuildMessagesForAgent(b.systemPrompt, b.name)

	var resp *schema.Message
	var toolsUsed []string
	var err error

	if b.toolExecutor != nil {
		resp, toolsUsed, err = b.toolExecutor.ExecuteWithTools(ctx, messages)
	} else {
		resp, err = b.llm.Generate(ctx, messages)
	}

	if err != nil {
		return &types.AgentResult{
			AgentType: b.name,
			Content:   fmt.Sprintf("执行失败: %v", err),
			Error:     err.Error(),
		}, err
	}
	log.Printf("调用了工具返回response before decode: %+v", resp)
	log.Printf("[%s] LLM response: content=%q, tool_calls=%d", b.name, resp.Content, len(resp.ToolCalls))
	decodedContent := decodeToolResult(resp.Content)

	var toolCalls []schema.ToolCall
	if len(resp.ToolCalls) > 0 {
		toolCalls = resp.ToolCalls
	}

	return &types.AgentResult{
		AgentType: b.name,
		Content:   decodedContent,
		ToolsUsed: toolsUsed,
		ToolCalls: toolCalls,
	}, nil
}

func decodeToolResult(result string) string {
	log.Printf("mcp解码[decodeToolResult] input length: %d", len(result))

	result = strings.TrimSpace(result)

	var outer string
	if err := json.Unmarshal([]byte(result), &outer); err == nil {
		log.Printf("[decodeToolResult] step0: result is a JSON string, unwrapping")
		result = outer
	}

	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(result), &wrapper); err != nil {
		log.Printf("[decodeToolResult] step1: failed to unmarshal wrapper: %v", err)
		return result
	}

	if content, ok := wrapper["content"].([]interface{}); ok && len(content) > 0 {
		if item, ok := content[0].(map[string]interface{}); ok {
			if text, ok := item["text"].(string); ok {
				log.Printf("[decodeToolResult] step1: found text field, trying to decode inner JSON")

				cleaned := strings.Trim(text, "\"")

				var inner map[string]interface{}
				if err := json.Unmarshal([]byte(cleaned), &inner); err != nil {
					log.Printf("[decodeToolResult] step2: failed to parse inner JSON: %v", err)
					return cleaned
				}

				formatted, _ := json.MarshalIndent(inner, "", "  ")
				log.Printf("[decodeToolResult] step2: decoded successfully, length: %d", len(formatted))
				return string(formatted)
			}
		}
	}

	formatted, _ := json.MarshalIndent(wrapper, "", "  ")
	return string(formatted)
}

func extractMCPToolResult(raw string) string {
	var wrapper map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
		log.Printf("[extractMCPToolResult] not JSON, returning raw")
		return raw
	}

	if structured, ok := wrapper["structuredContent"].(string); ok && structured != "" {
		decoded, err := base64.StdEncoding.DecodeString(structured)
		if err == nil {
			log.Printf("[extractMCPToolResult] decoded from structuredContent, length: %d", len(decoded))
			return string(decoded)
		}
		log.Printf("[extractMCPToolResult] base64 decode failed: %v", err)
	}

	if content, ok := wrapper["content"].([]interface{}); ok && len(content) > 0 {
		if item, ok := content[0].(map[string]interface{}); ok {
			if text, ok := item["text"].(string); ok {
				cleaned := strings.Trim(text, "\"")
				decoded, err := base64.StdEncoding.DecodeString(cleaned)
				if err == nil {
					log.Printf("[extractMCPToolResult] decoded from content.text, length: %d", len(decoded))
					return string(decoded)
				}
				log.Printf("[extractMCPToolResult] text is not base64, returning as-is")
				return text
			}
		}
	}

	return raw
}

func (b *BaseAgent) DefaultRoute(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	if result.Error != "" {
		return types.AgentTypeSummary, nil
	}
	return types.AgentTypeSummary, nil
}

func (b *BaseAgent) postProcess(result *types.AgentResult) *types.AgentResult {
	return result
}

func ExtractToolResultAsContext(toolResults string) string {
	var sb strings.Builder
	sb.WriteString(toolResults)
	return sb.String()
}
