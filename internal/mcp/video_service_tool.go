package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

// VideoServiceConfig 视频服务配置
type VideoServiceConfig struct {
	BaseURL     string        // 视频服务基础URL，如 "http://video-service:8080"
	Timeout     time.Duration // 超时时间
	APIKey      string        // API密钥（如果需要）
	EnableCache bool          // 是否启用缓存
}

// VideoServiceTool 真实的视频服务MCP工具
type VideoServiceTool struct {
	config VideoServiceConfig
	client *http.Client
}

// VideoInfo 视频信息结构
type VideoInfo struct {
	VideoID       string   `json:"video_id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Author        Author   `json:"author"`
	Duration      int      `json:"duration"` // 秒
	ViewCount     int64    `json:"view_count"`
	LikeCount     int64    `json:"like_count"`
	CoinCount     int64    `json:"coin_count"`
	FavoriteCount int64    `json:"favorite_count"`
	ShareCount    int64    `json:"share_count"`
	Tags          []string `json:"tags"`
	Category      string   `json:"category"`
	CoverURL      string   `json:"cover_url"`
	VideoURL      string   `json:"video_url"`
	CreatedAt     string   `json:"created_at"`
	UpdatedAt     string   `json:"updated_at"`
}

// Author 作者信息
type Author struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Avatar    string `json:"avatar"`
	Followers int64  `json:"followers"`
}

// NewVideoServiceTool 创建真实的视频服务工具
func NewVideoServiceTool(config VideoServiceConfig) *VideoServiceTool {
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &VideoServiceTool{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Name 工具名称
func (t *VideoServiceTool) Name() string {
	return "GetVideoInfo"
}

// Description 工具描述
func (t *VideoServiceTool) Description() string {
	return "通过视频ID从视频服务API获取视频的详细信息"
}

// Parameters 参数定义
func (t *VideoServiceTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"video_id": map[string]interface{}{
			"type":        "string",
			"description": "视频的唯一标识ID，如BV号或av号",
		},
	}
}

// Execute 执行工具调用 - 真实HTTP调用
func (t *VideoServiceTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	videoID, ok := params["video_id"].(string)
	if !ok || videoID == "" {
		return nil, fmt.Errorf("video_id参数不能为空")
	}

	log.Printf("🔧 [VideoServiceTool] 调用视频服务API | VideoID: %s", videoID)

	// 调用真实的视频服务API
	videoInfo, err := t.callVideoAPI(ctx, videoID)
	if err != nil {
		log.Printf("❌ [VideoServiceTool] 调用失败: %v", err)
		return nil, fmt.Errorf("调用视频服务失败: %w", err)
	}

	log.Printf("✅ [VideoServiceTool] 调用成功 | Title: %s", videoInfo.Title)

	// 转换为map返回
	return t.toMap(videoInfo), nil
}

// callVideoAPI 调用视频服务API
func (t *VideoServiceTool) callVideoAPI(ctx context.Context, videoID string) (*VideoInfo, error) {
	// 构建请求URL
	url := fmt.Sprintf("%s/api/v1/video/%s", t.config.BaseURL, videoID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 添加请求头
	req.Header.Set("Accept", "application/json")
	if t.config.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.config.APIKey)
	}

	// 发送请求
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("视频服务返回错误状态码: %d", resp.StatusCode)
	}

	// 解析响应
	var result struct {
		Code    int       `json:"code"`
		Message string    `json:"message"`
		Data    VideoInfo `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("视频服务返回错误: %s", result.Message)
	}

	return &result.Data, nil
}

// toMap 将VideoInfo转换为map
func (t *VideoServiceTool) toMap(info *VideoInfo) map[string]interface{} {
	return map[string]interface{}{
		"video_id":       info.VideoID,
		"title":          info.Title,
		"description":    info.Description,
		"author":         info.Author.Name,
		"author_id":      info.Author.ID,
		"duration":       info.Duration,
		"view_count":     info.ViewCount,
		"like_count":     info.LikeCount,
		"coin_count":     info.CoinCount,
		"favorite_count": info.FavoriteCount,
		"share_count":    info.ShareCount,
		"tags":           info.Tags,
		"category":       info.Category,
		"cover_url":      info.CoverURL,
		"video_url":      info.VideoURL,
		"created_at":     info.CreatedAt,
		"updated_at":     info.UpdatedAt,
	}
}

// ==================== 对比：普通函数调用 vs MCP调用 ====================

// NormalFunctionCall 普通函数调用示例
func NormalFunctionCall(videoID string) (*VideoInfo, error) {
	// 直接调用，没有协议层
	// 耦合度高，不利于扩展
	return getVideoFromDatabase(videoID)
}

// MCPFunctionCall MCP协议调用示例
func MCPFunctionCall(ctx context.Context, registry *Registry, videoID string) (map[string]interface{}, error) {
	// 通过MCP协议层调用
	// 解耦，支持动态发现和LLM集成
	params := map[string]interface{}{
		"video_id": videoID,
	}
	result, err := registry.Execute(ctx, "GetVideoInfo", params)
	if err != nil {
		return nil, err
	}
	return result.(map[string]interface{}), nil
}

// getVideoFromDatabase 模拟从数据库获取
func getVideoFromDatabase(videoID string) (*VideoInfo, error) {
	// 实际实现...
	return nil, nil
}
