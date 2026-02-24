// Package main MCP Server 启动入口
// 提供视频分析相关的 MCP 工具服务
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"video_agent/mcp_server"
)

func main() {
	log.Println("🚀 [MCP Server] 正在启动...")

	// 获取配置
	gatewayURL := getEnv("GATEWAY_URL", "http://localhost:8080")
	mcpPort := getEnv("MCP_PORT", "8081")

	log.Printf("📋 [MCP Server] 配置信息:")
	log.Printf("   - Gateway地址: %s", gatewayURL)
	log.Printf("   - MCP服务端口: %s", mcpPort)

	// 创建 MCP Server
	videoServer := mcp_server.NewVideoServer(gatewayURL)

	// 启动 MCP Server（使用内置SSE服务器）
	go func() {
		log.Printf("🚀 [MCP Server] 启动SSE服务器 | 地址: :%s", mcpPort)
		if err := videoServer.Start(":" + mcpPort); err != nil {
			log.Fatalf("❌ [MCP Server] 启动失败: %v", err)
		}
	}()

	// 等待中断信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("\n🛑 [MCP Server] 收到关闭信号，正在优雅关闭...")

	// 创建关闭上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 关闭 MCP Server
	if err := videoServer.Shutdown(ctx); err != nil {
		log.Printf("⚠️ [MCP Server] 关闭出错: %v", err)
	}

	log.Println("✅ [MCP Server] 已关闭")
}

// getEnv 获取环境变量，如果不存在返回默认值
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
