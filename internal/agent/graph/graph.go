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
			schema.SystemMessage(`你是一个意图识别专家。请严格按规则判断用户意图，只输出意图类型，不要解释：

【判断优先级 - 从上到下依次检查，匹配到立即停止】

最高优先级 - RAG（知识库查询）：
如果用户查询包含以下任何内容，立即回答 'RAG'：
- "网站" + "干啥/干什么/是什么/功能" → RAG
- "系统" + "干啥/干什么/是什么/功能" → RAG  
- "产品" + "干啥/干什么/是什么/功能" → RAG
- "怎么用"、"如何使用"、"有什么功能"
- "查找资料"、"搜索文档"、"查询知识库"
- "介绍" + 产品/网站/系统名称

第二优先级 - VideoSummary（视频内容总结）：
- "视频总结"、"视频内容"、"这个视频讲什么"、"总结一下视频" → VideoSummary

第三优先级 - CommentAnalysis（评论分析）：
- "评论"、"弹幕"、"观众反馈"、"评论分析"、"弹幕分析" → CommentAnalysis

第四优先级 - VideoRecommend（视频推荐）：
- "推荐"、"推荐视频"、"有什么好看的"、"感兴趣的视频"、"给我推荐点视频" → VideoRecommend

第五优先级 - UserLikedVideos（点赞查询）：
- "我点赞过"、"我赞过的"、"我收藏的视频"、"点赞记录"、"我喜欢的视频" → UserLikedVideos

第六优先级 - HotVideo（最火视频）：
- "最火视频"、"最热门视频"、"爆款视频"、"热搜视频"、"大家都在看什么" → HotVideo

第七优先级 - HotLive（最火直播）：
- "最火直播"、"热门直播"、"正在直播"、"直播推荐"、"有什么好看的直播" → HotLive

第八优先级 - Report（视频数据分析）：
- "视频数据"、"视频统计"、"视频报表"、"生成报告"、"分析视频" → Report

第九优先级 - Creative（创作分析）：
- "选题"、"热门"、"趋势"、"创作方向"、"竞品分析"、"什么选题最火" → Creative

最低优先级 - Chat（闲聊）：
- 问候语、日常对话、不涉及上述任何功能的问题 → Chat

【重要示例】
- "visionWorld网站干啥的" → RAG（包含"网站"+"干啥"）
- "这个系统怎么用" → RAG（包含"系统"+"怎么用"）
- "介绍一下产品功能" → RAG（包含"介绍"+"功能"）
- "帮我分析视频123的评论" → CommentAnalysis
- "给我推荐一些好看的视频" → VideoRecommend
- "最近什么视频最火" → HotVideo
- "帮我总结一下视频456的内容" → VideoSummary

【输出格式】
只输出一个单词：Report / Creative / RAG / Chat / CommentAnalysis / VideoRecommend / UserLikedVideos / HotVideo / HotLive / VideoSummary

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
		// 使用余弦相似度，阈值 0.75
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

	// 添加RAG知识库选择Agent节点
	if vg.ragSelectorAgent != nil {
		_ = g.AddLambdaNode("rag_selector_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
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

	// 添加评论分析Agent节点
	if vg.commentAnalysisAgent != nil {
		_ = g.AddLambdaNode("comment_analysis_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing comment analysis agent for query: %s", state.OriginalQuery)

			result, err := vg.commentAnalysisAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] comment analysis agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("评论分析执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeCommentAnalysis, result)

			nextAgent, err := vg.commentAnalysisAgent.Route(ctx, state, result)
			log.Printf("[Graph] comment analysis agent route result: %s", nextAgent)

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

	// 添加视频推荐Agent节点
	if vg.videoRecommendAgent != nil {
		_ = g.AddLambdaNode("video_recommend_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing video recommend agent for query: %s", state.OriginalQuery)

			result, err := vg.videoRecommendAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] video recommend agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("视频推荐执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeVideoRecommend, result)

			nextAgent, err := vg.videoRecommendAgent.Route(ctx, state, result)
			log.Printf("[Graph] video recommend agent route result: %s", nextAgent)

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

	// 添加用户点赞视频Agent节点
	if vg.userLikedVideosAgent != nil {
		_ = g.AddLambdaNode("user_liked_videos_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing user liked videos agent for query: %s", state.OriginalQuery)

			result, err := vg.userLikedVideosAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] user liked videos agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("点赞视频查询执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeUserLikedVideos, result)

			nextAgent, err := vg.userLikedVideosAgent.Route(ctx, state, result)
			log.Printf("[Graph] user liked videos agent route result: %s", nextAgent)

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

	// 添加热门视频Agent节点
	if vg.hotVideoAgent != nil {
		_ = g.AddLambdaNode("hot_video_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing hot video agent for query: %s", state.OriginalQuery)

			result, err := vg.hotVideoAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] hot video agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("热门视频查询执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeHotVideo, result)

			nextAgent, err := vg.hotVideoAgent.Route(ctx, state, result)
			log.Printf("[Graph] hot video agent route result: %s", nextAgent)

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

	// 添加热门直播Agent节点
	if vg.hotLiveAgent != nil {
		_ = g.AddLambdaNode("hot_live_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing hot live agent for query: %s", state.OriginalQuery)

			result, err := vg.hotLiveAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] hot live agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("热门直播查询执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeHotLive, result)

			nextAgent, err := vg.hotLiveAgent.Route(ctx, state, result)
			log.Printf("[Graph] hot live agent route result: %s", nextAgent)

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

	// 添加视频总结Agent节点
	if vg.videoSummaryAgent != nil {
		_ = g.AddLambdaNode("video_summary_agent", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *states.GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *states.GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			log.Printf("[Graph] executing video summary agent for query: %s", state.OriginalQuery)

			result, err := vg.videoSummaryAgent.Execute(ctx, state)
			if err != nil {
				log.Printf("[Graph] video summary agent error: %v", err)
				return []*schema.Message{
					schema.AssistantMessage(fmt.Sprintf("视频总结执行失败: %v", err), nil),
				}, nil
			}

			state.SetAgentResult(types.AgentTypeVideoSummary, result)

			nextAgent, err := vg.videoSummaryAgent.Route(ctx, state, result)
			log.Printf("[Graph] video summary agent route result: %s", nextAgent)

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
			if strings.Contains(content, "RAG") || strings.Contains(content, "知识库") {
				return "rag_selector_agent", nil
			}
			if strings.Contains(content, "COMMENTANALYSIS") || strings.Contains(content, "COMMENT_ANALYSIS") {
				return "comment_analysis_agent", nil
			}
			if strings.Contains(content, "VIDEORECOMMEND") || strings.Contains(content, "VIDEO_RECOMMEND") {
				return "video_recommend_agent", nil
			}
			if strings.Contains(content, "USERLIKEDVIDEOS") || strings.Contains(content, "USER_LIKED_VIDEOS") {
				return "user_liked_videos_agent", nil
			}
			if strings.Contains(content, "HOTVIDEO") || strings.Contains(content, "HOT_VIDEO") {
				return "hot_video_agent", nil
			}
			if strings.Contains(content, "HOTLIVE") || strings.Contains(content, "HOT_LIVE") {
				return "hot_live_agent", nil
			}
			if strings.Contains(content, "VIDEOSUMMARY") || strings.Contains(content, "VIDEO_SUMMARY") {
				return "video_summary_agent", nil
			}
			if strings.Contains(content, "CHAT") {
				return NodeSummary, nil
			}
			return NodeSummary, nil
		},
		map[string]bool{
			"report_agent":            true,
			"creative_analysis_agent": true,
			"rag_selector_agent":      true,
			"comment_analysis_agent":  true,
			"video_recommend_agent":   true,
			"user_liked_videos_agent": true,
			"hot_video_agent":         true,
			"hot_live_agent":          true,
			"video_summary_agent":     true,
			NodeRAG:                   true,
			NodeSummary:               true,
		},
	))

	_ = g.AddEdge("report_agent", NodeToToolCall)
	_ = g.AddEdge("creative_analysis_agent", NodeToToolCall)
	_ = g.AddEdge("rag_selector_agent", NodeRAG)
	_ = g.AddEdge(NodeRAG, NodeSummary)
	_ = g.AddEdge("comment_analysis_agent", NodeToToolCall)
	_ = g.AddEdge("video_recommend_agent", NodeToToolCall)
	_ = g.AddEdge("user_liked_videos_agent", NodeToToolCall)
	_ = g.AddEdge("hot_video_agent", NodeToToolCall)
	_ = g.AddEdge("hot_live_agent", NodeToToolCall)
	_ = g.AddEdge("video_summary_agent", NodeToToolCall)

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
2. 如果知识库内容与问题相关，请给出清晰、自然的回答
3. 如果知识库内容为空或不相关，请礼貌地告知用户未找到相关信息
4. 回答要简洁明了，直接回答问题`

	messages := []*schema.Message{
		schema.SystemMessage(ragAnswerPrompt),
	}

	// 添加检索上下文
	if ragResult != nil && ragResult.HasResult && ragResult.TopDocument != nil {
		contextMsg := fmt.Sprintf("【知识库内容】\n%s", ragResult.TopDocument.Content)
		messages = append(messages, schema.SystemMessage(contextMsg))
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
