package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"video_agent/internal/mcp"
)

// VideoAnalysisAgentV2 视频分析Agent V2 - 支持MCP工具调用
type VideoAnalysisAgentV2 struct {
	llm          model.ChatModel
	toolRegistry *mcp.Registry
}

// VideoAnalysisRequest 视频分析请求
type VideoAnalysisRequest struct {
	VideoID      string `json:"video_id"`
	VideoURL     string `json:"video_url,omitempty"`
	Query        string `json:"query"`         // 用户的分析问题
	AnalysisType string `json:"analysis_type"` // 分析类型: summary, content, sentiment, tags, all
}

// VideoAnalysisResponse 视频分析响应
type VideoAnalysisResponse struct {
	VideoID        string                 `json:"video_id"`
	Title          string                 `json:"title"`
	Description    string                 `json:"description"`
	Summary        string                 `json:"summary"`
	Content        string                 `json:"content"` // 详细内容分析
	Tags           []string               `json:"tags"`
	Sentiment      string                 `json:"sentiment"`
	KeyPoints      []string               `json:"key_points"`
	Suggestions    []string               `json:"suggestions"`
	RawData        map[string]interface{} `json:"raw_data"` // MCP工具返回的原始数据
	ProcessingTime int64                  `json:"processing_time_ms"`
}

// NewVideoAnalysisAgentV2 创建视频分析Agent V2
func NewVideoAnalysisAgentV2(llm model.ChatModel, toolRegistry *mcp.Registry) *VideoAnalysisAgentV2 {
	return &VideoAnalysisAgentV2{
		llm:          llm,
		toolRegistry: toolRegistry,
	}
}

// Analyze 分析视频 - 主入口
func (a *VideoAnalysisAgentV2) Analyze(ctx context.Context, req *VideoAnalysisRequest) (*VideoAnalysisResponse, error) {
	startTime := time.Now()
	log.Printf("🎬 [视频分析Agent] 开始分析视频 | VideoID: %s", req.VideoID)

	// 步骤1: 调用MCP工具获取视频信息
	videoInfo, err := a.getVideoInfoByMCP(ctx, req.VideoID)
	if err != nil {
		log.Printf("❌ [视频分析Agent] 获取视频信息失败: %v", err)
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}
	log.Printf("✅ [视频分析Agent] 获取视频信息成功 | video: %s", videoInfo)

	// 步骤2: 构建LLM提示词
	prompt := a.buildAnalysisPrompt(req, videoInfo)
	log.Printf("📝 [视频分析Agent] 构建分析提示词 | 长度: %d", len(prompt))
	log.Printf("📝 [视频分析Agent] 分析提示词 | %s", prompt)
	// 步骤3: 调用LLM进行深度分析
	analysisResult, err := a.callLLMForAnalysis(ctx, prompt, videoInfo)
	if err != nil {
		log.Printf("❌ [视频分析Agent] LLM分析失败: %v", err)
		return nil, fmt.Errorf("LLM分析失败: %w", err)
	}
	log.Printf("✅ [视频分析Agent] LLM分析完成")

	// 步骤4: 解析和组装结果
	response := a.assembleResponse(req.VideoID, videoInfo, analysisResult)
	response.ProcessingTime = time.Since(startTime).Milliseconds()
	response.RawData = videoInfo

	log.Printf("✅ [视频分析Agent] 分析完成 | 耗时: %dms", response.ProcessingTime)
	return response, nil
}

// getVideoInfoByMCP 通过MCP工具获取视频信息
func (a *VideoAnalysisAgentV2) getVideoInfoByMCP(ctx context.Context, videoID string) (map[string]interface{}, error) {
	log.Printf("🔧 [视频分析Agent] 调用MCP工具: GetVideoInfo | VideoID: %s", videoID)

	// 执行MCP工具调用
	params := map[string]interface{}{
		"video_id": videoID,
	}

	result, err := a.toolRegistry.Execute(ctx, "GetVideoInfo", params)
	if err != nil {
		// 如果工具不存在，使用模拟数据
		log.Printf("⚠️ [视频分析Agent] MCP工具未找到或执行失败，使用模拟数据: %v", err)
		return a.getMockVideoInfo(videoID), nil
	}

	// 解析结果
	videoInfo, ok := result.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("MCP工具返回格式错误")
	}

	log.Printf("✅ [视频分析Agent] MCP工具调用成功 | 返回字段: %v", videoInfo)
	return videoInfo, nil
}

