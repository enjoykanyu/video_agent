package biz

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

const (
	AgentTypeVideo      AgentType = "video"
	AgentTypeAnalysis   AgentType = "analysis"
	AgentTypeCreation   AgentType = "creation"
	AgentTypeReport     AgentType = "report"
	AgentTypeGeneral    AgentType = "general"
	AgentTypeSupervisor AgentType = "supervisor"
	AgentTypeSummary    AgentType = "summary"
	AgentTypeEnd        AgentType = "end"
)

const SummaryAgentPrompt = `# Role: 视频助手系统 - 总结Agent

## Profile
- language: 中文
- description: 负责整合所有Agent的处理结果，给用户一个完整的回复

## Skills
1. 结果整合: 将各Agent的处理结果整合成统一的回复
2. 信息提取: 从各Agent的回复中提取关键信息
3. 格式调整: 根据用户需求调整回复格式

## Rules
1. 必须客观总结各Agent的处理结果
2. 保持回复的准确性和完整性
3. 避免遗漏重要信息

## OutputFormat
直接输出整合后的回复内容
`

func VideoAgentPromptWithDocs(docs string) string {
	if docs == "" {
		return VideoAgentSystemPrompt
	}
	return VideoAgentSystemPrompt + "\n\n## 相关资料\n" + docs
}

func AnalysisAgentPromptWithDocs(docs string) string {
	if docs == "" {
		return AnalysisAgentSystemPrompt
	}
	return AnalysisAgentSystemPrompt + "\n\n## 相关资料\n" + docs
}

func CreationAgentPromptWithDocs(docs string) string {
	if docs == "" {
		return CreationAgentSystemPrompt
	}
	return CreationAgentSystemPrompt + "\n\n## 相关资料\n" + docs
}

func ReportAgentPromptWithDocs(docs string) string {
	if docs == "" {
		return ReportAgentSystemPrompt
	}
	return ReportAgentSystemPrompt + "\n\n## 相关资料\n" + docs
}

type VideoAgentState interface {
	GetStateName() string
	NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error)
	BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error)
}

type VideoSupervisorState struct{}

func (s VideoSupervisorState) GetStateName() string {
	return "supervisor"
}

func (s VideoSupervisorState) NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error) {
	decision, err := parseDecision(msgContent)
	if err != nil {
		return AgentTypeSummary, err
	}

	scheduler.Agents = decision.SelectedAgents
	scheduler.AgentIndex = 0

	if len(scheduler.Agents) == 0 {
		return AgentTypeSummary, nil
	}

	return scheduler.Agents[0], nil
}

func (s VideoSupervisorState) BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error) {
	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage(SupervisorPrompt),
		schema.MessagesPlaceholder("history", false))

	variables := map[string]any{
		"history": msgs,
	}

	messages, err := template.Format(context.Background(), variables)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

type VideoAgentStateImpl struct{}

func (s VideoAgentStateImpl) GetStateName() string {
	return "video_agent"
}

func (s VideoAgentStateImpl) NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error) {
	scheduler.AgentIndex++

	if scheduler.AgentIndex >= len(scheduler.Agents) {
		return AgentTypeSummary, nil
	}

	return scheduler.Agents[scheduler.AgentIndex], nil
}

func (s VideoAgentStateImpl) BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error) {
	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage(VideoAgentPromptWithDocs(docs)),
		schema.MessagesPlaceholder("history", false))

	variables := map[string]any{
		"history": msgs,
	}

	messages, err := template.Format(context.Background(), variables)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

type VideoAnalysisState struct{}

func (s VideoAnalysisState) GetStateName() string {
	return "analysis_agent"
}

func (s VideoAnalysisState) NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error) {
	scheduler.AgentIndex++

	if scheduler.AgentIndex >= len(scheduler.Agents) {
		return AgentTypeSummary, nil
	}

	return scheduler.Agents[scheduler.AgentIndex], nil
}

func (s VideoAnalysisState) BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error) {
	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage(AnalysisAgentPromptWithDocs(docs)),
		schema.MessagesPlaceholder("history", false))

	variables := map[string]any{
		"history": msgs,
	}

	messages, err := template.Format(context.Background(), variables)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

type VideoCreationState struct{}

func (s VideoCreationState) GetStateName() string {
	return "creation_agent"
}

func (s VideoCreationState) NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error) {
	scheduler.AgentIndex++

	if scheduler.AgentIndex >= len(scheduler.Agents) {
		return AgentTypeSummary, nil
	}

	return scheduler.Agents[scheduler.AgentIndex], nil
}

func (s VideoCreationState) BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error) {
	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage(CreationAgentPromptWithDocs(docs)),
		schema.MessagesPlaceholder("history", false))

	variables := map[string]any{
		"history": msgs,
	}

	messages, err := template.Format(context.Background(), variables)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

type VideoReportState struct{}

func (s VideoReportState) GetStateName() string {
	return "report_agent"
}

func (s VideoReportState) NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error) {
	scheduler.AgentIndex++

	if scheduler.AgentIndex >= len(scheduler.Agents) {
		return AgentTypeSummary, nil
	}

	return scheduler.Agents[scheduler.AgentIndex], nil
}

func (s VideoReportState) BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error) {
	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage(ReportAgentPromptWithDocs(docs)),
		schema.MessagesPlaceholder("history", false))

	variables := map[string]any{
		"history": msgs,
	}

	messages, err := template.Format(context.Background(), variables)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

type VideoSummaryState struct{}

func (s VideoSummaryState) GetStateName() string {
	return "summary"
}

func (s VideoSummaryState) NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error) {
	return AgentTypeEnd, nil
}

func (s VideoSummaryState) BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error) {
	template := prompt.FromMessages(schema.FString,
		schema.SystemMessage(SummaryAgentPrompt),
		schema.MessagesPlaceholder("history", false))

	variables := map[string]any{
		"history": msgs,
	}

	messages, err := template.Format(context.Background(), variables)
	if err != nil {
		return nil, err
	}

	return messages, nil
}

type VideoEndState struct{}

func (s VideoEndState) GetStateName() string {
	return "end"
}

func (s VideoEndState) NextAgent(scheduler *VideoGraphScheduler, msgContent string) (AgentType, error) {
	return AgentTypeEnd, nil
}

func (s VideoEndState) BuildMessages(scheduler *VideoGraphScheduler, msgs []*schema.Message, docs string) ([]*schema.Message, error) {
	return nil, nil
}

func parseDecision(content string) (*SupervisorDecision, error) {
	content = trimJSON(content)

	var decision struct {
		SelectedAgents []string `json:"selected_agents"`
		Reasoning      string   `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return nil, err
	}

	var agents []AgentType
	for _, a := range decision.SelectedAgents {
		switch a {
		case "video":
			agents = append(agents, AgentTypeVideo)
		case "analysis":
			agents = append(agents, AgentTypeAnalysis)
		case "creation":
			agents = append(agents, AgentTypeCreation)
		case "report":
			agents = append(agents, AgentTypeReport)
		}
	}

	return &SupervisorDecision{
		SelectedAgents: agents,
		Reasoning:      decision.Reasoning,
	}, nil
}

func trimJSON(content string) string {
	content = strings.TrimSpace(content)
	content = strings.Trim(content, "```json")
	content = strings.Trim(content, "```")
	content = strings.TrimSpace(content)
	return content
}
