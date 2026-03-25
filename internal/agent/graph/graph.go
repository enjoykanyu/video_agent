package graph

import (
	"context"
	"fmt"
	"log"
	"strings"

	"video_agent/internal/agent/agents/base"
	"video_agent/internal/agent/agents/comment_analysis"
	"video_agent/internal/agent/agents/creative_analysis"
	"video_agent/internal/agent/agents/hot_live"
	"video_agent/internal/agent/agents/hot_video"
	"video_agent/internal/agent/agents/rag_selector"
	report "video_agent/internal/agent/agents/report"
	"video_agent/internal/agent/agents/summary"
	"video_agent/internal/agent/agents/user_liked_videos"
	"video_agent/internal/agent/agents/video_recommend"
	"video_agent/internal/agent/agents/video_summary"
	"video_agent/internal/agent/state"
	states "video_agent/internal/agent/state"
	"video_agent/internal/agent/types"
	"video_agent/mcp"
	"video_agent/rag"

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

	// Agent 节点名称常量
	NodeReportAgent           = "report_agent"
	NodeCreativeAnalysisAgent = "creative_analysis_agent"
	NodeRAGSelectorAgent      = "rag_selector_agent"
	NodeCommentAnalysisAgent  = "comment_analysis_agent"
	NodeVideoRecommendAgent   = "video_recommend_agent"
	NodeUserLikedVideosAgent  = "user_liked_videos_agent"
	NodeHotVideoAgent         = "hot_video_agent"
	NodeHotLiveAgent          = "hot_live_agent"
	NodeVideoSummaryAgent     = "video_summary_agent"
)

type VideoGraph struct {
	runner                compose.Runnable[[]*schema.Message, []*schema.Message]
	llm                   model.ChatModel
	mcpTools              []tool.BaseTool
	reportAgent           *report.ReportAgentNode
	creativeAnalysisAgent *creative_analysis.CreativeAnalysisAgentNode
	ragSelectorAgent      *rag_selector.RAGSelectorAgentNode
	summaryNode           *summary.SummaryNode
	commentAnalysisAgent  *comment_analysis.CommentAnalysisAgentNode
	videoRecommendAgent   *video_recommend.VideoRecommendAgentNode
	userLikedVideosAgent  *user_liked_videos.UserLikedVideosAgentNode
	hotVideoAgent         *hot_video.HotVideoAgentNode
	hotLiveAgent          *hot_live.HotLiveAgentNode
	videoSummaryAgent     *video_summary.VideoSummaryAgentNode
}

