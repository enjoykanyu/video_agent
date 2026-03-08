package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type VideoGraphScheduler struct {
	CurrentAgent AgentType
	Agents       []AgentType
	AgentIndex   int
	Messages     []*schema.Message
	RAGDocs      []RAGDocument
}

func NewVideoGraphScheduler() *VideoGraphScheduler {
	return &VideoGraphScheduler{
		CurrentAgent: AgentTypeSupervisor,
		Agents:       []AgentType{},
		AgentIndex:   0,
		Messages:     []*schema.Message{},
	}
}

type VideoGraph struct {
	runner   interface{}
	llm      model.ChatModel
	mcpTools []tool.BaseTool
}

func NewVideoGraph(llm model.ChatModel, mcpTools []tool.BaseTool) (*VideoGraph, error) {
	vg := &VideoGraph{
		llm:      llm,
		mcpTools: mcpTools,
	}

	scheduler := NewVideoGraphScheduler()

	if err := vg.buildGraph(scheduler); err != nil {
		return nil, err
	}

	return vg, nil
}

func (vg *VideoGraph) buildGraph(scheduler *VideoGraphScheduler) error {
	ctx := context.Background()

	g := compose.NewGraph[[]*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *VideoGraphScheduler {
			return scheduler
		}))

	_ = g.AddPassthroughNode("decision")

	videoModel := vg.llm
	analysisModel := vg.llm
	creationModel := vg.llm
	reportModel := vg.llm
	summaryModel := vg.llm

	if len(vg.mcpTools) > 0 {
		toolInfos := extractToolInfos(vg.mcpTools)
		videoModel.BindTools(toolInfos)
		analysisModel.BindTools(toolInfos)
		creationModel.BindTools(toolInfos)
		reportModel.BindTools(toolInfos)
	}

	_ = g.AddChatModelNode("supervisor", vg.llm,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *VideoGraphScheduler) ([]*schema.Message, error) {
			if len(state.Messages) == 0 {
				return []*schema.Message{}, nil
			}
			lastMsg := state.Messages[len(state.Messages)-1]
			return []*schema.Message{
				schema.SystemMessage(SupervisorPrompt),
				schema.UserMessage(lastMsg.Content),
			}, nil
		}),
		compose.WithNodeName("supervisor"))

	_ = g.AddChatModelNode("video_agent", videoModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *VideoGraphScheduler) ([]*schema.Message, error) {
			return BuildAgentMessagesWithHistory(VideoAgentSystemPrompt, state.Messages, ""), nil
		}),
		compose.WithNodeName("video_agent"))

	_ = g.AddChatModelNode("analysis_agent", analysisModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *VideoGraphScheduler) ([]*schema.Message, error) {
			return BuildAgentMessagesWithHistory(AnalysisAgentSystemPrompt, state.Messages, ""), nil
		}),
		compose.WithNodeName("analysis_agent"))

	_ = g.AddChatModelNode("creation_agent", creationModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *VideoGraphScheduler) ([]*schema.Message, error) {
			return BuildAgentMessagesWithHistory(CreationAgentSystemPrompt, state.Messages, ""), nil
		}),
		compose.WithNodeName("creation_agent"))

	_ = g.AddChatModelNode("report_agent", reportModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *VideoGraphScheduler) ([]*schema.Message, error) {
			return BuildAgentMessagesWithHistory(ReportAgentSystemPrompt, state.Messages, ""), nil
		}),
		compose.WithNodeName("report_agent"))

	_ = g.AddChatModelNode("summary", summaryModel,
		compose.WithStatePreHandler(func(ctx context.Context, input []*schema.Message, state *VideoGraphScheduler) ([]*schema.Message, error) {
			return BuildAgentMessagesWithHistory(SummaryAgentPrompt, state.Messages, ""), nil
		}),
		compose.WithNodeName("summary"))

	_ = g.AddEdge(compose.START, "decision")

	_ = g.AddBranch("decision", compose.NewGraphBranch(
		func(ctx context.Context, in []*schema.Message) (string, error) {
			var next string
			err := compose.ProcessState(ctx, func(ctx context.Context, state *VideoGraphScheduler) error {
				if len(state.Agents) == 0 || state.AgentIndex >= len(state.Agents) {
					next = "summary"
					return nil
				}

				switch state.Agents[state.AgentIndex] {
				case AgentTypeVideo:
					next = "video_agent"
				case AgentTypeAnalysis:
					next = "analysis_agent"
				case AgentTypeCreation:
					next = "creation_agent"
				case AgentTypeReport:
					next = "report_agent"
				default:
					next = "summary"
				}
				return nil
			})
			if err != nil {
				return "", err
			}
			return next, nil
		},
		map[string]bool{
			"video_agent":    true,
			"analysis_agent": true,
			"creation_agent": true,
			"report_agent":   true,
			"summary":        true,
		}))

	_ = g.AddBranch("supervisor", compose.NewGraphBranch(
		func(ctx context.Context, in *schema.Message) (string, error) {
			if in == nil || in.Content == "" {
				return "summary", nil
			}

			var decision *SupervisorDecision
			err := compose.ProcessState(ctx, func(ctx context.Context, state *VideoGraphScheduler) error {
				var err error
				decision, err = vg.parseSupervisorDecision(in.Content)
				if err != nil {
					return err
				}

				state.Agents = decision.SelectedAgents
				state.AgentIndex = 0
				return nil
			})
			if err != nil {
				return "summary", err
			}

			if decision == nil || len(decision.SelectedAgents) == 0 {
				return "summary", nil
			}

			switch decision.SelectedAgents[0] {
			case AgentTypeVideo:
				return "video_agent", nil
			case AgentTypeAnalysis:
				return "analysis_agent", nil
			case AgentTypeCreation:
				return "creation_agent", nil
			case AgentTypeReport:
				return "report_agent", nil
			default:
				return "summary", nil
			}
		},
		map[string]bool{
			"video_agent":    true,
			"analysis_agent": true,
			"creation_agent": true,
			"report_agent":   true,
			"summary":        true,
		}))

	addAgentEdge := func(agentNode string) {
		_ = g.AddEdge(agentNode, "decision")
	}

	addAgentEdge("video_agent")
	addAgentEdge("analysis_agent")
	addAgentEdge("creation_agent")
	addAgentEdge("report_agent")

	_ = g.AddEdge("summary", compose.END)

	runner, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("compile graph failed: %w", err)
	}

	vg.runner = runner
	return nil
}