// buildAnalysisPrompt 构建分析提示词
func (a *VideoAnalysisAgentV2) buildAnalysisPrompt(req *VideoAnalysisRequest, videoInfo map[string]interface{}) string {
	// 提取视频信息 - 支持float64和int64两种类型
	title, _ := videoInfo["title"].(string)
	description, _ := videoInfo["description"].(string)
	author, _ := videoInfo["author"].(string)
	tags, _ := videoInfo["tags"].([]interface{})

	// 数字字段可能是int64或float64，需要统一处理
	duration := getFloat64FromMap(videoInfo, "duration")
	viewCount := getFloat64FromMap(videoInfo, "view_count")
	likeCount := getFloat64FromMap(videoInfo, "like_count")

	// 构建标签字符串
	tagStr := ""
	for i, tag := range tags {
		if i > 0 {
			tagStr += ", "
		}
		tagStr += fmt.Sprintf("%v", tag)
	}

	prompt := fmt.Sprintf(`你是一位专业的视频内容分析师。请对以下视频进行深入分析。

## 视频基本信息
- 视频ID: %s
- 标题: %s
- 作者: %s
- 时长: %.0f秒
- 播放量: %.0f
- 点赞数: %.0f
- 标签: %s

## 视频简介
%s

## 用户的分析问题
%s

## 分析类型
%s

请提供以下分析内容：

1. **视频摘要** (200字以内): 概括视频核心内容
2. **详细内容分析**: 分析视频的结构、节奏、亮点
3. **情感倾向**: 判断视频整体情感 (positive/negative/neutral)
4. **关键要点** (3-5点): 列出视频的关键信息点
5. **标签建议** (5-8个): 基于内容推荐合适的标签
6. **优化建议** (2-3条): 针对视频内容的改进建议
7. **用户互动分析** (1-2条): 考虑用户互动（评论、点赞、分享）对视频成功的影响
请以JSON格式返回，格式如下:
{
  "summary": "视频摘要...",
  "content_analysis": "详细内容分析...",
  "sentiment": "positive",
  "key_points": ["要点1", "要点2", "要点3"],
  "suggested_tags": ["标签1", "标签2", "标签3"],
  "suggestions": ["建议1", "建议2"],
  "user_interaction_analysis": ["互动1", "互动2"]
}`,
		req.VideoID,
		title,
		author,
		duration,
		viewCount,
		likeCount,
		tagStr,
		description,
		req.Query,
		req.AnalysisType,
	)

	return prompt
}

// callLLMForAnalysis 调用LLM进行分析
func (a *VideoAnalysisAgentV2) callLLMForAnalysis(ctx context.Context, prompt string, videoInfo map[string]interface{}) (map[string]interface{}, error) {
	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一位专业的视频内容分析师，擅长从视频元数据中提取洞察并生成有价值的分析。请严格按照要求的JSON格式返回结果。",
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	response, err := a.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM生成失败: %w", err)
	}

	// 解析JSON响应
	result, err := a.parseLLMResponse(response.Content)
	if err != nil {
		log.Printf("⚠️ [视频分析Agent] LLM响应解析失败，使用原始内容: %v", err)
		// 返回简化结果
		return map[string]interface{}{
			"summary":          response.Content[:min(len(response.Content), 200)],
			"content_analysis": response.Content,
			"sentiment":        "neutral",
			"key_points":       []string{"分析完成"},
			"suggested_tags":   []string{"视频分析"},
			"suggestions":      []string{"请查看详细分析"},
		}, nil
	}

	return result, nil
}

// parseLLMResponse 解析LLM的JSON响应
func (a *VideoAnalysisAgentV2) parseLLMResponse(content string) (map[string]interface{}, error) {
	// 尝试提取JSON部分
	jsonPattern := regexp.MustCompile(`\{[\s\S]*\}`)
	match := jsonPattern.FindString(content)
	if match == "" {
		return nil, fmt.Errorf("未找到JSON内容")
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(match), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}

	return result, nil
}