// AgentNode 定义 Agent 节点的通用接口
type AgentNode interface {
	Execute(ctx context.Context, state *states.GraphState) (*types.AgentResult, error)
	Route(ctx context.Context, state *states.GraphState, result *types.AgentResult) (types.AgentType, error)
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

	ragSelectorAgent := rag_selector.NewRAGSelectorAgentNode(llm, nil, nil)

	summaryNode := summary.NewSummaryNode(llm)

	commentAnalysisTools := selectToolsForAgent(mcpTools, types.AgentTypeCommentAnalysis)
	commentAnalysisTE := base.NewToolExecutor(commentAnalysisTools, llm)
	commentAnalysisAgent := comment_analysis.NewCommentAnalysisAgentNode(llm, commentAnalysisTE)

	videoRecommendTools := selectToolsForAgent(mcpTools, types.AgentTypeVideoRecommend)
	videoRecommendTE := base.NewToolExecutor(videoRecommendTools, llm)
	videoRecommendAgent := video_recommend.NewVideoRecommendAgentNode(llm, videoRecommendTE)

	userLikedVideosTools := selectToolsForAgent(mcpTools, types.AgentTypeUserLikedVideos)
	userLikedVideosTE := base.NewToolExecutor(userLikedVideosTools, llm)
	userLikedVideosAgent := user_liked_videos.NewUserLikedVideosAgentNode(llm, userLikedVideosTE)

	hotVideoTools := selectToolsForAgent(mcpTools, types.AgentTypeHotVideo)
	hotVideoTE := base.NewToolExecutor(hotVideoTools, llm)
	hotVideoAgent := hot_video.NewHotVideoAgentNode(llm, hotVideoTE)

	hotLiveTools := selectToolsForAgent(mcpTools, types.AgentTypeHotLive)
	hotLiveTE := base.NewToolExecutor(hotLiveTools, llm)
	hotLiveAgent := hot_live.NewHotLiveAgentNode(llm, hotLiveTE)

	videoSummaryTools := selectToolsForAgent(mcpTools, types.AgentTypeVideoSummary)
	videoSummaryTE := base.NewToolExecutor(videoSummaryTools, llm)
	videoSummaryAgent := video_summary.NewVideoSummaryAgentNode(llm, videoSummaryTE)

	vg := &VideoGraph{
		llm:                   llm,
		mcpTools:              mcpTools,
		reportAgent:           reportAgent,
		creativeAnalysisAgent: creativeAnalysisAgent,
		ragSelectorAgent:      ragSelectorAgent,
		summaryNode:           summaryNode,
		commentAnalysisAgent:  commentAnalysisAgent,
		videoRecommendAgent:   videoRecommendAgent,
		userLikedVideosAgent:  userLikedVideosAgent,
		hotVideoAgent:         hotVideoAgent,
		hotLiveAgent:          hotLiveAgent,
		videoSummaryAgent:     videoSummaryAgent,
	}

	if err := vg.buildGraph(); err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	return vg, nil
}

