package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

func main() {
	// 创建gin引擎
	r := gin.Default()
	// API路由组
	api := r.Group("/api")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status": "healthy",
			})
		})

		api.GET("/video", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"message": "Video endpoints will be implemented here",
			})
		})
	}

	// 启动服务器
	r.Run(":8080") // 默认在8080端口监听
}
