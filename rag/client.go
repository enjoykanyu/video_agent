package rag

import (
	"context"
	"log"
	"time"

	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
)

var MilvusCli cli.Client

func init() {
	// 使用带超时的上下文，避免阻塞程序启动
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client, err := cli.NewClient(ctx, cli.Config{
		Address: "localhost:19530",
		DBName:  "eino_rag",
	})
	if err != nil {
		log.Printf("[RAG] Milvus 连接失败 (将在使用时重试): %v", err)
		return
	}
	MilvusCli = client
	log.Println("[RAG] Milvus 客户端初始化成功")
}
