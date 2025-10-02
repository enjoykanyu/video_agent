package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ollama"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"video_agent/rag"
	"video_agent/tool"
)

// RAGConfig RAG配置
type RAGConfig struct {
	VectorStorePath string
	RAGStorePath    string
	TopK            int
	ModelName       string
	BaseURL         string
}

// NewRAGGraph 创建带有RAG功能的图代理
func NewRAGGraph(config *RAGConfig) error {
	ctx := context.Background()

	// 创建RAG管理器
	ragManager, err := rag.NewRAGManager(config.VectorStorePath, config.RAGStorePath)
	if err != nil {
		return fmt.Errorf("failed to create RAG manager: %w", err)
	}

	// 创建图
	g := compose.NewGraph[map[string]string, *schema.Message]()

	// 创建输入处理节点
	inputProcessor := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output map[string]string, err error) {
		return input, nil
	})

	// 创建消息构建节点
	messageBuilder := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output []*schema.Message, err error) {
		query := input["query"]
		if query == "" {
			query = input["content"]
		}

		return []*schema.Message{
			{
				Role:    schema.System,
				Content: "你是一个智能助手，能够基于检索到的文档信息提供准确的回答。",
			},
			{
				Role:    schema.User,
				Content: query,
			},
		}, nil
	})

	// 创建RAG增强节点
	ragEnhancer := tool.CreateEnhancedRAGNode(ragManager, config.TopK)

	// 创建模型节点
	model, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: config.BaseURL,
		Model:   config.ModelName,
	})
	if err != nil {
		return fmt.Errorf("failed to create chat model: %w", err)
	}

	// 添加节点到图
	err = g.AddLambdaNode("input_processor", inputProcessor)
	if err != nil {
		return err
	}

	err = g.AddLambdaNode("message_builder", messageBuilder)
	if err != nil {
		return err
	}

	err = g.AddLambdaNode("rag_enhancer", ragEnhancer)
	if err != nil {
		return err
	}

	err = g.AddChatModelNode("model", model)
	if err != nil {
		return err
	}

	// 连接节点
	err = g.AddEdge(compose.START, "input_processor")
	if err != nil {
		return err
	}

	err = g.AddEdge("input_processor", "message_builder")
	if err != nil {
		return err
	}

	err = g.AddEdge("message_builder", "rag_enhancer")
	if err != nil {
		return err
	}

	err = g.AddEdge("rag_enhancer", "model")
	if err != nil {
		return err
	}

	err = g.AddEdge("model", compose.END)
	if err != nil {
		return err
	}

	// 编译图
	r, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("failed to compile graph: %w", err)
	}

	// 测试运行
	testInput := map[string]string{
		"query": "什么是机器学习？",
	}

	fmt.Println("=== RAG图代理测试 ===")
	fmt.Printf("输入: %s\n", testInput["query"])
	fmt.Println("正在执行RAG检索和模型生成...")

	// 创建带超时的上下文
	ctxWithTimeout, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	answer, err := r.Invoke(ctxWithTimeout, testInput)
	if err != nil {
		return fmt.Errorf("failed to invoke graph: %w", err)
	}

	fmt.Printf("回答: %s\n", answer.Content)
	fmt.Println("=== RAG图代理测试完成 ===")

	return nil
}

