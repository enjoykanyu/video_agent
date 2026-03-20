package rag

import (
	"context"
	"log"
	"time"

	cli "github.com/milvus-io/milvus-sdk-go/v2/client"
)

var MilvusCli cli.Client

func EnsureMilvusConnected() error {
	if MilvusCli != nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := cli.NewClient(ctx, cli.Config{
		Address: "localhost:19530",
		DBName:  "eino_rag",
	})
	if err != nil {
		log.Printf("[RAG] Milvus 连接失败: %v", err)
		return err
	}

	MilvusCli = client
	log.Println("[RAG] Milvus 客户端连接成功")
	return nil
}

func init() {
	if err := EnsureMilvusConnected(); err != nil {
		log.Printf("[RAG] Milvus 初始连接失败，将在首次检索时重试: %v", err)
	}
}
