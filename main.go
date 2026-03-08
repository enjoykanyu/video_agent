package main333

// import (
// 	"fmt"
// 	"video_agent/rag"
// 	// "video_agent/api"
// )

// func main() {
// 	fmt.Println("🚀 启动基于Eino框架的RAG多智能体系统...")
// 	fmt.Println("📋 系统组件:")
// 	fmt.Println("  • Eino RAG管理器 (Ollama + Elasticsearch)")
// 	fmt.Println("  • 向量相似度搜索")
// 	fmt.Println("  • Graph工作流编排")
// 	fmt.Println("  • Ollama模型集成")
// 	fmt.Println()

// 	// 测试Eino RAG功能
// 	fmt.Println("🔍 测试Eino RAG功能...")
// 	// testEinoRAG()
// 	fmt.Println()

// 	// 测试基础RAG功能
// 	fmt.Println("🔍 测试基础RAG功能...")
// 	// testBasicRAG()
// 	fmt.Println()
// 	//docs := []*schema.Document{
// 	//	{
// 	//		ID:      "9",
// 	//		Content: "测试下",
// 	//		MetaData: map[string]any{
// 	//			"author": "哈哈哈",
// 	//		},
// 	//	},
// 	//}
// 	//rag.IndexerRAG(docs)
// 	//rag.RetrieverRAG("测试下")
// 	docs := rag.TransDoc()
// 	rag.IndexerRAG(docs)
// 	rag.RetrieverRAG("哈哈哈")
// 	// // 测试Eino RAG功能V2
// 	// fmt.Println("🔍 测试Eino RAG功能V2...")
// 	// testEinoRAGV2()
// 	// fmt.Println()

// 	// // 测试高级RAG功能
// 	// fmt.Println("🎯 测试高级RAG功能...")
// 	// testAdvancedRAG()

// 	// // 测试高级Eino RAG功能V2
// 	// fmt.Println("🎯 测试高级Eino RAG功能V2...")
// 	// testAdvancedEinoRAGV2()
// 	// fmt.Println()

// 	// 保持程序运行，等待用户输入
// 	fmt.Println("💡 系统运行中，按 Ctrl+C 退出...")
// 	// waitForExit()
// 	// fmt.Println("🚀 启动多智能体系统...")
// 	// fmt.Println("📋 系统组件:")
// 	// fmt.Println("  • 意图识别Agent")
// 	// fmt.Println("  • 多工具分流调度器")
// 	// fmt.Println("  • RAG知识库模块")
// 	// fmt.Println("  • Graph工作流编排")
// 	// fmt.Println("  • Gin HTTP API服务器")
// 	// fmt.Println()

// 	// // 初始化RAG知识库
// 	// fmt.Println("📚 初始化RAG知识库...")
// 	// if err := rag.InitKnowledgeBase(context.Background()); err != nil {
// 	// 	// 暂时不处理错误，因为es8和ollama可能未启动，后续运行若需要再处理
// 	// 	fmt.Printf("⚠️  RAG知识库初始化失败 (可忽略): %v\n", err)
// 	// } else {
// 	// 	fmt.Println("✅ RAG知识库初始化完成")
// 	// }
// 	// fmt.Println()

// 	// // 测试意图识别
// 	// fmt.Println("🧠 测试意图识别...")
// 	// testIntentRecognition()
// 	// fmt.Println()

// 	// // 测试工具调度
// 	// fmt.Println("🔧 测试工具调度...")
// 	// testToolDispatch()
// 	// fmt.Println()

// 	// // 测试RAG功能
// 	// fmt.Println("🔍 测试RAG检索...")
// 	// testRAGFunction()
// 	// fmt.Println()

// 	// // 测试完整工作流
// 	// fmt.Println("🔄 测试完整Graph工作流...")
// 	// agent.TestFullWorkflow()
// 	// fmt.Println()

// 	// // 启动HTTP服务器
// 	// fmt.Println("🌐 启动HTTP API服务器...")
// 	// startHTTPServer()

// }

// // func testIntentRecognition() {
// // 	ctx := context.Background()

// // 	testCases := []string{
// // 		"帮我添加一个待办事项",
// // 		"什么是人工智能",
// // 		"搜索关于机器学习的资料",
// // 		"随便问点什么",
// // 	}
// // 	for _, input := range testCases {
// // 		result, err := agent.RecognizeIntentAPI(ctx, input)
// // 		if err != nil {
// // 			fmt.Printf("❌ 意图识别失败: %s -> %v\n", input, err)
// // 		} else {
// // 			fmt.Printf("✅ %s -> %s (置信度: %.2f)\n", input, result.Type, result.Confidence)
// // 		}
// // 	}
// // }

