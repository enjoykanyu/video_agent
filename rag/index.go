package rag

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/cloudwego/eino-ext/components/indexer/milvus"
	"github.com/cloudwego/eino/schema"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

var collection = "website_kb"

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
		Name:     "vector",
		DataType: entity.FieldTypeFloatVector,
		TypeParams: map[string]string{
			"dim": "1024",
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
	// 初始化自定义嵌入器（确保返回 Float64 向量）
	embedder, err := NewOllamaEmbedder(&OllamaEmbedderConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3-embedding:0.6b",
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
		DocumentConverter: floatDocumentConverter,
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
		fmt.Println("开始存储")
		fmt.Println(doc.ID)
		_, err := indexer.Store(ctx, storeDoc)
		if err != nil {
			log.Fatalf("Failed to store documents: %v", err)
		}
		//println("Stored documents with IDs: %v", ids)
		//fmt.Print(ids)
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
