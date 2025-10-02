package rag

import (
	"context"
	"github.com/cloudwego/eino-ext/components/embedding/ollama"
	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"log"
	"time"
)

var collection = "test_rag"

var fields = []*entity.Field{
	{
		Name:     "id",
		DataType: entity.FieldTypeVarChar,
		TypeParams: map[string]string{
			"max_length": "255",
		},
		PrimaryKey: true,
	},
	{
		Name:     "vector",                    // 确保字段名匹配
		DataType: entity.FieldTypeFloatVector, // nomic-embed-text 返回浮点向量
		TypeParams: map[string]string{
			"dim": "768", // nomic-embed-text 正确维度
		},
	},
	{
		Name:     "content",
		DataType: entity.FieldTypeVarChar,
		TypeParams: map[string]string{
			"max_length": "8192",
		},
	},
	{
		Name:     "metadata",
		DataType: entity.FieldTypeJSON,
	},
}

func IndexerRAG(docs []*schema.Document) {
	ctx := context.Background()
	// 初始化嵌入器
	embedder, err := ollama.NewEmbedder(ctx, &ollama.EmbeddingConfig{
		BaseURL: "http://localhost:11434",
		Model:   "nomic-embed-text:v1.5",
		Timeout: 10 * time.Second,
	})
	if err != nil {
		panic(err)
	}

	indexer, err := milvus.NewIndexer(ctx, &milvus.IndexerConfig{
		Client:            MilvusCli,
		Collection:        collection,
		Fields:            fields,
		Embedding:         embedder,
		DocumentConverter: binaryDocumentConverter,
	})
	if err != nil {
		log.Fatalf("Failed to create indexer: %v", err)
	}
	for _, doc := range docs {
		storeDoc := []*schema.Document{
			{
				ID:       doc.ID,
				Content:  doc.Content,
				MetaData: doc.MetaData,
			},
		}
		ids, err := indexer.Store(ctx, storeDoc)
		if err != nil {
			log.Fatalf("Failed to store documents: %v", err)
		}
		println("Stored documents with IDs: %v", ids)
	}
}

func binaryDocumentConverter(ctx context.Context, docs []*schema.Document, vectors [][]float64) ([]interface{}, error) {
	rows := make([]interface{}, 0, len(docs))
	for i, doc := range docs {
		// 将 float64 向量转换为二进制向量
		binaryVec := make([]byte, len(vectors[i])/8)
		for j := 0; j < len(vectors[i]); j += 8 {
			var b byte
			for k := 0; k < 8 && j+k < len(vectors[i]); k++ {
				if vectors[i][j+k] > 0 {
					b |= 1 << k
				}
			}
			binaryVec[j/8] = b
		}

		row := map[string]interface{}{
			"id":       doc.ID,
			"content":  doc.Content,
			"vector":   binaryVec,
			"metadata": doc.MetaData,
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func floatDocumentConverter(ctx context.Context, docs []*schema.Document, vectors [][]float64) ([]interface{}, error) {
	rows := make([]interface{}, 0, len(docs))
	for i, doc := range docs {
		// float64 -> float32
		float32Vec := make([]float32, len(vectors[i]))
		for j, v := range vectors[i] {
			float32Vec[j] = float32(v)
		}
		row := map[string]interface{}{
			"id":       doc.ID,
			"content":  doc.Content,
			"vector":   float32Vec,
			"metadata": doc.MetaData,
		}
		rows = append(rows, row)
	}
	return rows, nil
}