// createAgentLambda 创建 Agent 节点的 Lambda 函数（使用标准 Node 类型模式）
func (vg *VideoGraph) createAgentLambda(agent AgentNode, agentType types.AgentType, agentName string) *compose.Lambda {
	return compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		var state *states.GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
			state = s
			return nil
		})
		if err != nil {
			return nil, err
		}

		log.Printf("[Graph] executing %s for query: %s", agentName, state.OriginalQuery)

		result, err := agent.Execute(ctx, state)
		if err != nil {
			log.Printf("[Graph] %s error: %v", agentName, err)
			return []*schema.Message{
				schema.AssistantMessage(fmt.Sprintf("执行失败: %v", err), nil),
			}, nil
		}

		state.SetAgentResult(agentType, result)

		nextAgent, _ := agent.Route(ctx, state, result)
		log.Printf("[Graph] %s route result: %s", agentName, nextAgent)

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
	})
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
		case types.AgentTypeRAGSelector:
			filtered = nil
		case types.AgentTypeRAG:
			if strings.Contains(toolName, "search") || strings.Contains(toolName, "retrieve") ||
				strings.Contains(toolName, "query") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeCommentAnalysis:
			if strings.Contains(toolName, "comment") || strings.Contains(toolName, "danmaku") ||
				strings.Contains(toolName, "video") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeVideoRecommend:
			if strings.Contains(toolName, "recommend") || strings.Contains(toolName, "interest") ||
				strings.Contains(toolName, "video") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeUserLikedVideos:
			if strings.Contains(toolName, "like") || strings.Contains(toolName, "user") ||
				strings.Contains(toolName, "video") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeHotVideo:
			if strings.Contains(toolName, "hot") || strings.Contains(toolName, "video") ||
				strings.Contains(toolName, "trend") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeHotLive:
			if strings.Contains(toolName, "live") || strings.Contains(toolName, "hot") ||
				strings.Contains(toolName, "stream") {
				filtered = append(filtered, t)
			}
		case types.AgentTypeVideoSummary:
			if strings.Contains(toolName, "video") || strings.Contains(toolName, "transcribe") ||
				strings.Contains(toolName, "file") {
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
			schema.SystemMessage(`你是一个意图识别专家。请分析用户查询，只输出意图类型。

【意图类型定义】
1. RAG - 知识库查询：询问网站/系统/产品的功能、介绍、使用方法
2. Report - 视频数据分析：分析视频数据、生成报表、统计数据、查询视频信息
3. VideoSummary - 视频内容总结：总结视频内容、视频讲什么
4. CommentAnalysis - 评论分析：分析评论、弹幕、观众反馈
5. VideoRecommend - 视频推荐：推荐视频、找好看的内容（注意：不是分析已有视频）
6. UserLikedVideos - 点赞查询：查询点赞记录、喜欢的视频
7. HotVideo - 热门视频：查询最火视频、热门内容
8. HotLive - 热门直播：查询热门直播、正在直播
9. Creative - 创作分析：选题分析、趋势分析、竞品分析
10. Chat - 闲聊：问候、日常对话

【关键区分】
- Report：用户想"分析/查看/查询"某个具体视频的数据或信息（有明确视频ID或想查某个视频）
- VideoRecommend：用户想"推荐/找"视频（没有具体视频ID，想要推荐列表）
- VideoSummary：用户想"总结/概括"视频内容（视频讲了什么）

【Few-shot示例】
Q: "这个网站干啥的"
A: RAG

Q: "VisionWorld是什么"
A: RAG

Q: "系统怎么用"
A: RAG

Q: "介绍一下产品功能"
A: RAG

Q: "分析一下视频123的数据"
A: Report

Q: "分析下视频1766329556"
A: Report

Q: "查看视频123的信息"
A: Report

Q: "查询视频数据"
A: Report

Q: "总结一下视频内容"
A: VideoSummary

Q: "这个视频讲了什么"
A: VideoSummary

Q: "推荐一些好看的视频"
A: VideoRecommend

Q: "有什么好看的视频"
A: VideoRecommend

Q: "最近什么视频最火"
A: HotVideo

Q: "帮我分析评论"
A: CommentAnalysis

Q: "你好"
A: Chat

【任务】
分析以下查询，只输出意图类型（RAG/Report/VideoSummary/CommentAnalysis/VideoRecommend/UserLikedVideos/HotVideo/HotLive/Creative/Chat）：

用户查询：{query}`),
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

		// 获取优化后的查询（优先使用）或原始查询
		query := state.GetOptimizedQuery()
		if query == "" {
			query = state.OriginalQuery
		}
		log.Printf("[Graph] RAG retrieval for query: %s", query)

		// 企业级 RAG 流程：检索 → 阈值过滤 → LLM 生成
		// 使用向量检索
		ragResult := rag.RetrieverRAGTop1(query, rag.ScoreRelevant)

		var answer string

		// 记录检索结果到状态
		if ragResult.HasResult && ragResult.TopDocument != nil {
			var ragDocs []types.RAGDocument
			ragDocs = append(ragDocs, types.RAGDocument{
				ID:       ragResult.TopDocument.ID,
				Content:  ragResult.TopDocument.Content,
				Metadata: ragResult.TopDocument.MetaData,
			})
			state.SetRAGDocuments(ragDocs)
			log.Printf("[Graph] RAG Top-1: score=%.4f, level=%s",
				ragResult.TopDocument.Score, rag.GetSimilarityLevel(ragResult.TopDocument.Score))

			// 使用检索到的文档生成回答
			answer = generateRAGAnswer(ctx, vg.llm, query, ragResult)
		} else {
			// 没有检索到文档，尝试使用选中的知识库信息生成回答
			log.Printf("[Graph] RAG no documents found, trying to use knowledge base info")
			if ragSelection := state.GetRAGSelection(); ragSelection != nil {
				answer = generateAnswerFromKnowledgeBases(ctx, vg.llm, state.OriginalQuery, ragSelection)
			} else {
				// 没有知识库信息，使用默认回答
				answer = generateRAGAnswer(ctx, vg.llm, query, ragResult)
			}
		}

		// 保存到 FinalAnswer
		state.FinalAnswer = answer

		// 【关键】保存到 AgentResults，让 Summary 节点能看到
		state.SetAgentResult(types.AgentTypeRAG, &types.AgentResult{
			AgentType: types.AgentTypeRAG,
			Content:   answer,
			ToolsUsed: []string{"rag_retrieval"},
		})

		return []*schema.Message{
			schema.AssistantMessage(answer, nil),
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

	// 添加 Report Agent 节点（使用标准 Lambda 封装）
	if vg.reportAgent != nil {
		reportLambda := vg.createAgentLambda(vg.reportAgent, types.AgentTypeReport, NodeReportAgent)
		_ = g.AddLambdaNode(NodeReportAgent, reportLambda)
	}

	// 添加创作分析 Agent 节点（使用标准 Lambda 封装）
	if vg.creativeAnalysisAgent != nil {
		creativeLambda := vg.createAgentLambda(vg.creativeAnalysisAgent, types.AgentTypeCreativeAnalysis, NodeCreativeAnalysisAgent)
		_ = g.AddLambdaNode(NodeCreativeAnalysisAgent, creativeLambda)
	}

	// 添加 RAG 知识库选择 Agent 节点（保留特殊处理逻辑）
	if vg.ragSelectorAgent != nil {
		_ = g.AddLambdaNode(NodeRAGSelectorAgent, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing RAG selector agent for query: %s", state.OriginalQuery)

			result, err := vg.ragSelectorAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] RAG selector agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("RAG知识库选择失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeRAGSelector, result)

			// 解析选择结果，设置到状态中
			if selection, parseErr := rag_selector.ParseSelectionResult(result.Content); parseErr == nil {
				state.SetRAGSelection(selection)
				// 保存优化后的查询
				state.SetOptimizedQuery(selection.Query)
				log.Printf("[Graph] RAG selected %d knowledge bases, optimized query: %s",
					len(selection.SelectedKBs), selection.Query)
			}

			nextAgent, err := vg.ragSelectorAgent.Route(ctx, state, result)
			log.Printf("[Graph] RAG selector agent route result: %s", nextAgent)

			return []*schema.Message{
				schema.AssistantMessage(result.Content, nil),
			}, nil
		}))
	}

	// 添加评论分析 Agent 节点（使用标准 Lambda 封装）
	if vg.commentAnalysisAgent != nil {
		commentLambda := vg.createAgentLambda(vg.commentAnalysisAgent, types.AgentTypeCommentAnalysis, NodeCommentAnalysisAgent)
		_ = g.AddLambdaNode(NodeCommentAnalysisAgent, commentLambda)
	}

	// 添加视频推荐 Agent 节点（使用标准 Lambda 封装）
	if vg.videoRecommendAgent != nil {
		recommendLambda := vg.createAgentLambda(vg.videoRecommendAgent, types.AgentTypeVideoRecommend, NodeVideoRecommendAgent)
		_ = g.AddLambdaNode(NodeVideoRecommendAgent, recommendLambda)
	}

	// 添加用户点赞视频 Agent 节点（使用标准 Lambda 封装）
	if vg.userLikedVideosAgent != nil {
		likedLambda := vg.createAgentLambda(vg.userLikedVideosAgent, types.AgentTypeUserLikedVideos, NodeUserLikedVideosAgent)
		_ = g.AddLambdaNode(NodeUserLikedVideosAgent, likedLambda)
	}

	// 添加热门视频 Agent 节点（使用标准 Lambda 封装）
	if vg.hotVideoAgent != nil {
		hotVideoLambda := vg.createAgentLambda(vg.hotVideoAgent, types.AgentTypeHotVideo, NodeHotVideoAgent)
		_ = g.AddLambdaNode(NodeHotVideoAgent, hotVideoLambda)
	}

	// 添加热门直播 Agent 节点（使用标准 Lambda 封装）
	if vg.hotLiveAgent != nil {
		hotLiveLambda := vg.createAgentLambda(vg.hotLiveAgent, types.AgentTypeHotLive, NodeHotLiveAgent)
		_ = g.AddLambdaNode(NodeHotLiveAgent, hotLiveLambda)
	}

	// 添加视频总结 Agent 节点（使用标准 Lambda 封装）
	if vg.videoSummaryAgent != nil {
		videoSummaryLambda := vg.createAgentLambda(vg.videoSummaryAgent, types.AgentTypeVideoSummary, NodeVideoSummaryAgent)
		_ = g.AddLambdaNode(NodeVideoSummaryAgent, videoSummaryLambda)
	}

	// 添加 Summary 节点，用于整合和格式化最终结果（必须在路由分支之前添加）
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

	// 使用标准 GraphBranch 进行意图路由
	_ = g.AddBranch(NodeTransList, compose.NewGraphBranch(
		func(ctx context.Context, msgs []*schema.Message) (string, error) {
			if len(msgs) == 0 {
				return compose.END, nil
			}
			content := strings.ToUpper(msgs[len(msgs)-1].Content)
			if strings.Contains(content, "REPORT") {
				return NodeReportAgent, nil
			}
			if strings.Contains(content, "CREATIVE") {
				return NodeCreativeAnalysisAgent, nil
			}
			if strings.Contains(content, "RAG") || strings.Contains(content, "知识库") {
				return NodeRAGSelectorAgent, nil
			}
			if strings.Contains(content, "COMMENTANALYSIS") || strings.Contains(content, "COMMENT_ANALYSIS") {
				return NodeCommentAnalysisAgent, nil
			}
			if strings.Contains(content, "VIDEORECOMMEND") || strings.Contains(content, "VIDEO_RECOMMEND") {
				return NodeVideoRecommendAgent, nil
			}
			if strings.Contains(content, "USERLIKEDVIDEOS") || strings.Contains(content, "USER_LIKED_VIDEOS") {
				return NodeUserLikedVideosAgent, nil
			}
			if strings.Contains(content, "HOTVIDEO") || strings.Contains(content, "HOT_VIDEO") {
				return NodeHotVideoAgent, nil
			}
			if strings.Contains(content, "HOTLIVE") || strings.Contains(content, "HOT_LIVE") {
				return NodeHotLiveAgent, nil
			}
			if strings.Contains(content, "VIDEOSUMMARY") || strings.Contains(content, "VIDEO_SUMMARY") {
				return NodeVideoSummaryAgent, nil
			}
			if strings.Contains(content, "CHAT") {
				return NodeSummary, nil
			}
			return NodeSummary, nil
		},
		map[string]bool{
			NodeReportAgent:           true,
			NodeCreativeAnalysisAgent: true,
			NodeRAGSelectorAgent:      true,
			NodeCommentAnalysisAgent:  true,
			NodeVideoRecommendAgent:   true,
			NodeUserLikedVideosAgent:  true,
			NodeHotVideoAgent:         true,
			NodeHotLiveAgent:          true,
			NodeVideoSummaryAgent:     true,
			NodeRAG:                   true,
			NodeSummary:               true,
		},
	))

	// 使用常量定义节点连接边
	_ = g.AddEdge(NodeReportAgent, NodeToToolCall)
	_ = g.AddEdge(NodeCreativeAnalysisAgent, NodeToToolCall)
	_ = g.AddEdge(NodeRAGSelectorAgent, NodeRAG)
	_ = g.AddEdge(NodeRAG, NodeSummary)
	_ = g.AddEdge(NodeCommentAnalysisAgent, NodeToToolCall)
	_ = g.AddEdge(NodeVideoRecommendAgent, NodeToToolCall)
	_ = g.AddEdge(NodeUserLikedVideosAgent, NodeToToolCall)
	_ = g.AddEdge(NodeHotVideoAgent, NodeToToolCall)
	_ = g.AddEdge(NodeHotLiveAgent, NodeToToolCall)
	_ = g.AddEdge(NodeVideoSummaryAgent, NodeToToolCall)

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