func (vg *VideoGraph) parseSupervisorDecision(content string) (*SupervisorDecision, error) {
	content = strings.TrimSpace(content)
	content = strings.Trim(content, "```json")
	content = strings.Trim(content, "```")
	content = strings.TrimSpace(content)

	var decision struct {
		SelectedAgents []string `json:"selected_agents"`
		Reasoning      string   `json:"reasoning"`
	}

	if err := json.Unmarshal([]byte(content), &decision); err != nil {
		return nil, fmt.Errorf("parse decision failed: %w", err)
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

func (vg *VideoGraph) Chat(ctx context.Context, message string) (string, error) {
	if vg.runner == nil {
		return "", fmt.Errorf("graph not initialized")
	}

	messages := []*schema.Message{
		schema.UserMessage(message),
	}

	result, err := vg.runner.(interface {
		Invoke(ctx context.Context, in []*schema.Message) (*schema.Message, error)
	}).Invoke(ctx, messages)

	if err != nil {
		return "", err
	}

	if result == nil {
		return "处理完成", nil
	}

	return result.Content, nil
}

func (vg *VideoGraph) StreamChat(ctx context.Context, message string) (*schema.StreamReader[*schema.Message], error) {
	if vg.runner == nil {
		return nil, fmt.Errorf("graph not initialized")
	}

	messages := []*schema.Message{
		schema.UserMessage(message),
	}

	return vg.runner.(interface {
		Stream(ctx context.Context, in []*schema.Message) (*schema.StreamReader[*schema.Message], error)
	}).Stream(ctx, messages)
}
