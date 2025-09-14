package main

import (
	// "net/http"
	// agent
	// "github.com/gin-gonic/gin"
	"video_agent/agent"
)

func main() {
	// 创建gin引擎
	// r := gin.Default()
	//运行agent
	// agent.NewAgent()
	agent.Graph_agent()
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
