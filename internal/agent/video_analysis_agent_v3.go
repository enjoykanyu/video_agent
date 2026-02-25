// Package agent 提供基于Eino ReAct Agent的视频分析功能

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"video_agent/internal/mcp"
)

// VideoAnalysisAgentV3 视频分析Agent V3 - 基于Eino ReAct Agent
type VideoAnalysisAgentV3 struct {
	llm         model.ChatModel
	mcpManager  *mcp.Manager
	agent       *react.Agent
	toolsCalled []string // 记录LLM调用的工具列表
}

// NewVideoAnalysisAgentV3 创建视频分析Agent V3
// 从MCP Manager获取所有工具，绑定到ReAct Agent
func NewVideoAnalysisAgentV3(llm model.ChatModel, mcpManager *mcp.Manager) (*VideoAnalysisAgentV3, error) {
	ctx := context.Background()

	// 1. 从MCP Manager获取所有可用工具
	tools, err := mcpManager.GetTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("从MCP获取工具失败: %w", err)
	}

	log.Printf("✅ [VideoAnalysisAgentV3] 加载 %d 个MCP工具", len(tools))
	for i, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			log.Printf("   [%d] 获取工具信息失败: %v", i, err)
			continue
		}
		log.Printf("   [%d] 工具名称: %s", i, info.Name)
		log.Printf("       描述: %s", info.Desc)
	}

	// 2. 创建ReAct Agent，绑定所有工具
	// LLM会根据用户输入自动选择合适的工具
	reactAgent, err := react.NewAgent(ctx, &react.AgentConfig{
		Model: llm,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools, // ← 绑定所有MCP工具，LLM自动选择
		},
		MaxStep: 3, // 限制最大步数为3步：1.决策 2.工具调用 3.生成回复（默认12步）
		// 配置流式工具调用检测器，解决流式模式下工具调用检测问题
		StreamToolCallChecker: func(ctx context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
			defer sr.Close()
			for {
				msg, err := sr.Recv()
				if err != nil {
					if errors.Is(err, io.EOF) {
						break
					}
					return false, err
				}
				if len(msg.ToolCalls) > 0 {
					log.Printf("🤖 [ReAct Agent] 检测到工具调用: %d 个", len(msg.ToolCalls))
					return true, nil
				}
			}
			return false, nil
		},
		MessageModifier: func(ctx context.Context, input []*schema.Message) []*schema.Message {
			// 添加系统提示词，指导LLM如何分析视频
			log.Printf("🤖 [ReAct Agent] MessageModifier 被调用，准备调用LLM")
			for i, msg := range input {
				log.Printf("🤖 [ReAct Agent] 输入消息[%d] role=%s, content=%s", i, msg.Role, truncateString(msg.Content, 100))
			}
			systemMsg := &schema.Message{
				Role:    schema.System,
				Content: getVideoAnalysisSystemPrompt(),
			}
			result := append([]*schema.Message{systemMsg}, input...)
			log.Printf("🤖 [ReAct Agent] 已添加系统提示词，共 %d 条消息", len(result))
			return result
		},
	})
	if err != nil {
		return nil, fmt.Errorf("创建ReAct Agent失败: %w", err)
	}

	return &VideoAnalysisAgentV3{
		llm:        llm,
		mcpManager: mcpManager,
		agent:      reactAgent,
	}, nil
}

