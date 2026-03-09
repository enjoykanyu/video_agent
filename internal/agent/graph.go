package agent

import (
	"context"
	"fmt"
	"log"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// VideoGraph eino图编排
type VideoGraph struct {
	runner       compose.Runnable[[]*schema.Message, *schema.Message]
	llm          model.ChatModel
	mcpManager   *MCPClientManager
	toolExecutor *ToolExecutor

	// 所有Agent节点
	supervisor     *SupervisorNode
	videoAgent     *VideoAgentNode
	analysisAgent  *AnalysisAgentNode
	creationAgent  *CreationAgentNode
	reportAgent    *ReportAgentNode
	profileAgent   *ProfileAgentNode
	recommendAgent *RecommendAgentNode
	summaryNode    *SummaryNode
}

func NewVideoGraph(llm model.ChatModel, mcpManager *MCPClientManager) (*VideoGraph, error) {
	te := NewToolExecutor(mcpManager, 5)

	vg := &VideoGraph{
		llm:          llm,
		mcpManager:   mcpManager,
		toolExecutor: te,

		supervisor:     NewSupervisorNode(llm),
		videoAgent:     NewVideoAgentNode(llm, te),
		analysisAgent:  NewAnalysisAgentNode(llm, te),
		creationAgent:  NewCreationAgentNode(llm, te),
		reportAgent:    NewReportAgentNode(llm, te),
		profileAgent:   NewProfileAgentNode(llm, te),
		recommendAgent: NewRecommendAgentNode(llm, te),
		summaryNode:    NewSummaryNode(llm),
	}

	if err := vg.buildGraph(); err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	return vg, nil
}

func (vg *VideoGraph) buildGraph() error {
	ctx := context.Background()

	// 创建带状态的图
	// 输入: []*schema.Message, 输出: *schema.Message, 状态: *GraphState
	g := compose.NewGraph[[]*schema.Message, *schema.Message](
		compose.WithGenLocalState(func(ctx context.Context) *GraphState {
			return NewGraphState("", "", "")
		}),
	)

	// ==================== 注册节点 ====================

	// 1. Supervisor节点 - Lambda节点，输入消息，输出消息
	err := g.AddLambdaNode("supervisor", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		var state *GraphState
		// 从状态中获取GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *GraphState) error {
			state = s
			// 首次进入：将用户输入写入state
			if state.OriginalQuery == "" && len(input) > 0 {
				state.OriginalQuery = input[len(input)-1].Content
				state.Messages = input
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("process state: %w", err)
		}

		plan, err := vg.supervisor.Execute(ctx, state)
		if err != nil {
			return nil, fmt.Errorf("supervisor execute: %w", err)
		}

		state.SetPlan(plan)

		// 将plan信息作为消息传递给branch
		return []*schema.Message{
			{Role: schema.Assistant, Content: fmt.Sprintf("plan:%d", len(plan.ExecutionOrder))},
		}, nil
	}), compose.WithNodeName("supervisor"))
	if err != nil {
		return fmt.Errorf("add supervisor node: %w", err)
	}

	// 2. 注册Agent执行节点 - 每个Agent作为Lambda节点
	agentNodes := map[string]Agent{
		AgentTypeVideo.NodeName():     vg.videoAgent,
		AgentTypeAnalysis.NodeName():  vg.analysisAgent,
		AgentTypeCreation.NodeName():  vg.creationAgent,
		AgentTypeReport.NodeName():    vg.reportAgent,
		AgentTypeProfile.NodeName():   vg.profileAgent,
		AgentTypeRecommend.NodeName(): vg.recommendAgent,
	}

	for nodeName, agent := range agentNodes {
		agentCopy := agent // 闭包捕获
		err := g.AddLambdaNode(nodeName, compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
			var state *GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return nil, err
			}

			// 执行Agent
			result, execErr := agentCopy.Execute(ctx, state)
			if execErr != nil {
				log.Printf("[%s] execution error: %v", agentCopy.Name(), execErr)
				result = &AgentResult{
					AgentType: agentCopy.Name(),
					Content:   fmt.Sprintf("执行出错: %v", execErr),
					Error:     execErr.Error(),
				}
			}

			// 保存结果到state
			state.SetAgentResult(agentCopy.Name(), result)
			state.AppendMessage(&schema.Message{
				Role:    schema.Assistant,
				Content: result.Content,
			})

			// 路由决策
			nextAgent, routeErr := agentCopy.Route(ctx, state, result)
			if routeErr != nil {
				nextAgent = AgentTypeSummary
			}
			result.NextAgent = nextAgent

			return []*schema.Message{
				{Role: schema.Assistant, Content: string(nextAgent)},
			}, nil
		}), compose.WithNodeName(nodeName))
		if err != nil {
			return fmt.Errorf("add agent node %s: %w", nodeName, err)
		}
	}

	// 3. 路由器节点 - 根据Agent的路由决策将消息路由到下一个节点
	err = g.AddLambdaNode("router", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) ([]*schema.Message, error) {
		// router直接传递，路由逻辑在branch里
		return input, nil
	}), compose.WithNodeName("router"))
	if err != nil {
		return fmt.Errorf("add router node: %w", err)
	}

	// 4. Summary节点
	err = g.AddLambdaNode("summary", compose.InvokableLambda(func(ctx context.Context, input []*schema.Message) (*schema.Message, error) {
		var state *GraphState
		err := compose.ProcessState(ctx, func(ctx context.Context, s *GraphState) error {
			state = s
			return nil
		})
		if err != nil {
			return nil, err
		}

		content, err := vg.summaryNode.Execute(ctx, state)
		if err != nil {
			return &schema.Message{Role: schema.Assistant, Content: "处理完成，但整合结果时出现问题。"}, nil
		}

		state.FinalAnswer = content
		return &schema.Message{Role: schema.Assistant, Content: content}, nil
	}), compose.WithNodeName("summary"))
	if err != nil {
		return fmt.Errorf("add summary node: %w", err)
	}

	// ==================== 连接边 ====================

	// START -> supervisor
	if err := g.AddEdge(compose.START, "supervisor"); err != nil {
		return fmt.Errorf("add edge START->supervisor: %w", err)
	}

	// supervisor -> branch (根据plan路由到第一个Agent或直接到summary)
	supervisorBranch := compose.NewGraphBranch(
		func(ctx context.Context, msgs []*schema.Message) (string, error) {
			var state *GraphState
			err := compose.ProcessState(ctx, func(ctx context.Context, s *GraphState) error {
				state = s
				return nil
			})
			if err != nil {
				return "summary", nil
			}

			nextAgent, hasMore := state.GetNextAgent()
			if !hasMore || state.Plan == nil || len(state.Plan.ExecutionOrder) == 0 {
				return "summary", nil
			}

			nodeName := nextAgent.NodeName()
			log.Printf("[Graph] supervisor -> %s", nodeName)
			return nodeName, nil
		},
		map[string]bool{
			AgentTypeVideo.NodeName():     true,
			AgentTypeAnalysis.NodeName():  true,
			AgentTypeCreation.NodeName():  true,
			AgentTypeReport.NodeName():    true,
			AgentTypeProfile.NodeName():   true,
			AgentTypeRecommend.NodeName(): true,
			"summary":                     true,
		},
	)
	if err := g.AddBranch("supervisor", supervisorBranch); err != nil {
		return fmt.Errorf("add supervisor branch: %w", err)
	}

	// 每个Agent -> router
	for nodeName := range agentNodes {
		if err := g.AddEdge(nodeName, "router"); err != nil {
			return fmt.Errorf("add edge %s->router: %w", nodeName, err)
		}
	}

	// router -> branch (根据Agent的路由决策到下一个Agent或summary)
	routerBranch := compose.NewGraphBranch(
		func(ctx context.Context, msgs []*schema.Message) (string, error) {
			if len(msgs) == 0 {
				return "summary", nil
			}

			// 消息内容包含下一个Agent的类型
			nextAgentStr := msgs[len(msgs)-1].Content
			nextAgent := AgentType(nextAgentStr)

			switch nextAgent {
			case AgentTypeVideo:
				return AgentTypeVideo.NodeName(), nil
			case AgentTypeAnalysis:
				return AgentTypeAnalysis.NodeName(), nil
			case AgentTypeCreation:
				return AgentTypeCreation.NodeName(), nil
			case AgentTypeReport:
				return AgentTypeReport.NodeName(), nil
			case AgentTypeProfile:
				return AgentTypeProfile.NodeName(), nil
			case AgentTypeRecommend:
				return AgentTypeRecommend.NodeName(), nil
			case AgentTypeSummary, AgentTypeEnd:
				return "summary", nil
			default:
				return "summary", nil
			}
		},
		map[string]bool{
			AgentTypeVideo.NodeName():     true,
			AgentTypeAnalysis.NodeName():  true,
			AgentTypeCreation.NodeName():  true,
			AgentTypeReport.NodeName():    true,
			AgentTypeProfile.NodeName():   true,
			AgentTypeRecommend.NodeName(): true,
			"summary":                     true,
		},
	)
	if err := g.AddBranch("router", routerBranch); err != nil {
		return fmt.Errorf("add router branch: %w", err)
	}

	// summary -> END
	if err := g.AddEdge("summary", compose.END); err != nil {
		return fmt.Errorf("add edge summary->END: %w", err)
	}

	// ==================== 编译图 ====================

	runner, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("compile graph: %w", err)
	}

	vg.runner = runner
	log.Printf("[Graph] compiled successfully")

	return nil
}

// Chat 同步调用
func (vg *VideoGraph) Chat(ctx context.Context, message string, sessionID, userID string) (string, error) {
	if vg.runner == nil {
		return "", fmt.Errorf("graph not compiled")
	}

	input := []*schema.Message{
		schema.UserMessage(message),
	}

	result, err := vg.runner.Invoke(ctx, input)
	if err != nil {
		return "", fmt.Errorf("graph invoke: %w", err)
	}

	if result == nil {
		return "处理完成", nil
	}

	return result.Content, nil
}

// StreamChat 流式调用
func (vg *VideoGraph) StreamChat(ctx context.Context, message string, sessionID, userID string) (*schema.StreamReader[*schema.Message], error) {
	if vg.runner == nil {
		return nil, fmt.Errorf("graph not compiled")
	}

	input := []*schema.Message{
		schema.UserMessage(message),
	}

	return vg.runner.Stream(ctx, input)
}
