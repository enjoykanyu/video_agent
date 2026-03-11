package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"

	"video_agent/mcp"
)

const (
	NodeIntentModel = "intent_model"
	NodeTransList   = "trans_list"
	NodeRAG         = "rag_retrieval"
	NodeToToolCall  = "to_tool_call"
	NodeMCPInput    = "mcp_input"
	NodeMCP         = "mcp"
)

type VideoGraph struct {
	runner      compose.Runnable[[]*schema.Message, []*schema.Message]
	llm         model.ChatModel
	mcpTools    []tool.BaseTool
	reportAgent *ReportAgentNode
}

func NewVideoGraph(llm model.ChatModel, mcpServers []MCPServer) (*VideoGraph, error) {
	ctx := context.Background()

	var mcpTools []tool.BaseTool
	mcpTools, err := mcp.GetMCPTool(ctx)
	if err != nil {
		log.Printf("[Graph] warning: get MCP tools failed: %v (continuing without MCP)", err)
		mcpTools = nil
	}

	if llm == nil {
		return nil, fmt.Errorf("llm is required")
	}

	reportTools := selectToolsForAgent(mcpTools, AgentTypeReport)
	te := NewToolExecutor(reportTools, llm)
	reportAgent := NewReportAgentNode(llm, te)

	vg := &VideoGraph{
		llm:         llm,
		mcpTools:    mcpTools,
		reportAgent: reportAgent,
	}

	if err := vg.buildGraph(); err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	return vg, nil
}

func selectToolsForAgent(allTools []tool.BaseTool, agentType AgentType) []tool.BaseTool {
	if len(allTools) == 0 {
		return nil
	}

	ctx := context.Background()
	var filtered []tool.BaseTool

	for _, t := range allTools {
		info, _ := t.Info(ctx)
		toolName := info.Name

		switch agentType {
		case AgentTypeReport:
			if strings.Contains(toolName, "video") || strings.Contains(toolName, "user") ||
				strings.Contains(toolName, "analysis") || strings.Contains(toolName, "data") {
				filtered = append(filtered, t)
			}
		case AgentTypeCreation:
			if strings.Contains(toolName, "create") || strings.Contains(toolName, "upload") ||
				strings.Contains(toolName, "publish") {
				filtered = append(filtered, t)
			}
		case AgentTypeAnalysis:
			if strings.Contains(toolName, "video") || strings.Contains(toolName, "user") ||
				strings.Contains(toolName, "analytics") || strings.Contains(toolName, "stat") {
				filtered = append(filtered, t)
			}
		case AgentTypeProfile:
			if strings.Contains(toolName, "user") || strings.Contains(toolName, "profile") {
				filtered = append(filtered, t)
			}
		case AgentTypeRecommend:
			if strings.Contains(toolName, "video") || strings.Contains(toolName, "recommend") {
				filtered = append(filtered, t)
			}
		default:
			filtered = append(filtered, t)
		}
	}

	log.Printf("[Graph] agent %s selected %d tools", agentType, len(filtered))
	return filtered
}

