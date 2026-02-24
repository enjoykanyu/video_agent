// Package mcp_server 提供MCP Server实现
package mcp_server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// VideoServer MCP视频服务Server
type VideoServer struct {
	sseServer  *server.SSEServer
	gatewayURL string
}

// NewVideoServer 创建视频MCP Server
func NewVideoServer(gatewayURL string) *VideoServer {
	// 创建MCP Server
	mcpServer := server.NewMCPServer(
		"video-agent-mcp",
		"1.0.0",
	)

	vs := &VideoServer{
		gatewayURL: gatewayURL,
	}

	// 注册工具
	vs.registerTools(mcpServer)

	// 创建SSE Server，使用 /mcp 前缀
	vs.sseServer = server.NewSSEServer(mcpServer,
		server.WithBasePath("/mcp"),
		server.WithSSEEndpoint("/sse"),
	)

	return vs
}

// registerTools 注册MCP工具
func (vs *VideoServer) registerTools(s *server.MCPServer) {
	// 注册获取视频工具
	videoTool := mcp.NewTool("get_video_by_id",
		mcp.WithDescription("通过视频ID获取视频的详细信息，包括标题、描述、播放量、点赞数等"),
		mcp.WithString("video_id",
			mcp.Required(),
			mcp.Description("视频的唯一标识ID"),
		),
	)

	s.AddTool(videoTool, vs.handleGetVideo)

	// 注册获取用户信息工具
	userTool := mcp.NewTool("get_user_info",
		mcp.WithDescription("获取用户的详细信息"),
		mcp.WithString("user_id",
			mcp.Required(),
			mcp.Description("用户的唯一标识ID"),
		),
	)

	s.AddTool(userTool, vs.handleGetUser)

	log.Printf("✅ [MCP Server] 注册工具完成")
}

// handleGetVideo 处理获取视频请求
func (vs *VideoServer) handleGetVideo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Printf("🛠️ [MCP Server] 工具被调用: get_video_by_id")

	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		log.Printf("❌ [MCP Server] 参数类型错误: %T", request.Params.Arguments)
		return nil, fmt.Errorf("invalid arguments type")
	}

	log.Printf("🛠️ [MCP Server] 工具参数: %+v", args)

	videoID, ok := args["video_id"].(string)
	if !ok || videoID == "" {
		return nil, fmt.Errorf("video_id参数不能为空")
	}

	log.Printf("🔧 [MCP Server] 获取视频 | VideoID: %s", videoID)

	// 调用Gateway获取视频信息
	video, err := vs.fetchVideoFromGateway(ctx, videoID)
	if err != nil {
		log.Printf("❌ [MCP Server] 获取视频失败: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("获取视频失败: %v", err)), nil
	}

	// 返回JSON结果
	resultJSON, _ := json.Marshal(video)
	log.Printf("✅ [MCP Server] 工具返回数据: %s", string(resultJSON))
	return mcp.NewToolResultJSON(resultJSON)
}

// handleGetUser 处理获取用户请求
func (vs *VideoServer) handleGetUser(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	log.Printf("🛠️ [MCP Server] 工具被调用: get_user_info")

	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		log.Printf("❌ [MCP Server] 参数类型错误: %T", request.Params.Arguments)
		return nil, fmt.Errorf("invalid arguments type")
	}

	log.Printf("🛠️ [MCP Server] 工具参数: %+v", args)

	userID, ok := args["user_id"].(string)
	if !ok || userID == "" {
		return nil, fmt.Errorf("user_id参数不能为空")
	}

	log.Printf("🔧 [MCP Server] 获取用户 | UserID: %s", userID)

	// 调用Gateway获取用户信息
	user, err := vs.fetchUserFromGateway(ctx, userID)
	if err != nil {
		log.Printf("❌ [MCP Server] 获取用户失败: %v", err)
		return mcp.NewToolResultError(fmt.Sprintf("获取用户失败: %v", err)), nil
	}

	// 返回JSON结果
	resultJSON, _ := json.Marshal(user)
	log.Printf("✅ [MCP Server] 工具返回数据: %s", string(resultJSON))
	return mcp.NewToolResultJSON(resultJSON)
}

// fetchVideoFromGateway 从Gateway获取视频信息
func (vs *VideoServer) fetchVideoFromGateway(ctx context.Context, videoID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/video/%s", vs.gatewayURL, videoID)
	log.Printf("🌐 [MCP Server] 请求Gateway: %s", url)

	// 创建HTTP客户端，设置超时
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求Gateway失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gateway返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var video map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&video); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	log.Printf("✅ [MCP Server] 获取视频成功: %s", videoID)
	return video, nil
}

// fetchUserFromGateway 从Gateway获取用户信息
func (vs *VideoServer) fetchUserFromGateway(ctx context.Context, userID string) (map[string]interface{}, error) {
	url := fmt.Sprintf("%s/api/user/%s", vs.gatewayURL, userID)
	log.Printf("🌐 [MCP Server] 请求Gateway: %s", url)

	// 创建HTTP客户端，设置超时
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// 创建请求
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求Gateway失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Gateway返回错误状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var user map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	log.Printf("✅ [MCP Server] 获取用户成功: %s", userID)
	return user, nil
}

// RegisterRoutes 注册Gin路由
func (vs *VideoServer) RegisterRoutes(r *gin.Engine) {
	// MCP SSE端点
	r.GET("/mcp/sse", gin.WrapH(vs.sseServer.SSEHandler()))

	// MCP消息端点
	r.POST("/mcp/message", gin.WrapH(vs.sseServer.MessageHandler()))

	// 健康检查
	r.GET("/mcp/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "ok",
			"server":  "video-agent-mcp",
			"version": "1.0.0",
		})
	})
}

// Start 启动MCP Server
func (vs *VideoServer) Start(addr string) error {
	log.Printf("🚀 [MCP Server] 启动 | 地址: %s", addr)
	return vs.sseServer.Start(addr)
}

// Shutdown 关闭MCP Server
func (vs *VideoServer) Shutdown(ctx context.Context) error {
	return vs.sseServer.Shutdown(ctx)
}
