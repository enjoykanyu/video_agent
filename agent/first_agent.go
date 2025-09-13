package agent

import (
	"context"
	"fmt"
	"log"
	"video_agent/tool" // 导入本地tool包

	"github.com/cloudwego/eino-ext/components/model/ollama"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func NewAgent() {
	ctx := context.Background() // 添加context初始化

	// 初始化 tools - 使用本地tool包中的所有工具
	// 首先创建为BaseTool切片
	todoToolsBase := []einotool.BaseTool{
		&tool.ListTodoTool{},   // 列出todo项目
		&tool.AddTodoTool{},    // 添加todo项目
		&tool.SearchRepoTool{}, // 搜索仓库
	}
	// 创建并配置 ChatModel
	chatModel, err := ollama.NewChatModel(ctx, &ollama.ChatModelConfig{
		BaseURL: "http://localhost:11434", // Ollama 服务地址
		Model:   "qwen3:0.6b",             // 模型名称
	})
	if err != nil {
		log.Fatal(err)
	}
	// 获取工具信息
	toolInfos := make([]*schema.ToolInfo, 0, len(todoToolsBase))
	for _, t := range todoToolsBase {
		info, err := t.Info(ctx)
		if err != nil {
			log.Fatal(err)
		}
		toolInfos = append(toolInfos, info)
	}
	err = chatModel.BindTools(toolInfos)
	if err != nil {
		log.Fatal(err)
	}

	// 创建 tools 节点
	todoToolsNode, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: todoToolsBase,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 构建完整的处理链
	chain := compose.NewChain[[]*schema.Message, []*schema.Message]()
	chain.
		AppendChatModel(chatModel, compose.WithNodeName("chat_model")).
		AppendToolsNode(todoToolsNode, compose.WithNodeName("tools"))

	// 编译并运行 chain
	agent, err := chain.Compile(ctx)
	if err != nil {
		log.Fatal(err)
	}

	// 运行示例
	resp, err := agent.Invoke(ctx, []*schema.Message{
		{
			Role:    schema.User,
			Content: "列出我的TODO列表，添加一个学习 Eino 的 TODO，同时搜索一下 cloudwego/eino 的仓库地址",
		},
	})
	if err != nil {
		log.Fatal(err)
	}

	// 输出结果
	for _, msg := range resp {
		fmt.Println(msg.Content)
	}
}