// generateRAGAnswer 使用 LLM 生成自然语言回答
func generateRAGAnswer(ctx context.Context, llm model.ChatModel, query string, ragResult *rag.RAGResult) string {
	const ragAnswerPrompt = `你是一个专业的知识库助手。请根据检索到的知识库内容回答用户的问题。

【回答规则】
1. 只使用提供的知识库内容回答，不要编造信息
2. 必须直接回答用户问题，不要绕弯子或给出模糊的"参考相关文档"之类的回答
3. 如果知识库内容包含功能介绍，请直接列出具体功能，不要概括性描述
4. 回答要简洁明了，突出核心信息，禁止出现"若需进一步信息"、"请参考相关文档"等推诿性语句

【强制要求】
- 用户问"这个网站是做什么的"或"有什么功能"时，必须直接列出网站的核心功能
- 禁止回答"主要涉及产品文档中的核心内容"这种模糊表述
- 必须从知识库内容中提取具体功能点并清晰呈现

【示例】
用户问：这个网站是做什么的？
知识库内容包含：视频观看、直播互动、内容创作
正确回答：VisionWorld是一个在线视频平台，主要功能包括：1. 视频观看 - 浏览各类创意视频；2. 直播互动 - 观看实时直播、发送弹幕、赠送礼物；3. 内容创作 - 创作者上传分享视频作品。

错误回答：该网站的功能介绍与使用说明主要涉及产品文档中的核心内容...`

	messages := []*schema.Message{
		schema.SystemMessage(ragAnswerPrompt),
	}

	// 添加检索上下文 - 使用所有检索到的文档，而不仅仅是 Top-1
	if ragResult != nil && ragResult.HasResult && len(ragResult.Documents) > 0 {
		var contextBuilder strings.Builder
		contextBuilder.WriteString("【知识库内容】\n")
		for i, doc := range ragResult.Documents {
			contextBuilder.WriteString(fmt.Sprintf("\n--- 文档 %d (相似度: %.2f) ---\n", i+1, doc.Score))
			contextBuilder.WriteString(doc.Content)
		}
		messages = append(messages, schema.SystemMessage(contextBuilder.String()))
	} else {
		messages = append(messages, schema.SystemMessage("【知识库内容】\n未找到相关信息。"))
	}

	// 添加用户问题
	messages = append(messages, schema.UserMessage(query))

	// 调用 LLM 生成回答
	resp, err := llm.Generate(ctx, messages)
	if err != nil {
		log.Printf("[Graph] RAG answer generation failed: %v", err)
		// 降级：直接返回文档内容或提示
		if ragResult != nil && ragResult.HasResult && ragResult.TopDocument != nil {
			return ragResult.TopDocument.Content
		}
		return "抱歉，我在知识库中没有找到相关信息。您可以尝试换一种方式提问。"
	}

	return resp.Content
}

