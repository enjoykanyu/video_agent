package agent

import (
	"context"
	"fmt"
	"log"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"video_agent/tool" // 导入本地tool包
)


func newAgent() {
	ctx := context.Background() // 添加context初始化
	
	// 初始化 tools - 使用本地tool包中的ListTodoTool
	// 首先创建为BaseTool切片
	todoToolsBase := []einotool.BaseTool{
		&tool.ListTodoTool{}, // 使用本地tool包中的ListTodoTool
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

	// 创建 tools 节点
	_, err := compose.NewToolNode(ctx, &compose.ToolsNodeConfig{
		Tools: todoToolsBase,
	})
	if err != nil {
		log.Fatal(err)
	}

	// 简化版本：直接使用工具节点，暂时跳过ChatModel部分
	// 这里可以后续添加ChatModel的集成
	
	fmt.Println("Eino Agent 初始化成功")
	fmt.Printf("已加载 %d 个工具\n", len(todoToolsBase))
	for _, info := range toolInfos {
		fmt.Printf("- 工具名称: %s, 描述: %s\n", info.Name, info.Desc)
	}
	
	// 示例：直接调用工具 - 需要类型断言为InvokableTool
	if invokableTool, ok := todoToolsBase[0].(einotool.InvokableTool); ok {
		result, err := invokableTool.InvokableRun(ctx, "{}")
		if err != nil {
			log.Printf("工具调用失败: %v", err)
			return
		}
		fmt.Printf("工具调用结果: %s\n", result)
	} else {
		log.Printf("工具不支持InvokableRun方法")
	}
}