// // func testToolDispatch() {
// // 	ctx := context.Background()

// // 	// 创建工具调度器
// // 	dispatcher, err := agent.NewToolDispatcher(ctx)
// // 	if err != nil {
// // 		fmt.Printf("❌ 创建工具调度器失败: %v\n", err)
// // 		return
// // 	}

// // 	testCases := []struct {
// // 		intent agent.IntentType
// // 		input  string
// // 	}{
// // 		{agent.IntentMCP, "添加待办事项：学习Go语言"},
// // 		{agent.IntentQA, "解释一下机器学习的概念"},
// // 		{agent.IntentRAG, "查找深度学习的相关资料"},
// // 	}

// // 	for _, tc := range testCases {
// // 		result, err := dispatcher.DispatchByIntent(ctx, tc.intent, tc.input)
// // 		if err != nil {
// // 			fmt.Printf("❌ 工具调度失败: %s -> %v\n", tc.intent, err)
// // 		} else {
// // 			fmt.Printf("✅ %s: %s -> %+v\n", tc.intent, tc.input, result)
// // 		}
// // 	}
// // }

// // func testRAGFunction() {
// // 	// ctx := context.Background()

// // 	// // 测试文档存储
// // 	// documents := []string{
// // 	// 	"机器学习是人工智能的一个分支，专注于开发能够从数据中学习的算法。",
// // 	// 	"深度学习是机器学习的一个子领域，使用多层神经网络来处理复杂模式。",
// // 	// 	"自然语言处理(NLP)是人工智能的一个领域，专注于计算机与人类语言之间的交互。",
// // 	// }

// // 	// for i, doc := range documents {
// // 	// 	err := rag.StoreDocument(ctx, fmt.Sprintf("doc-%d", i+1), doc)
// // 	// 	if err != nil {
// // 	// 		fmt.Printf("❌ 文档存储失败: %v\n", err)
// // 	// 	} else {
// // 	// 		fmt.Printf("✅ 文档存储成功: %s\n", doc[:30]+"...")
// // 	// 	}
// // 	// }

// // 	// 测试检索
// // 	// query := "机器学习"
// // 	// results, err := rag.SearchDocuments(ctx, query, 3)
// // 	// if err != nil {
// // 	// 	fmt.Printf("❌ 检索失败: %v\n", err)
// // 	// } else {
// // 	// 	fmt.Printf("✅ 检索结果(%s):\n", query)
// // 	// 	for i, result := range results {
// // 	// 		fmt.Printf("  %d. %s\n", i+1, result.Content[:50]+"...")
// // 	// 	}
// // 	// }
// // }

// // func startHTTPServer() {
// // 	// 创建Gin服务器
// // 	server := api.NewGinServer()
// // 	server.SetupRoutes()

// // 	// 设置优雅关闭
// // 	stop := make(chan os.Signal, 1)
// // 	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

// // 	go func() {
// // 		// 启动服务器
// // 		if err := server.Start(":8080"); err != nil {
// // 			log.Fatalf("❌ 服务器启动失败: %v", err)
// // 		}
// // 	}()

// // 	fmt.Println("✅ HTTP服务器已启动")
// // 	fmt.Println("📍 访问 http://localhost:8080 查看API文档")
// // 	fmt.Println("🛑 按 Ctrl+C 停止服务器")

// // 	// 等待中断信号
// // 	<-stop
// // 	fmt.Println("\n🛑 接收到停止信号，正在关闭服务器...")

// // 	// 优雅关闭
// // 	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
// // 	defer cancel()

// // 	fmt.Println("👋 服务器已关闭")
// // 	<-ctx.Done()
// // }

// // // testEinoRAG 测试Eino RAG功能
// // func testEinoRAG() {
// // 	config := &agent.EinoRAGConfig{
// // 		ESAddresses: []string{"http://localhost:9200"},
// // 		ESIndexName: "eino_rag_demo",
// // 		OllamaHost:  "http://localhost:11434",
// // 		ModelName:   "nomic-embed-text",
// // 		TopK:        3,
// // 	}

