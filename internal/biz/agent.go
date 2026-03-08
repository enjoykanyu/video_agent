package biz

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type AnalysisAgent struct {
	llm          model.ChatModel
	mcpTools     []tool.BaseTool
	systemPrompt string
	timeout      time.Duration
}

func NewAnalysisAgent(llm model.ChatModel, mcpTools []tool.BaseTool) *AnalysisAgent {
	return &AnalysisAgent{
		llm:          llm,
		mcpTools:     mcpTools,
		systemPrompt: AnalysisAgentSystemPrompt,
		timeout:      60 * time.Second,
	}
}

const AnalysisAgentSystemPrompt = `# Role: 视频助手 - 数据分析Agent

## Profile
- language: 中文
- description: 专业的数据分析助手，负责视频数据分析、趋势追踪、竞品分析等
- expertise: 数据分析、趋势分析、统计分析
- target_audience: 需要数据分析服务的用户

## Skills
1. 视频数据分析
2. 趋势追踪分析
3. 竞品对比分析
4. 热点分析

## Rules
1. 基于数据给出客观分析
2. 提供有价值的洞察和建议

## OutputFormat
- 格式: JSON
- structure: {analysis: {...}, insights: [...], suggestions: [...]}
`

func (aa *AnalysisAgent) Call(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, aa.timeout)
	defer cancel()

	if len(aa.mcpTools) > 0 {
		aa.llm.BindTools(extractToolInfos(aa.mcpTools))
	}

	resp, err := aa.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("analysis agent generate failed: %w", err)
	}

	return resp, nil
}

func (aa *AnalysisAgent) StreamCall(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	ctx, cancel := context.WithTimeout(ctx, aa.timeout)
	defer cancel()

	if len(aa.mcpTools) > 0 {
		aa.llm.BindTools(extractToolInfos(aa.mcpTools))
	}

	return aa.llm.Stream(ctx, messages)
}

type CreationAgent struct {
	llm          model.ChatModel
	mcpTools     []tool.BaseTool
	systemPrompt string
	timeout      time.Duration
}

func NewCreationAgent(llm model.ChatModel, mcpTools []tool.BaseTool) *CreationAgent {
	return &CreationAgent{
		llm:          llm,
		mcpTools:     mcpTools,
		systemPrompt: CreationAgentSystemPrompt,
		timeout:      60 * time.Second,
	}
}

const CreationAgentSystemPrompt = `# Role: 视频助手 - 内容创作Agent

## Profile
- language: 中文
- description: 专业的内容创作助手，负责文案生成、脚本编写、标题创作等
- expertise: 内容创作、文案撰写、创意生成
- target_audience: 需要内容创作服务的用户

## Skills
1. 文案生成
2. 标题创作
3. 脚本编写
4. 封面建议

## OutputFormat
- 格式: JSON
- structure: {title: "...", content: "...", tags: [...]}
`

func (ca *CreationAgent) Call(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, ca.timeout)
	defer cancel()

	if len(ca.mcpTools) > 0 {
		ca.llm.BindTools(extractToolInfos(ca.mcpTools))
	}

	resp, err := ca.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("creation agent generate failed: %w", err)
	}

	return resp, nil
}

func (ca *CreationAgent) StreamCall(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	ctx, cancel := context.WithTimeout(ctx, ca.timeout)
	defer cancel()

	if len(ca.mcpTools) > 0 {
		ca.llm.BindTools(extractToolInfos(ca.mcpTools))
	}

	return ca.llm.Stream(ctx, messages)
}

type ReportAgent struct {
	llm          model.ChatModel
	mcpTools     []tool.BaseTool
	systemPrompt string
	timeout      time.Duration
}

func NewReportAgent(llm model.ChatModel, mcpTools []tool.BaseTool) *ReportAgent {
	return &ReportAgent{
		llm:          llm,
		mcpTools:     mcpTools,
		systemPrompt: ReportAgentSystemPrompt,
		timeout:      60 * time.Second,
	}
}

const ReportAgentSystemPrompt = `# Role: 视频助手 - 报表Agent

## Profile
- language: 中文
- description: 专业的报表生成助手，负责周报、月报、数据报表生成
- expertise: 数据汇总、报表生成、统计报告
- target_audience: 需要报表服务的用户

## Skills
1. 周报生成
2. 月报生成
3. 数据汇总
4. 报表导出

## OutputFormat
- 格式: JSON
- structure: {summary: {...}, metrics: {...}, report: "..."}
`

func (ra *ReportAgent) Call(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, ra.timeout)
	defer cancel()

	if len(ra.mcpTools) > 0 {
		ra.llm.BindTools(extractToolInfos(ra.mcpTools))
	}

	resp, err := ra.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("report agent generate failed: %w", err)
	}

	return resp, nil
}

func (ra *ReportAgent) StreamCall(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	ctx, cancel := context.WithTimeout(ctx, ra.timeout)
	defer cancel()

	if len(ra.mcpTools) > 0 {
		ra.llm.BindTools(extractToolInfos(ra.mcpTools))
	}

	return ra.llm.Stream(ctx, messages)
}

type GeneralChatAgent struct {
	llm          model.ChatModel
	mcpTools     []tool.BaseTool
	systemPrompt string
	timeout      time.Duration
}

func NewGeneralChatAgent(llm model.ChatModel, mcpTools []tool.BaseTool) *GeneralChatAgent {
	return &GeneralChatAgent{
		llm:          llm,
		mcpTools:     mcpTools,
		systemPrompt: GeneralChatAgentSystemPrompt,
		timeout:      30 * time.Second,
	}
}

const GeneralChatAgentSystemPrompt = `# Role: 视频助手 - 通用对话Agent

## Profile
- language: 中文
- description: 友好的通用对话助手，可以回答各种问题
- expertise: 对话交流、信息解答
- target_audience: 所有用户

## Skills
1. 问答交流
2. 信息查询
3. 日常对话

## OutputFormat
- 格式: text
`

func (gca *GeneralChatAgent) Call(ctx context.Context, messages []*schema.Message) (*schema.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, gca.timeout)
	defer cancel()

	resp, err := gca.llm.Generate(ctx, messages)
	if err != nil {
		return nil, fmt.Errorf("general chat agent generate failed: %w", err)
	}

	return resp, nil
}

func (gca *GeneralChatAgent) StreamCall(ctx context.Context, messages []*schema.Message) (*schema.StreamReader[*schema.Message], error) {
	ctx, cancel := context.WithTimeout(ctx, gca.timeout)
	defer cancel()

	return gca.llm.Stream(ctx, messages)
}

func BuildAgentMessages(systemPrompt, userMessage string) []*schema.Message {
	return []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userMessage),
	}
}

func BuildAgentMessagesWithHistory(systemPrompt string, history []*schema.Message, userMessage string) []*schema.Message {
	messages := make([]*schema.Message, 0, len(history)+2)
	messages = append(messages, schema.SystemMessage(systemPrompt))
	messages = append(messages, history...)
	messages = append(messages, schema.UserMessage(userMessage))
	return messages
}

func ExtractMessageContent(msgs []*schema.Message) string {
	var sb strings.Builder
	for _, msg := range msgs {
		sb.WriteString(msg.Content)
		sb.WriteString("\n")
	}
	return sb.String()
}
