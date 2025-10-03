package rag

import (
	"context"
	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"time"

	"github.com/cloudwego/eino-ext/components/retriever/milvus"
	"github.com/cloudwego/eino/schema"
)

func RetrieverRAG(query string) []*schema.Document {
	ctx := context.Background()
	// 初始化嵌入器
	embedder, err := ollama.NewEmbedder(ctx, &ollama.EmbeddingConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3-embedding:0.6b",
		Timeout: 10 * time.Second,
	})
	if err != nil {
		panic(err)
	}
	retriever, err := milvus.NewRetriever(ctx, &milvus.RetrieverConfig{
		Client:      MilvusCli,
		Collection:  "test_index",
		Partition:   nil,
		VectorField: "vector",
		OutputFields: []string{
			"id",
			"content",
			"metadata",
		},
		TopK:      1,
		Embedding: embedder,
	})
	if err != nil {
		panic(err)
	}

	results, err := retriever.Retrieve(ctx, query)
	if err != nil {
		panic(err)
	}
	// 打印检索结果
	println("检索到的文档数量:", len(results))
	for i, doc := range results {
		println("文档", i+1, ":")
		println("  ID:", doc.ID)
		println("  内容:", doc.Content)
		println("  元数据:", doc.MetaData)
	}
	return results
}