// assembleResponse 组装最终响应
func (a *VideoAnalysisAgentV2) assembleResponse(videoID string, videoInfo, analysisResult map[string]interface{}) *VideoAnalysisResponse {
	// 提取视频基本信息
	title, _ := videoInfo["title"].(string)
	description, _ := videoInfo["description"].(string)

	// 提取分析结果
	summary, _ := analysisResult["summary"].(string)
	contentAnalysis, _ := analysisResult["content_analysis"].(string)
	sentiment, _ := analysisResult["sentiment"].(string)

	// 提取数组字段
	keyPoints := a.extractStringArray(analysisResult, "key_points")
	suggestedTags := a.extractStringArray(analysisResult, "suggested_tags")
	suggestions := a.extractStringArray(analysisResult, "suggestions")

	// 合并标签（原始标签 + 建议标签）
	originalTags := a.extractStringArray(videoInfo, "tags")
	allTags := append(originalTags, suggestedTags...)
	if len(allTags) > 10 {
		allTags = allTags[:10]
	}

	return &VideoAnalysisResponse{
		VideoID:     videoID,
		Title:       title,
		Description: description,
		Summary:     summary,
		Content:     contentAnalysis,
		Tags:        allTags,
		Sentiment:   sentiment,
		KeyPoints:   keyPoints,
		Suggestions: suggestions,
	}
}

// extractStringArray 从map中提取字符串数组
func (a *VideoAnalysisAgentV2) extractStringArray(data map[string]interface{}, key string) []string {
	var result []string
	if arr, ok := data[key].([]interface{}); ok {
		for _, item := range arr {
			if str, ok := item.(string); ok {
				result = append(result, str)
			}
		}
	}
	return result
}

// getMockVideoInfo 获取模拟视频信息（当MCP工具不可用时）
func (a *VideoAnalysisAgentV2) getMockVideoInfo(videoID string) map[string]interface{} {
	return map[string]interface{}{
		"video_id":    videoID,
		"title":       "示例视频标题 - " + videoID,
		"description": "这是一个示例视频的描述信息。视频内容丰富，包含多个精彩时刻。",
		"author":      "示例作者",
		"duration":    300.0,
		"view_count":  10000.0,
		"like_count":  500.0,
		"tags":        []string{"示例", "视频", "测试"},
		"created_at":  time.Now().Format(time.RFC3339),
	}
}

// RegisterMCPTools 注册MCP工具到注册表
func RegisterMCPTools(registry *mcp.Registry) {
	// 注册获取视频信息工具
	if err := registry.Register(&GetVideoByIDTool{}); err != nil {
		log.Printf("⚠️ [MCP] 注册工具失败: %v", err)
	} else {
		log.Printf("✅ [MCP] 注册工具: GetVideoInfo")
	}
}

// GetVideoByIDTool 通过ID获取视频信息工具
type GetVideoByIDTool struct{}

func (t *GetVideoByIDTool) Name() string {
	return "GetVideoInfo"
}

func (t *GetVideoByIDTool) Description() string {
	return "通过视频ID获取视频的详细信息，包括标题、描述、播放量、标签等"
}

func (t *GetVideoByIDTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"video_id": map[string]interface{}{
			"type":        "string",
			"description": "视频的唯一标识ID",
		},
	}
}

func (t *GetVideoByIDTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	videoID, ok := params["video_id"].(string)
	if !ok || videoID == "" {
		return nil, fmt.Errorf("video_id参数不能为空")
	}

	// TODO: 这里应该调用实际的视频服务API
	// 目前返回模拟数据
	return map[string]interface{}{
		"video_id":    videoID,
		"title":       "精彩视频 - " + videoID,
		"description": "这是一个非常精彩的视频，内容丰富，值得观看。",
		"author":      "优秀创作者",
		"duration":    600.0,
		"view_count":  50000.0,
		"like_count":  3000.0,
		"tags":        []string{"精彩", "热门", "推荐"},
		"created_at":  time.Now().Format(time.RFC3339),
	}, nil
}

// getFloat64FromMap 从map中获取float64值，支持int64和float64类型
func getFloat64FromMap(m map[string]interface{}, key string) float64 {
	if val, ok := m[key]; ok {
		switch v := val.(type) {
		case float64:
			return v
		case int64:
			return float64(v)
		case int:
			return float64(v)
		case float32:
			return float64(v)
		}
	}
	return 0
}
