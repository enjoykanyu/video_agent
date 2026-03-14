package graph

import (
	"context"
	"fmt"
	"log"
	"strings"

	"video_agent/internal/agent/agents/base"
	"video_agent/internal/agent/agents/creative_analysis"
	report "video_agent/internal/agent/agents/report"
	"video_agent/internal/agent/agents/summary"
	"video_agent/internal/agent/state"
	states "video_agent/internal/agent/state"
	"video_agent/internal/agent/types"
	"video_agent/mcp"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

const (
	NodeIntentModel = "intent_model"
	NodeTransList   = "trans_list"
	NodeRAG         = "rag_retrieval"
	NodeToToolCall  = "to_tool_call"
	NodeMCPInput    = "mcp_input"
	NodeMCP         = "mcp"
	NodeSummary     = "summary"
)

type VideoGraph struct {
	runner                compose.Runnable[[]*schema.Message, []*schema.Message]
	llm                   model.ChatModel
	mcpTools              []tool.BaseTool
	reportAgent           *report.ReportAgentNode
	creativeAnalysisAgent *creative_analysis.CreativeAnalysisAgentNode
	summaryNode           *summary.SummaryNode
}

func NewVideoGraph(llm model.ChatModel, mcpServers []types.MCPServer) (*VideoGraph, error) {
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

	reportTools := selectToolsForAgent(mcpTools, types.AgentTypeReport)
	te := base.NewToolExecutor(reportTools, llm)
	reportAgent := report.NewReportAgentNode(llm, te)

	creativeAnalysisTools := selectToolsForAgent(mcpTools, types.AgentTypeCreativeAnalysis)
	creativeAnalysisTE := base.NewToolExecutor(creativeAnalysisTools, llm)
	creativeAnalysisAgent := creative_analysis.NewCreativeAnalysisAgentNode(llm, creativeAnalysisTE)

	summaryNode := summary.NewSummaryNode(llm)

	vg := &VideoGraph{
		llm:                   llm,
		mcpTools:              mcpTools,
		reportAgent:           reportAgent,
		creativeAnalysisAgent: creativeAnalysisAgent,
		summaryNode:           summaryNode,
	}

	if err := vg.buildGraph(); err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	return vg, nil
}

func selectToolsForAgent(allTools []tool.BaseTool, agentType types.AgentType) []tool.BaseTool {
	if len(allTools) == 0 {
		return nil
	}

	ctx := context.Background()
	var filtered []tool.BaseTool

	for _, t := range allTools {
		info, _ := t.Info(ctx)
		toolName := info.Name

		switch agentType {
		case types.AgentTypeReport:
			if strings.Contains(toolName, "video") || strings.Contains(toolName, "user") ||
				strings.Contains(toolName, "analysis") || strings.Contains(toolName, "data") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeCreation:
			if strings.Contains(toolName, "create") || strings.Contains(toolName, "upload") ||
				strings.Contains(toolName, "publish") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeAnalysis:
			if strings.Contains(toolName, "video") || strings.Contains(toolName, "user") ||
				strings.Contains(toolName, "analytics") || strings.Contains(toolName, "stat") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeProfile:
			if strings.Contains(toolName, "user") || strings.Contains(toolName, "profile") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeRecommend:
			if strings.Contains(toolName, "video") || strings.Contains(toolName, "recommend") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeCreativeAnalysis:
			if strings.Contains(toolName, "trend") || strings.Contains(toolName, "hot") ||
				strings.Contains(toolName, "search") || strings.Contains(toolName, "video") {
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
		compose.WithGenLocalState(func(ctx context.Context) *states.GraphState {
			return state.NewGraphState("", "", "")
		}),
	)

	_ = g.AddLambdaNode(NodeIntentModel, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		var state *states.GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
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
			schema.SystemMessage(`你是一个意图识别专家。请严格按规则判断用户意图：
规则：
- 如果用户询问视频数据分析、视频统计、视频报表、生成报告、分析某个视频的任何内容，必须回答 'Report'
- 如果用户询问领域热门选题、创作趋势分析、竞品内容分析、受众需求洞察、"什么选题最火"、"XX领域有什么好的创作方向"等内容，必须回答 'Creative'
- 如果用户只是闲聊、问候、普通问答，回答 'Chat'
注意：
- 只要涉及"分析视频"、"视频数据"、"视频统计"、"报表"等关键词，都必须回答 'Report'
- 只要涉及"选题"、"热门"、"趋势"、"领域"、"创作方向"、"受众"等关键词，都必须回答 'Creative'`),
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
		var state *states.GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
			state = s
			return nil
		})
		if err != nil {
			return nil, err
		}

		log.Printf("[Graph] RAG retrieval for query: %s", state.OriginalQuery)

		state.SetRAGDocuments([]types.RAGDocument{})
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
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
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

			state.SetAgentResult(types.AgentTypeReport, result)

			nextAgent, err := vg.reportAgent.Route(ctx, state, result)
			log.Printf("[Graph] report agent route result: %s", nextAgent)

			// 返回包含 ToolCalls 的消息，让后续节点处理
			if len(result.ToolCalls) > 0 {
				return []*schema.Message{{
					Role:      schema.Assistant,
					Content:   result.Content,
					ToolCalls: result.ToolCalls,
				}}, nil
			}
			// 没有 ToolCalls，返回空消息继续到 Summary 节点
			return []*schema.Message{}, nil
		}))
	}

	// 添加创作分析Agent节点
	if vg.creativeAnalysisAgent != nil {
		_ = g.AddLambdaNode("creative_analysis_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing creative analysis agent for query: %s", state.OriginalQuery)

			result, err := vg.creativeAnalysisAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] creative analysis agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("创作分析执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeCreativeAnalysis, result)

			nextAgent, err := vg.creativeAnalysisAgent.Route(ctx, state, result)
			log.Printf("[Graph] creative analysis agent route result: %s", nextAgent)

			// 返回包含 ToolCalls 的消息
			if len(result.ToolCalls) > 0 {
				return []*schema.Message{{
					Role:      schema.Assistant,
					Content:   result.Content,
					ToolCalls: result.ToolCalls,
				}}, nil
			}
			return []*schema.Message{}, nil
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
			if strings.Contains(content, "CREATIVE") {
				return "creative_analysis_agent", nil
			}
			return NodeRAG, nil
		},
		map[string]bool{
			"report_agent":            true,
			"creative_analysis_agent": true,
			NodeRAG:                   true,
		},
	))

	_ = g.AddEdge("report_agent", NodeToToolCall)
	_ = g.AddEdge("creative_analysis_agent", NodeToToolCall)
	_ = g.AddEdge(NodeRAG, compose.END)

	// 添加 Summary 节点，用于整合和格式化最终结果
	_ = g.AddLambdaNode(NodeSummary, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		var state *states.GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
			state = s
			return nil
		})
		if err != nil {
			return nil, err
		}

		log.Printf("[Graph] executing summary node for query: %s", state.OriginalQuery)

		result, err := vg.summaryNode.Execute(ctx, state)
		if err != nil {
			log.Printf("[Graph] summary node error: %v", err)
			return []*schema.Message{
				schema.AssistantMessage(fmt.Sprintf("整合结果失败: %v", err), nil),
			}, nil
		}

		state.FinalAnswer = result
		return []*schema.Message{
			schema.AssistantMessage(result, nil),
		}, nil
	}))

	if len(vg.mcpTools) > 0 {
		_ = g.AddBranch(NodeToToolCall, compose.NewGraphBranch(
			func(ctx context.Context, msgs []*schema.Message) (string, error) {
				if len(msgs) == 0 {
					return NodeSummary, nil
				}
				msg := msgs[len(msgs)-1]
				if len(msg.ToolCalls) == 0 {
					return NodeSummary, nil
				}
				return NodeMCPInput, nil
			},
			map[string]bool{
				NodeMCPInput: true,
				NodeSummary:  true,
			},
		))

		_ = g.AddEdge(NodeMCPInput, NodeMCP)
		_ = g.AddEdge(NodeMCP, NodeSummary)
		_ = g.AddEdge(NodeSummary, compose.END)
	} else {
		_ = g.AddEdge(NodeToToolCall, NodeSummary)
		_ = g.AddEdge(NodeSummary, compose.END)
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