func (vg *VideoGraph) buildGraph() error {
	ctx := context.Background()

	g := compose.NewGraph[[]*schema.Message, []*schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *GraphState {
			return NewGraphState("", "", "")
		}),
	)

	_ = g.AddLambdaNode(NodeIntentModel, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		var state *GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *GraphState) error {
			state = s
			if state.OriginalQuery == "" && len(input) > 0 {
				state.OriginalQuery = input[len(input)-1].Content
				state.Messages = input
			}
			return nil
		})
		if err != nil {
			return nil, err
		}

		intentTemp := prompt.FromMessages(schema.FString,
			schema.SystemMessage("你是一个意图识别专家。请严格按规则判断用户意图：\n规则：\n- 如果用户询问视频数据分析、视频统计、视频报表、生成报告、分析某个视频的任何内容，必须回答 'Report'\n- 如果用户只是闲聊、问候、普通问答，回答 'Chat'\n注意：只要涉及\"分析视频\"、\"视频数据\"、\"视频统计\"、\"报表\"等关键词，都必须回答 'Report'"),
			schema.UserMessage("{query}"),
		)

		output, err := intentTemp.Format(ctx, map[string]any{
			"query": state.OriginalQuery,
		})
		if err != nil {
			return nil, err
		}

		resp, err := vg.llm.Generate(ctx, output)
		if err != nil {
			return nil, err
		}

		log.Printf("[Graph] intent decision: %s", resp.Content)

		return []*schema.Message{resp}, nil
	}))

	_ = g.AddLambdaNode(NodeTransList, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		if len(input) == 0 {
			return input, nil
		}
		return input, nil
	}))

	_ = g.AddLambdaNode(NodeRAG, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		var state *GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *GraphState) error {
			state = s
			return nil
		})
		if err != nil {
			return nil, err
		}

		log.Printf("[Graph] RAG retrieval for query: %s", state.OriginalQuery)

		state.SetRAGDocuments([]RAGDocument{})
		state.FinalAnswer = "RAG检索完成"

		return []*schema.Message{
			schema.AssistantMessage("RAG检索完成", nil),
		}, nil
	}))

	_ = g.AddLambdaNode(NodeToToolCall, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		if len(input) == 0 {
			return input, nil
		}
		msg := input[len(input)-1]

		hasToolCall := len(msg.ToolCalls) > 0
		if !hasToolCall {
			log.Printf("[Graph] no tool call in message, skip MCP")
			return input, nil
		}

		toolCallMsg, err := mcp.MsgToToolCall(ctx, msg)
		if err != nil {
			return nil, err
		}
		return []*schema.Message{toolCallMsg}, nil
	}))

	if vg.reportAgent != nil {
		_ = g.AddLambdaNode("report_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing report agent for query: %s", state.OriginalQuery)

			result, err := vg.reportAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] report agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(AgentTypeReport, result)
			state.FinalAnswer = result.Content

			nextAgent, err := vg.reportAgent.Route(ctx, state, result)
			log.Printf("[Graph] report agent route result: %s", nextAgent)

			msgs := make([]*schema.Message, 0)
			if len(result.ToolCalls) > 0 {
				msgs = append(msgs, &schema.Message{
					Role:      schema.Assistant,
					Content:   result.Content,
					ToolCalls: result.ToolCalls,
				})
			} else {
				msgs = append(msgs, schema.AssistantMessage(result.Content, nil))
			}
			return msgs, nil
		}))
	}

	if len(vg.mcpTools) > 0 {
		_ = g.AddLambdaNode(NodeMCPInput, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) (*schema.Message, error) {
			if len(input) == 0 {
				return nil, nil
			}
			msg := input[len(input)-1]
			if len(msg.ToolCalls) == 0 {
				return nil, nil
			}
			return msg, nil
		}))

		mcpNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
			Tools: vg.mcpTools,
		})
		if err != nil {
			log.Printf("[Graph] warning: create MCP tool node failed: %v", err)
		} else {
			err = g.AddToolsNode(NodeMCP, mcpNode)
			if err != nil {
				log.Printf("[Graph] warning: add MCP tool node failed: %v", err)
			} else {
				log.Printf("[Graph] MCP tool node registered, tools count: %d", len(vg.mcpTools))
			}
		}
	}

	_ = g.AddEdge(compose.START, NodeIntentModel)
	_ = g.AddEdge(NodeIntentModel, NodeTransList)

	_ = g.AddBranch(NodeTransList, compose.NewGraphBranch(
		func(ctx context.Context, msgs []*schema.Message) (string, error) {
			if len(msgs) == 0 {
				return compose.END, nil
			}
			content := strings.ToUpper(msgs[len(msgs)-1].Content)
			if strings.Contains(content, "REPORT") {
				return "report_agent", nil
			}
			return NodeRAG, nil
		},
		map[string]bool{
			"report_agent": true,
			NodeRAG:        true,
		},
	))

	_ = g.AddEdge("report_agent", NodeToToolCall)
	_ = g.AddEdge(NodeRAG, compose.END)

	if len(vg.mcpTools) > 0 {
		_ = g.AddBranch(NodeToToolCall, compose.NewGraphBranch(
			func(ctx context.Context, msgs []*schema.Message) (string, error) {
				if len(msgs) == 0 {
					return compose.END, nil
				}
				msg := msgs[len(msgs)-1]
				if len(msg.ToolCalls) == 0 {
					return compose.END, nil
				}
				return NodeMCPInput, nil
			},
			map[string]bool{
				NodeMCPInput: true,
				compose.END:  true,
			},
		))

		_ = g.AddEdge(NodeMCPInput, NodeMCP)
		_ = g.AddEdge(NodeMCP, compose.END)
	} else {
		_ = g.AddEdge(NodeToToolCall, compose.END)
	}

	compiled, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("compile graph: %w", err)
	}

	vg.runner = compiled
	return nil
}

func (vg *VideoGraph) Run(ctx context.Context, messages []*schema.Message) ([]*schema.Message, error) {
	if vg.runner == nil {
		return nil, fmt.Errorf("graph not initialized")
	}
	return vg.runner.Invoke(ctx, messages)
}