// // 	fmt.Println("🚀 初始化Eino RAG图代理...")
// // 	if err := agent.NewEinoRAGGraph(config); err != nil {
// // 		fmt.Printf("❌ Eino RAG图代理初始化失败: %v\n", err)
// // 		fmt.Println("💡 提示: 请确保Elasticsearch和Ollama服务已启动")
// // 		fmt.Println("   • Elasticsearch: docker run -d -p 9200:9200 -e \"discovery.type=single-node\" elasticsearch:8.8.0")
// // 		fmt.Println("   • Ollama: curl -fsSL https://ollama.com/install.sh | sh && ollama pull nomic-embed-text")
// // 	} else {
// // 		fmt.Println("✅ Eino RAG图代理初始化成功")
// // 	}
// // }

// // // testBasicRAG 测试基础RAG功能
// // func testBasicRAG() {
// // 	config := &agent.RAGConfig{
// // 		VectorStorePath: "./data/vector_store/documents.json",
// // 		RAGStorePath:    "./data/rag_store/documents.json",
// // 		TopK:            3,
// // 		ModelName:       "qwen3:0.6b",
// // 		BaseURL:         "http://localhost:11434",
// // 	}

// // 	if err := agent.NewRAGGraph(config); err != nil {
// // 		fmt.Printf("❌ 基础RAG测试失败: %v\n", err)
// // 		return
// // 	}
// // 	fmt.Println("✅ 基础RAG功能测试完成")
// // }

// // // testAdvancedRAG 测试高级RAG功能
// // func testAdvancedRAG() {
// // 	config := &agent.RAGConfig{
// // 		VectorStorePath: "./data/vector_store/documents.json",
// // 		RAGStorePath:    "./data/rag_store/documents.json",
// // 		TopK:            2,
// // 		ModelName:       "qwen3:0.6b",
// // 		BaseURL:         "http://localhost:11434",
// // 	}

// // 	if err := agent.NewAdvancedRAGGraph(config); err != nil {
// // 		fmt.Printf("❌ 高级RAG测试失败: %v\n", err)
// // 		return
// // 	}
// // 	fmt.Println("✅ 高级RAG功能测试完成")
// // }

// // // waitForExit 等待退出信号
// // func waitForExit() {
// // 	// 设置信号处理
// // 	sigChan := make(chan os.Signal, 1)
// // 	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

// // 	// 等待信号
// // 	<-sigChan
// // 	fmt.Println("\n🛑 接收到退出信号，正在关闭系统...")

// // 	// 给系统一些时间清理资源
// // 	time.Sleep(1 * time.Second)
// // 	fmt.Println("👋 系统已安全关闭")
// // }

// // // testEinoRAGV2 测试Eino RAG功能V2
// // func testEinoRAGV2() {
// // 	config := &agent.EinoRAGConfigV2{
// // 		ESAddresses: []string{"http://localhost:9200"},
// // 		ESIndexName: "eino_rag_v2",
// // 		OllamaHost:  "http://localhost:11434",
// // 		EmbedModel:  "nomic-embed-text",
// // 		ChatModel:   "llama3.2",
// // 		TopK:        5,
// // 	}

// // 	if err := agent.NewEinoRAGGraphV2(config); err != nil {
// // 		fmt.Printf("❌ Eino RAG V2测试失败: %v\n", err)
// // 		return
// // 	}

// // 	fmt.Println("✅ Eino RAG V2测试完成")
// // }

// // // testAdvancedEinoRAGV2 测试高级Eino RAG功能V2
// // func testAdvancedEinoRAGV2() {
// // 	config := &agent.EinoRAGConfigV2{
// // 		ESAddresses: []string{"http://localhost:9200"},
// // 		ESIndexName: "advanced_eino_rag_v2",
// // 		OllamaHost:  "http://localhost:11434",
// // 		EmbedModel:  "nomic-embed-text",
// // 		ChatModel:   "llama3.2",
// // 		TopK:        3,
// // 	}

// // 	if err := agent.NewAdvancedEinoRAGGraphV2(config); err != nil {
// // 		fmt.Printf("❌ 高级Eino RAG V2测试失败: %v\n", err)
// // 		return
// // 	}

// // 	fmt.Println("✅ 高级Eino RAG V2测试完成")
// // }

// // // init 初始化函数
// // func init() {
// // 	fmt.Println("🐙 基于Eino框架的RAG多智能体系统初始化中...")
// // 	fmt.Println("🏗️  架构: CloudWeGo Eino + RAG + Ollama")
// // 	fmt.Println("🎯 功能: RAG检索 → Graph编排 → 模型生成")
// // 	fmt.Println("----------------------------------------")
// // }