// Analyze 分析视频 - 主入口
// 用户输入分析请求，Agent自动选择工具并生成分析报告
func (a *VideoAnalysisAgentV3) Analyze(ctx context.Context, videoID string, query string) (string, error) {
	startTime := time.Now()
	log.Printf("🎬 [VideoAnalysisAgentV3] 开始分析 | VideoID: %s | Query: %s", videoID, query)

	// 清空之前的工具调用记录
	a.toolsCalled = []string{}

	// 构建用户输入
	userInput := fmt.Sprintf("请分析视频 %s。用户的具体问题：%s", videoID, query)
	if query == "" {
		userInput = fmt.Sprintf("请对视频 %s 进行全面分析，包括内容摘要、情感倾向、关键要点和优化建议。", videoID)
	}

	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: userInput,
		},
	}

	// 调用ReAct Agent
	// Agent内部流程：
	// 1. LLM分析用户输入，决定调用哪些工具
	// 2. 调用选中的工具（通过MCP Server）
	// 3. 根据工具返回结果生成分析回复
	log.Printf("🤖 [VideoAnalysisAgentV3] ReAct Agent 开始执行...")
	log.Printf("🤖 [VideoAnalysisAgentV3] 用户消息: %s", userInput)
	log.Printf("🤖 [VideoAnalysisAgentV3] 系统提示词: %s", truncateString(getVideoAnalysisSystemPrompt(), 200))
	log.Printf("🤖 [VideoAnalysisAgentV3] 等待LLM决策是否调用工具...")

	response, err := a.agent.Generate(ctx, messages)
	if err != nil {
		log.Printf("❌ [VideoAnalysisAgentV3] Agent执行失败: %v", err)
		return "", fmt.Errorf("视频分析失败: %w", err)
	}

	elapsed := time.Since(startTime)
	log.Printf("✅ [VideoAnalysisAgentV3] 分析完成 | 耗时: %v | 回复长度: %d", elapsed, len(response.Content))
	log.Printf("� [VideoAnalysisAgentV3] LLM回复内容（前500字）:\n%s", truncateString(response.Content, 500))

	// 尝试从回复中检测是否使用了工具数据
	if containsToolDataReferences(response.Content) {
		log.Printf("✅ [VideoAnalysisAgentV3] 检测到回复中引用了工具数据")
	} else {
		log.Printf("⚠️ [VideoAnalysisAgentV3] 警告：回复中未检测到工具数据引用")
	}

	return response.Content, nil
}

// GetToolsCalled 获取LLM在最后一次分析中调用的工具列表
func (a *VideoAnalysisAgentV3) GetToolsCalled() []string {
	return a.toolsCalled
}

// StreamAnalyze 流式分析视频
func (a *VideoAnalysisAgentV3) StreamAnalyze(ctx context.Context, videoID string, query string) (*schema.StreamReader[*schema.Message], error) {
	log.Printf("🎬 [VideoAnalysisAgentV3] 开始流式分析 | VideoID: %s", videoID)

	userInput := fmt.Sprintf("请分析视频 %s。用户的具体问题：%s", videoID, query)
	if query == "" {
		userInput = fmt.Sprintf("请对视频 %s 进行全面分析。", videoID)
	}

	// 注意：ReAct Agent 已在初始化时配置 MessageModifier 自动添加系统提示词
	// 这里只需要提供用户输入
	messages := []*schema.Message{
		{
			Role:    schema.User,
			Content: userInput,
		},
	}

	log.Printf("🤖 [VideoAnalysisAgentV3] 流式调用ReAct Agent")
	log.Printf("🤖 [VideoAnalysisAgentV3] 用户输入: %s", userInput)
	startTime := time.Now()

	// 流式调用
	streamReader, err := a.agent.Stream(ctx, messages)
	if err != nil {
		log.Printf("❌ [VideoAnalysisAgentV3] ReAct Agent Stream 调用失败: %v", err)
		return nil, fmt.Errorf("流式分析失败: %w", err)
	}

	log.Printf("✅ [VideoAnalysisAgentV3] ReAct Agent Stream 调用成功，耗时: %v", time.Since(startTime))
	return streamReader, nil
}

