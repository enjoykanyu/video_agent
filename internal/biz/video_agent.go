package biz

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type VideoAgent struct {
	llm        model.ChatModel
	mcpTools   []tool.BaseTool
	systemPrompt string
	timeout    time.Duration
}

func NewVideoAgent(llm model.ChatModel, mcpTools []tool.BaseTool) *VideoAgent {
	return &VideoAgent{
		llm:          llm,
		mcpTools:     mcpTools,
		systemPrompt: VideoAgentSystemPrompt,
		timeout:      60 * time.Second,
	}
}

const VideoAgentSystemPrompt = `# Role: 视频助手 - 视频Agent

## Profile
- language: 中文
- description: 专业的视频助手，负责处理视频相关的所有操作请求
- expertise: 视频信息获取、视频数据查询、视频内容理解
- target_audience: 需要视频相关服务的用户

## Skills
1. 视频信息获取
2. 视频数据分析
3. 视频内容理解
4. 视频相关问题解答

## Rules
1. 必须准确理解用户对视频的需求
2. 必要时调用 MCP 工具获取视频信息
3. 返回结果要清晰、准确

## Workflows
- 步骤 1: 解析用户请求，提取视频ID或视频URL
- 步骤 2: 调用 MCP 工具获取视频信息
- 步骤 3: 返回视频相关信息或数据

## OutputFormat
- 格式: JSON
- structure: {video_info: {...}, analysis: {...}}
`

func (va *VideoAgent) Call(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, va.timeout)
	defer cancel()

	if len(va.mcpTools) > 0 {
		va.llm.BindTools(extractToolInfos(va.mcpTools))
	}

	resp, err := va.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("video agent generate failed: %w", err)
	}

	return resp, nil
}

func (va *VideoAgent) StreamCall(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	ctx, cancel := context.WithTimeout(ctx, va.timeout)
	defer cancel()

	if len(va.mcpTools) > 0 {
		va.llm.BindTools(extractToolInfos(va.mcpTools))
	}

	return va.llm.Stream(ctx, messages)
}

func extractToolInfos(tools []tool.BaseTool) []*schema.ToolInfo {
	var infos []*schema.ToolInfo
	for _, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		infos = append(infos, info)
	}
	return infos
}