// generateAnswerFromKnowledgeBases 当无法检索文档时，基于选中的知识库生成回答
func generateAnswerFromKnowledgeBases(
	ctx context.Context,
	llm model.ChatModel,
	query string,
	ragSelection interface{},
) string {
	// 解析选中的知识库
	selection, ok := ragSelection.(*rag_selector.RAGSelectionResult)
	if !ok {
		return "抱歉，暂时无法获取相关信息。"
	}

	// 【修复】当无法检索到真实文档时，不应该基于知识库元数据（名称和描述）生成具体回答
	// 因为元数据只是知识库的配置信息，不是真实的产品文档内容
	// 基于元数据生成回答会误导用户，让用户以为得到了准确信息

	// 记录日志用于调试
	var kbNames []string
	for _, kb := range selection.SelectedKBs {
		kbNames = append(kbNames, kb.Name)
	}
	log.Printf("[Graph] RAG retrieval failed, selected KBs: %v", kbNames)

	// 返回明确的提示信息，告知用户无法获取具体信息
	return "抱歉，我暂时无法获取相关文档内容来回答您的问题。可能原因：\n1. 知识库服务暂时不可用\n2. 相关文档尚未导入\n\n建议您：\n- 稍后再试\n- 联系管理员检查知识库配置"
}
