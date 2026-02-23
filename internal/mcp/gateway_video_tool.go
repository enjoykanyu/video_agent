package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

// GatewayVideoTool 调用Gateway获取视频信息的MCP工具
type GatewayVideoTool struct {
	gatewayBaseURL string // Gateway地址，如 "http://localhost:8080"
	httpClient     *http.Client
	apiKey         string // 如果Gateway需要认证
}

// GatewayVideoResponse Gateway返回的视频数据结构
// 适配Gateway实际返回: {"code":0,"message":"success","data":{"video":{...}}}
type GatewayVideoResponse struct {
	Code    int                  `json:"code"`
	Message string               `json:"message"`
	Data    *GatewayVideoWrapper `json:"data"`
}

// GatewayVideoWrapper data字段的包装层
type GatewayVideoWrapper struct {
	Video *VideoData `json:"video"`
}

// VideoData 视频数据结构（根据你的Gateway实际结构调整）
// 字段名使用JSON标签匹配Gateway返回的字段名
type VideoData struct {
	VideoID     int64    `json:"video_id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	AuthorID    int64    `json:"author_id"`
	AuthorName  string   `json:"username"` // Gateway返回的是username
	Duration    int      `json:"duration"`
	ViewCount   int64    `json:"view_count"`
	LikeCount   int64    `json:"like_count"`
	Tags        []string `json:"tags"`
	CoverURL    string   `json:"cover_url"`
	VideoURL    string   `json:"video_url"`
	CreatedAt   int64    `json:"create_time"` // Gateway返回的是create_time
	Status      string   `json:"status"`
}

// NewGatewayVideoTool 创建Gateway视频工具
// gatewayURL: Gateway的HTTP地址，如 "http://localhost:8080"
func NewGatewayVideoTool(gatewayURL string) *GatewayVideoTool {
	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080" // 默认地址
	}
	return &GatewayVideoTool{
		gatewayBaseURL: gatewayURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewGatewayVideoToolWithURL 从环境变量或配置创建
func NewGatewayVideoToolWithURL() *GatewayVideoTool {
	// 可以从环境变量读取
	gatewayURL := "http://localhost:8080"
	return NewGatewayVideoTool(gatewayURL)
}

// NewGatewayVideoToolWithAuth 创建带认证的Gateway视频工具
func NewGatewayVideoToolWithAuth(gatewayURL, apiKey string) *GatewayVideoTool {
	tool := NewGatewayVideoTool(gatewayURL)
	tool.apiKey = apiKey
	return tool
}

// Name 工具名称
func (t *GatewayVideoTool) Name() string {
	return "GetVideoInfo"
}

// Description 工具描述
func (t *GatewayVideoTool) Description() string {
	return "通过视频ID从Gateway服务获取视频的详细信息"
}

// Parameters 参数定义
func (t *GatewayVideoTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"video_id": map[string]interface{}{
			"type":        "string",
			"description": "视频的唯一标识ID",
		},
	}
}

// Execute 执行工具调用 - 真实HTTP调用Gateway
func (t *GatewayVideoTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	videoID, ok := params["video_id"].(string)
	if !ok || videoID == "" {
		return nil, fmt.Errorf("video_id参数不能为空")
	}

	log.Printf("🔧 [GatewayVideoTool] 调用Gateway获取视频 | VideoID: %s | Gateway: %s", videoID, t.gatewayBaseURL)

	// 调用Gateway的HTTP接口
	videoData, err := t.callGatewayAPI(ctx, videoID)
	if err != nil {
		log.Printf("❌ [GatewayVideoTool] Gateway调用失败: %v", err)
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	if videoData == nil {
		return nil, fmt.Errorf("视频不存在: %s", videoID)
	}

	log.Printf("✅ [GatewayVideoTool] Gateway调用成功 | Title: %s", videoData.Title)

	// 转换为标准map返回
	return t.toMap(videoData), nil
}

// callGatewayAPI 调用Gateway的HTTP接口
// 适配 Gateway: func (h *VideoHandler) GetVideoDetail(c *gin.Context)
// 路由: GET /api/video/:id (id为uint64)
func (t *GatewayVideoTool) callGatewayAPI(ctx context.Context, videoID string) (*VideoData, error) {
	// 问题1: Gateway期望uint64类型的ID，但传入的可能是BV号或字符串
	// 尝试将videoID转换为uint64
	var numericID uint64
	var err error

	// 如果是纯数字字符串，直接转换
	if numericID, err = strconv.ParseUint(videoID, 10, 64); err != nil {
		// 如果不是纯数字（如BV号），需要映射或报错
		// 方案A: 使用字符串作为ID（如果Gateway支持）
		// 方案B: 通过其他服务将BV号映射为数字ID
		log.Printf("⚠️ [GatewayVideoTool] VideoID不是数字格式: %s，尝试直接使用字符串", videoID)
		// 这里我们直接使用原始字符串，让Gateway处理
		numericID = 0
	}

	// 构建请求URL
	// 根据你的Gateway实际路由: /api/video/:id
	var url string
	if numericID > 0 {
		url = fmt.Sprintf("%s/api/video/%d", t.gatewayBaseURL, numericID)
	} else {
		// 如果转换失败，使用字符串格式（需要Gateway支持）
		url = fmt.Sprintf("%s/api/video/%s", t.gatewayBaseURL, videoID)
	}

	log.Printf("🔧 [GatewayVideoTool] 请求URL: %s", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 添加请求头
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	// 如果有API Key，添加认证头
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	// 发送请求
	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Gateway返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应 - 适配Gateway的响应格式
	log.Printf("🔧 [GatewayVideoTool] 解析响应体: %s", string(body))

	// Gateway返回: { "code": 0, "message": "success", "data": { "video": {...} } }
	var gatewayResp GatewayVideoResponse
	if err := json.Unmarshal(body, &gatewayResp); err == nil {
		log.Printf("🔧 [GatewayVideoTool] 解析为包装格式 | Code: %d, Message: %s",
			gatewayResp.Code, gatewayResp.Message)
		if gatewayResp.Code == 0 || gatewayResp.Code == 200 {
			if gatewayResp.Data != nil && gatewayResp.Data.Video != nil {
				log.Printf("✅ [GatewayVideoTool] 成功解析视频数据 | VideoID: %d, Title: %s",
					gatewayResp.Data.Video.VideoID, gatewayResp.Data.Video.Title)
				return gatewayResp.Data.Video, nil
			}
			log.Printf("⚠️ [GatewayVideoTool] 包装格式中data.video为空")
		}
	} else {
		log.Printf("🔧 [GatewayVideoTool] 解析包装格式失败: %v", err)
	}

	// 如果包装格式解析失败，尝试直接解析为VideoData
	var directData VideoData
	if err := json.Unmarshal(body, &directData); err != nil {
		log.Printf("❌ [GatewayVideoTool] 直接解析失败: %v", err)
		return nil, fmt.Errorf("解析响应失败: %w, 响应: %s", err, string(body))
	}

	log.Printf("🔧 [GatewayVideoTool] 直接解析结果 | VideoID: %d, Title: %s", directData.VideoID, directData.Title)
	return &directData, nil
}

// getMapKeys 获取map的所有key（用于调试）
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// toMap 将VideoData转换为map（与之前保持一致）
func (t *GatewayVideoTool) toMap(data *VideoData) map[string]interface{} {
	return map[string]interface{}{
		"video_id":    data.VideoID,
		"title":       data.Title,
		"description": data.Description,
		"author_id":   data.AuthorID,
		"author":      data.AuthorName,
		"duration":    data.Duration,
		"view_count":  data.ViewCount,
		"like_count":  data.LikeCount,
		"tags":        data.Tags,
		"cover_url":   data.CoverURL,
		"video_url":   data.VideoURL,
		"created_at":  data.CreatedAt,
		"status":      data.Status,
	}
}

// ==================== 使用示例 ====================

// ExampleUsage 使用示例
// func ExampleUsage() {
// 	// 方式1: 创建Gateway视频工具
// 	tool := NewGatewayVideoTool("http://localhost:8080")

// 	// 方式2: 如果Gateway需要认证
// 	// tool := NewGatewayVideoToolWithAuth("http://localhost:8080", "your-api-key")

// 	// 注册到MCP Registry
// 	registry := NewRegistry()
// 	if err := registry.Register(tool); err != nil {
// 		log.Printf("注册工具失败: %v", err)
// 		return
// 	}

// 	// 在Agent中使用
// 	ctx := context.Background()
// 	result, err := registry.Execute(ctx, "get_video_by_id", map[string]interface{}{
// 		"video_id": "123", // 使用数字ID
// 	})
// 	if err != nil {
// 		log.Printf("执行失败: %v", err)
// 		return
// 	}

// 	log.Printf("结果: %+v", result)
// }