// getVideoAnalysisSystemPrompt 获取视频分析系统提示词
func getVideoAnalysisSystemPrompt() string {
	// 		prompt := fmt.Sprintf(`你是一位专业的视频内容分析师。请对以下视频进行深入分析。

	// ## 视频基本信息
	// - 视频ID: %s
	// - 标题: %s
	// - 作者: %s
	// - 时长: %.0f秒
	// - 播放量: %.0f
	// - 点赞数: %.0f
	// - 标签: %s

	// ## 视频简介
	// %s

	// ## 用户的分析问题
	// %s

	// ## 分析类型
	// %s

	// 请提供以下分析内容：

	// 1. **视频摘要** (200字以内): 概括视频核心内容
	// 2. **详细内容分析**: 分析视频的结构、节奏、亮点
	// 3. **情感倾向**: 判断视频整体情感 (positive/negative/neutral)
	// 4. **关键要点** (3-5点): 列出视频的关键信息点
	// 5. **标签建议** (5-8个): 基于内容推荐合适的标签
	// 6. **优化建议** (2-3条): 针对视频内容的改进建议
	// 7. **用户互动分析** (1-2条): 考虑用户互动（评论、点赞、分享）对视频成功的影响
	// 请以JSON格式返回，格式如下:
	// {
	//   "summary": "视频摘要...",
	//   "content_analysis": "详细内容分析...",
	//   "sentiment": "positive",
	//   "key_points": ["要点1", "要点2", "要点3"],
	//   "suggested_tags": ["标签1", "标签2", "标签3"],
	//   "suggestions": ["建议1", "建议2"],
	//   "user_interaction_analysis": ["互动1", "互动2"]
	// }`,
	// 		req.VideoID,
	// 		title,
	// 		author,
	// 		duration,
	// 		viewCount,
	// 		likeCount,
	// 		tagStr,
	// 		description,
	// 		req.Query,
	// 		req.AnalysisType,
	// 	)
	return `你是一位专业的视频内容分析师，擅长深度分析视频内容。

**强制要求：**
1. **第一步：必须调用 get_video_by_id 工具**
   - 用户提供了视频ID，你必须先调用 get_video_by_id 工具获取视频的真实数据
   - 工具参数：{"video_id": "用户提供的视频ID"}
   - 等待工具返回数据后，再进行分析

2. **第二步：基于工具返回的真实数据分析**
   - 你只能使用工具返回的字段：title, description, author, view_count, like_count, comment_count, duration, tags
   - **严禁编造数据**
   - 如果工具调用失败，请明确告知用户"无法获取视频数据"

3. **分析内容：**
   - 内容摘要：基于 title 和 description
   - 数据洞察：使用真实的 view_count, like_count, comment_count
   - 情感倾向：基于内容判断
   - 关键要点：3-5个核心观点
   - 优化建议：如何改进视频内容

**工具信息：**
- 工具名称：get_video_by_id
- 功能：通过视频ID获取视频详细信息
- 必需参数：video_id (string)
- 返回字段：video_id, title, description, author, view_count, like_count, comment_count, duration, tags

**输出格式：**
1. 开头必须写："我已调用工具获取视频数据"
2. 分析中必须引用具体数据，例如："根据工具返回的数据，该视频标题为'XXX'，获得XXX次播放"
3. 如果未获取到数据，必须说明"未能获取视频数据，无法进行分析"`
}

// Close 关闭Agent
func (a *VideoAnalysisAgentV3) Close() error {
	if a.mcpManager != nil {
		return a.mcpManager.Close()
	}
	return nil
}

// VideoAnalysisResultV3 视频分析结果结构（V3版本）
type VideoAnalysisResultV3 struct {
	VideoID        string                 `json:"video_id"`
	Analysis       string                 `json:"analysis"`
	ToolsUsed      []string               `json:"tools_used"` // LLM调用了哪些工具
	RawData        map[string]interface{} `json:"raw_data"`   // 工具返回的原始数据
	ProcessingTime int64                  `json:"processing_time_ms"`
}

// AnalyzeWithDetail 详细分析（返回结构化结果）
// 注意：V3版本使用ReAct Agent，工具由LLM动态选择，具体调用了哪些工具由Agent内部管理
func (a *VideoAnalysisAgentV3) AnalyzeWithDetail(ctx context.Context, videoID string, query string) (*VideoAnalysisResultV3, error) {
	startTime := time.Now()

	analysis, err := a.Analyze(ctx, videoID, query)
	if err != nil {
		return nil, err
	}

	// V3版本中，工具由LLM动态选择，这里记录为"dynamic"表示动态选择
	// 实际调用的工具列表需要通过Agent的回调或日志获取
	// 获取LLM实际调用的工具列表
	toolsUsed := a.GetToolsCalled()
	if len(toolsUsed) == 0 {
		toolsUsed = []string{"unknown"} // 如果无法获取，标记为unknown
	}

	return &VideoAnalysisResultV3{
		VideoID:        videoID,
		Analysis:       analysis,
		ToolsUsed:      toolsUsed, // LLM动态选择的工具列表
		ProcessingTime: time.Since(startTime).Milliseconds(),
	}, nil
}

// 辅助函数：解析工具调用结果
func parseToolResultV3(result interface{}) (map[string]interface{}, error) {
	switch v := result.(type) {
	case map[string]interface{}:
		return v, nil
	case string:
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(v), &data); err != nil {
			return nil, err
		}
		return data, nil
	default:
		return nil, fmt.Errorf("未知的结果类型: %T", result)
	}
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// containsToolDataReferences 检查回复中是否包含工具数据引用
func containsToolDataReferences(content string) bool {
	// 检查是否包含常见的工具数据字段引用
	keywords := []string{"播放量", "view_count", "点赞数", "like_count", "评论数", "comment_count",
		"视频标题", "title", "视频描述", "description", "作者", "author"}
	for _, keyword := range keywords {
		if strings.Contains(content, keyword) {
			return true
		}
	}
	return false
}
