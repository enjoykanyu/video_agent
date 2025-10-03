package rag

import (
	"context"
	"fmt"
	"github.com/cloudwego/eino-ext/components/document/transformer/splitter/markdown"
	"github.com/cloudwego/eino/schema"
	"os"
	"strconv"
)

func TransDoc() []*schema.Document {
	ctx := context.Background()

	// 初始化分割器
	splitter, err := markdown.NewHeaderSplitter(ctx, &markdown.HeaderConfig{
		Headers: map[string]string{
			"#":   "h1",
			"##":  "h2",
			"###": "h3",
		},
		TrimHeaders: false,
	})
	if err != nil {
		panic(err)
	}

	// 准备要分割的文档
	content, err := os.OpenFile("./document.md", os.O_CREATE|os.O_RDWR, 0755)
	if err != nil {
		panic(err)
	}
	defer content.Close()
	bs, err := os.ReadFile("./document.md")
	if err != nil {
		panic(err)
	}
	docs := []*schema.Document{
		{
			ID:      "doc1", //uuid.New().String(),
			Content: string(bs),
		},
	}

	// 执行分割
	results, err := splitter.Transform(ctx, docs)
	for k, doc := range results {
		doc.ID = results[0].ID + "_" + strconv.Itoa(k)
		fmt.Println(doc.ID)
	}
	if err != nil {
		panic(err)
	}

	for _, doc := range results {
		//println("片段i", i, "内容content", doc.Content)
		//println(doc.ID)
		for k, v := range doc.MetaData {
			if k == "h1" || k == "h3" {
				println("标题", k, "文字", v)
			}
		}
	}

	return results
}
