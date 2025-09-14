package main

import (
	"context"
	"video_agent/agent"
)

// "net/http"
// agent
// "github.com/gin-gonic/gin"

func main() {
	// 创建gin引擎
	// r := gin.Default()
	//运行agent
	// agent.NewAgent()
	// agent.Graph_agent()
	//运行有大模型的graph
	// agent.NewGraphWithModel()
	//运行有记忆的graph
	ctx := context.Background()
	agent.OrcGraphWithState(ctx, map[string]string{"role": "test1_role", "content": "你在哪"})
	// 基本路由
	// r.GET("/", func(c *gin.Context) {
	// 	c.JSON(http.StatusOK, gin.H{
	// 		"message": "Welcome to Video Agent API with Ollama",
	// 		"status":  "running",
	// 		"version": "1.0.0",
	// 	})
	// })

	// // API路由组
	// api := r.Group("/api")
	// {
	// 	api.GET("/hello", func(c *gin.Context) {
	// 		c.JSON(http.StatusOK, gin.H{
	// 			"status": "success",
	// 		})
	// 	})
	// }

	// // 启动服务器
	// r.Run(":8080") // 默认在8080端口监听
}