// NewAdvancedRAGGraph 创建高级RAG图，包含更多功能
func NewAdvancedRAGGraph(config *RAGConfig) error {
	ctx := context.Background()

	// 创建RAG管理器
	ragManager, err := rag.NewRAGManager(config.VectorStorePath, config.RAGStorePath)
	if err != nil {
		return fmt.Errorf("failed to create RAG manager: %w", err)
	}

	// 创建图
	g := compose.NewGraph[map[string]string, *schema.Message]()

	// 路由节点 - 根据输入类型决定处理方式
	router := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output map[string]string, err error) {
		// 直接传递输入数据，通过分支逻辑处理不同类型
		return input, nil
	})

	// 搜索处理节点
	searchProcessor := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output []*schema.Message, err error) {
		query := input["query"]
		docs, err := ragManager.SearchSimilarDocuments(query, config.TopK)
		if err != nil {
			return nil, err
		}

		var contextStr string
		for i, doc := range docs {
			contextStr += fmt.Sprintf("文档 %d: %s\n", i+1, doc.Content)
		}

		return []*schema.Message{
			{
				Role:    schema.System,
				Content: fmt.Sprintf("基于以下检索到的文档回答问题：\n\n%s", contextStr),
			},
			{
				Role:    schema.User,
				Content: query,
			},
		}, nil
	})

	// 聊天处理节点
	chatProcessor := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output []*schema.Message, err error) {
		query := input["query"]
		return []*schema.Message{
			{
				Role:    schema.System,
				Content: "你是一个智能助手，请回答用户的问题。",
			},
			{
				Role:    schema.User,
				Content: query,
			},
		}, nil
	})

	// 文档添加节点
	addDocProcessor := compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output *schema.Message, err error) {
		content := input["content"]
		if content == "" {
			return &schema.Message{
				Role:    schema.System,
				Content: "错误：缺少文档内容",
			}, nil
		}

		metadata := make(map[string]interface{})
		if category, ok := input["category"]; ok {
			metadata["category"] = category
		}
		if source, ok := input["source"]; ok {
			metadata["source"] = source
		}

		err = ragManager.AddDocument(content, metadata)
		if err != nil {
			return &schema.Message{
				Role:    schema.System,
				Content: fmt.Sprintf("添加文档失败: %v", err),
			}, nil
		}

		return &schema.Message{
			Role:    schema.System,
			Content: "文档添加成功",
		}, nil
	})

	// 创建模型节点
	model, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: config.BaseURL,
		Model:   config.ModelName,
	})
	if err != nil {
		return fmt.Errorf("failed to create chat model: %w", err)
	}

	// 添加节点
	err = g.AddLambdaNode("router", router)
	if err != nil {
		return err
	}

	err = g.AddLambdaNode("search_processor", searchProcessor)
	if err != nil {
		return err
	}

	err = g.AddLambdaNode("chat_processor", chatProcessor)
	if err != nil {
		return err
	}

	err = g.AddLambdaNode("add_doc_processor", addDocProcessor)
	if err != nil {
		return err
	}

	err = g.AddChatModelNode("model", model)
	if err != nil {
		return err
	}

	// 添加分支
	err = g.AddBranch("router", compose.NewGraphBranch(func(ctx context.Context, in map[string]string) (endNode string, err error) {
		if in["type"] == "search" {
			return "search_processor", nil
		} else if in["type"] == "chat" {
			return "chat_processor", nil
		} else if in["type"] == "add_doc" {
			return "add_doc_processor", nil
		}
		return "chat_processor", nil // 默认为聊天处理
	}, map[string]bool{"search_processor": true, "chat_processor": true, "add_doc_processor": true}))
	if err != nil {
		return err
	}

	// 连接边
	err = g.AddEdge(compose.START, "router")
	if err != nil {
		return err
	}

	err = g.AddEdge("search_processor", "model")
	if err != nil {
		return err
	}

	err = g.AddEdge("chat_processor", "model")
	if err != nil {
		return err
	}

	err = g.AddEdge("add_doc_processor", compose.END)
	if err != nil {
		return err
	}

	err = g.AddEdge("model", compose.END)
	if err != nil {
		return err
	}

	// 编译图
	r, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("failed to compile graph: %w", err)
	}

	// 测试不同功能
	fmt.Println("=== 高级RAG图代理测试 ===")

	// 测试搜索
	fmt.Println("\n1. 搜索模式测试:")
	searchInput := map[string]string{
		"type":  "search",
		"query": "什么是机器学习？",
	}
	answer, err := r.Invoke(ctx, searchInput)
	if err != nil {
		return fmt.Errorf("failed to invoke search: %w", err)
	}
	fmt.Printf("搜索结果: %s\n", answer.Content)

	// 测试添加文档
	fmt.Println("\n2. 添加文档测试:")
	addInput := map[string]string{
		"type":     "add_doc",
		"content":  "Eino是一个强大的Go语言AI框架，支持构建复杂的AI应用。",
		"category": "technology",
		"source":   "manual",
	}
	result, err := r.Invoke(ctx, addInput)
	if err != nil {
		return fmt.Errorf("failed to invoke add_doc: %w", err)
	}
	fmt.Printf("添加结果: %s\n", result.Content)

	// 再次搜索测试新文档
	fmt.Println("\n3. 搜索新文档测试:")
	searchInput2 := map[string]string{
		"type":  "search",
		"query": "Eino框架是什么？",
	}
	answer2, err := r.Invoke(ctx, searchInput2)
	if err != nil {
		return fmt.Errorf("failed to invoke search2: %w", err)
	}
	fmt.Printf("搜索结果: %s\n", answer2.Content)

	fmt.Println("\n=== 高级RAG图代理测试完成 ===")

	return nil
}
