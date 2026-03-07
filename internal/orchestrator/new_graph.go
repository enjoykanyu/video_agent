package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"video_agent/internal/agent"
	"video_agent/internal/mcp"
	"video_agent/internal/memory"
	"video_agent/rag"
)

// PlanExecuteInput 图编排输入
type PlanExecuteInput struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Message   string `json:"message"`
}

// PlanExecuteOutput 图编排输出
type PlanExecuteOutput struct {
	SessionID string                 `json:"session_id"`
	Reply     string                 `json:"reply"`
	Intent    string                 `json:"intent"`
	Agent     string                 `json:"agent"`
	Plan      *ExecutionPlan         `json:"plan,omitempty"`
	Steps     []StepResult           `json:"steps,omitempty"`
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// ExecutionPlan 执行计划
type ExecutionPlan struct {
	ID          string     `json:"id"`
	Goal        string     `json:"goal"`
	Description string     `json:"description"`
	Steps       []PlanStep `json:"steps"`
	TotalSteps  int        `json:"total_steps"`
	CreatedAt   time.Time  `json:"created_at"`
}

// PlanStep 计划步骤
type PlanStep struct {
	ID             string   `json:"id"`
	Order          int      `json:"order"`
	Description    string   `json:"description"`
	AgentType      string   `json:"agent_type"` // video/analysis/creation/report
	Action         string   `json:"action"`
	Dependencies   []string `json:"dependencies"`
	ExpectedOutput string   `json:"expected_output"`
}

// StepResult 步骤执行结果
type StepResult struct {
	StepID      string      `json:"step_id"`
	StepOrder   int         `json:"step_order"`
	AgentType   string      `json:"agent_type"`
	Action      string      `json:"action"`
	Input       interface{} `json:"input"`
	Output      interface{} `json:"output"`
	Status      string      `json:"status"` // success/failed/pending
	Error       string      `json:"error,omitempty"`
	DurationMs  int64       `json:"duration_ms"`
	StartedAt   time.Time   `json:"started_at"`
	CompletedAt time.Time   `json:"completed_at"`
}

// PEGraphState PlanExecute图状态
type PEGraphState struct {
	SessionID        string           `json:"session_id"`
	UserID           string           `json:"user_id"`
	OriginalMessage  string           `json:"original_message"`
	Intent           agent.IntentType `json:"intent"`
	IntentConfidence float64          `json:"intent_confidence"`
	VideoID          string           `json:"video_id,omitempty"`
	RAGContext       string           `json:"rag_context,omitempty"`

	// Plan-and-Execute核心字段
	Plan             *ExecutionPlan        `json:"plan,omitempty"`
	CurrentStepIndex int                   `json:"current_step_index"`
	StepResults      []StepResult          `json:"step_results"`
	ToolResults      []ToolExecutionResult `json:"tool_results"`

	// 最终结果
	FinalAnswer string                 `json:"final_answer"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// 系统提示词定义

const (
	SystemPromptPlanning = `你是小V的任务规划专家。你的职责是将用户的复杂任务拆解为可执行的步骤计划。

【角色定位】
- 你擅长分析任务需求，制定清晰的执行计划
- 你了解视频分析、内容创作、数据报表等不同领域的执行流程

【规划原则】
1. 步骤要具体、可执行、有明确的输出
2. 考虑步骤间的依赖关系
3. 每个步骤指定合适的Agent类型
4. 计划要覆盖任务的完整生命周期

【可用Agent类型】
- video: 视频处理Agent，负责视频下载、帧提取、语音转文字
- analysis: 分析Agent，负责数据分析、情感分析、趋势分析
- creation: 创作Agent，负责内容生成、文案创作
- report: 报表Agent，负责数据汇总、报告生成

【输出格式】
必须输出JSON格式的执行计划：
{
  "goal": "任务目标",
  "description": "计划描述",
  "steps": [
    {
      "id": "step_1",
      "order": 1,
      "description": "步骤描述",
      "agent_type": "video|analysis|creation|report",
      "action": "具体动作",
      "dependencies": [],
      "expected_output": "期望输出"
    }
  ]
}`

	SystemPromptVideo = `你是小V的视频处理专家。你的职责是处理视频相关的技术任务。

【角色定位】
- 你擅长视频下载、帧提取、语音转文字等技术操作
- 你能够调用视频处理工具获取视频数据

【任务范围】
1. 视频下载和缓存
2. 关键帧提取
3. 语音转文字
4. 视频元数据获取

【输出要求】
- 输出结构化的视频数据
- 包含错误处理和状态报告`

	SystemPromptAnalysis = `你是小V的数据分析专家。你的职责是基于视频数据进行深度分析。

【角色定位】
- 你擅长视频内容分析、情感分析、趋势分析
- 你能够从数据中提取关键洞察

【分析维度】
1. 内容摘要和主题提取
2. 情感倾向分析
3. 数据表现分析（播放量、互动率等）
4. 竞品对比分析
5. 优化建议

【输出要求】
- 结构化的分析报告
- 数据驱动的洞察
- 可执行的建议`

	SystemPromptCreation = `你是小V的内容创作专家。你的职责是创作高质量的视频内容。

【角色定位】
- 你擅长文案创作、标题优化、内容策划
- 你了解不同平台的调性和用户喜好

【创作类型】
1. 视频标题和描述
2. 脚本创作
3. 标签和话题推荐
4. 封面文案

【输出要求】
- 创意新颖、吸引人
- 符合平台调性
- 有传播潜力`

	SystemPromptReport = `你是小V的报表生成专家。你的职责是整合多源数据生成专业报告。

【角色定位】
- 你擅长数据汇总、可视化描述、报告撰写
- 你能够整合多个步骤的结果

【报告类型】
1. 视频分析报告
2. 周报/月报
3. 竞品分析报告
4. 趋势报告

【输出要求】
- 结构清晰、重点突出
- 数据准确、引用完整
- 结论明确、建议可行`

	SystemPromptSynthesis = `你是小V的综合整理专家。你的职责是整合多步执行结果，生成最终回答。

【角色定位】
- 你擅长信息整合、逻辑梳理、表达优化
- 你能够将多步骤结果整合为连贯的回答

【整合原则】
1. 保持信息的完整性和准确性
2. 逻辑清晰、层次分明
3. 语言流畅、易于理解
4. 突出关键信息和核心结论

【输出要求】
- 直接回答用户问题
- 引用相关数据和结果
- 提供总结性见解`
)

// PlanExecuteGraph 图编排器
type PlanExecuteGraph struct {
	graph         compose.Runnable[PlanExecuteInput, PlanExecuteOutput]
	llm           model.ChatModel
	intentAgent   *agent.IntentRecognitionAgent
	memoryManager *memory.MemoryManager
	mcpManager    *mcp.Manager
	ragManager    *rag.RAGManager
	systemPrompts map[string]string
}

// NewPlanExecuteGraph 创建新的图编排器
func NewPlanExecuteGraph(
	llm model.ChatModel,
	intentAgent *agent.IntentRecognitionAgent,
	memoryManager *memory.MemoryManager,
	mcpManager *mcp.Manager,
	ragManager *rag.RAGManager,
) (*PlanExecuteGraph, error) {

	peg := &PlanExecuteGraph{
		llm:           llm,
		intentAgent:   intentAgent,
		memoryManager: memoryManager,
		mcpManager:    mcpManager,
		ragManager:    ragManager,
		systemPrompts: map[string]string{
			"planning":  SystemPromptPlanning,
			"video":     SystemPromptVideo,
			"analysis":  SystemPromptAnalysis,
			"creation":  SystemPromptCreation,
			"report":    SystemPromptReport,
			"synthesis": SystemPromptSynthesis,
		},
	}

	if err := peg.buildGraph(); err != nil {
		return nil, err
	}

	return peg, nil
}

// buildGraph 构建图编排
func (peg *PlanExecuteGraph) buildGraph() error {
	ctx := context.Background()

	// 创建状态图
	g := compose.NewGraph[PlanExecuteInput, PlanExecuteOutput](
		compose.WithGenLocalState(func(ctx context.Context) *PEGraphState {
			return &PEGraphState{
				Metadata:    make(map[string]interface{}),
				StepResults: make([]StepResult, 0),
			}
		}),
	)

	// 节点定义
	intentNode := compose.InvokableLambda(peg.intentRecognitionNode)
	ragNode := compose.InvokableLambda(peg.ragRetrievalNode)
	routerNode := compose.InvokableLambda(peg.routerNode)
	planningNode := compose.InvokableLambda(peg.planningNode)
	executorNode := compose.InvokableLambda(peg.executorNode)
	stepCheckNode := compose.InvokableLambda(peg.stepCheckNode)
	synthesisNode := compose.InvokableLambda(peg.synthesisNode)
	outputNode := compose.InvokableLambda(peg.outputNode)

	// 添加节点
	g.AddLambdaNode("intent", intentNode)
	g.AddLambdaNode("rag", ragNode)
	g.AddLambdaNode("router", routerNode)
	g.AddLambdaNode("planning", planningNode)
	g.AddLambdaNode("executor", executorNode)
	g.AddLambdaNode("step_check", stepCheckNode)
	g.AddLambdaNode("synthesis", synthesisNode)
	g.AddLambdaNode("output", outputNode)

	// 连接边
	g.AddEdge(compose.START, "intent")
	g.AddEdge("intent", "rag")
	g.AddEdge("intent", "router")
	g.AddEdge("rag", "router")

	// 路由分支
	g.AddBranch("router", compose.NewGraphBranch(
		func(ctx context.Context, state PEGraphState) (string, error) {
			if state.Intent == agent.IntentKnowledgeQA && state.RAGContext != "" {
				log.Printf("🔀 [路由] 知识问答，直接RAG回答")
				return "synthesis", nil
			}
			log.Printf("🔀 [路由] 复杂任务，进入Plan-and-Execute")
			return "planning", nil
		},
		map[string]bool{"synthesis": true, "planning": true},
	))

	// Plan-and-Execute循环
	g.AddEdge("planning", "executor")
	g.AddEdge("executor", "step_check")
	g.AddBranch("step_check", compose.NewGraphBranch(
		func(ctx context.Context, state PEGraphState) (string, error) {
			if state.Plan != nil && state.CurrentStepIndex < len(state.Plan.Steps) {
				log.Printf("🔀 [步骤检查] 继续执行步骤 %d/%d",
					state.CurrentStepIndex+1, len(state.Plan.Steps))
				return "executor", nil
			}
			log.Printf("🔀 [步骤检查] 所有步骤完成，进入综合")
			return "synthesis", nil
		},
		map[string]bool{"executor": true, "synthesis": true},
	))

	g.AddEdge("synthesis", "output")
	g.AddEdge("output", compose.END)

	// 编译图
	runnable, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("编译图编排失败: %w", err)
	}

	peg.graph = runnable
	return nil
}

// 节点实现“
func (peg *PlanExecuteGraph) intentRecognitionNode(ctx context.Context, input PlanExecuteInput) (PEGraphState, error) {
	log.Printf("🎯 [意图识别] SessionID: %s", input.SessionID)

	intent, err := peg.intentAgent.Recognize(ctx, input.Message)
	if err != nil {
		log.Printf("⚠️ [意图识别] 失败: %v, 使用通用对话", err)
		intent = &agent.Intent{
			Type:       agent.IntentGeneralChat,
			Confidence: 1.0,
			RawQuery:   input.Message,
		}
	}

	videoID := extractVideoID(input.Message)

	state := PEGraphState{
		SessionID:        input.SessionID,
		UserID:           input.UserID,
		OriginalMessage:  input.Message,
		Intent:           intent.Type,
		IntentConfidence: intent.Confidence,
		VideoID:          videoID,
		Metadata:         map[string]interface{}{"intent_recognized_at": time.Now()},
		StepResults:      make([]StepResult, 0),
	}

	log.Printf("🎯 [意图识别] 结果: type=%s, confidence=%.2f", intent.Type, intent.Confidence)
	return state, nil
}

func (peg *PlanExecuteGraph) ragRetrievalNode(ctx context.Context, state PEGraphState) (PEGraphState, error) {
	if peg.ragManager == nil {
		return state, nil
	}

	docs, err := peg.ragManager.SearchSimilarDocuments(state.OriginalMessage, 3)
	if err != nil {
		log.Printf("⚠️ RAG检索失败: %v", err)
		return state, nil
	}

	var contextBuilder strings.Builder
	for _, doc := range docs {
		contextBuilder.WriteString(doc.Content + "\n")
	}
	state.RAGContext = contextBuilder.String()
	state.Metadata["rag_doc_count"] = len(docs)

	log.Printf("📚 [RAG] 检索到 %d 个文档", len(docs))
	return state, nil
}

func (peg *PlanExecuteGraph) routerNode(ctx context.Context, state PEGraphState) (PEGraphState, error) {
	log.Printf("🔀 [路由] Intent: %s, RAGContext长度: %d", state.Intent, len(state.RAGContext))
	return state, nil
}

func (peg *PlanExecuteGraph) planningNode(ctx context.Context, state PEGraphState) (PEGraphState, error) {
	log.Printf("📋 [规划] 开始生成执行计划")
	startTime := time.Now()

	systemPrompt := peg.systemPrompts["planning"]
	userPrompt := fmt.Sprintf(`请为以下任务生成执行计划：

用户问题：%s
意图类型：%s
视频ID：%s

请生成详细的执行计划，包含具体的执行步骤。`,
		state.OriginalMessage, state.Intent, state.VideoID)

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	}

	response, err := peg.llm.Generate(ctx, messages)
	if err != nil {
		return state, fmt.Errorf("规划失败: %w", err)
	}

	plan, err := parseExecutionPlan(response.Content)
	if err != nil {
		log.Printf("⚠️ [规划] 解析计划失败: %v, 使用默认计划", err)
		plan = generateDefaultPlan(state.Intent, state.OriginalMessage)
	}

	state.Plan = plan
	state.CurrentStepIndex = 0
	state.Metadata["planning_duration_ms"] = time.Since(startTime).Milliseconds()

	log.Printf("📋 [规划] 生成计划: %d 个步骤", len(plan.Steps))
	for i, step := range plan.Steps {
		log.Printf("   [%d] %s: %s", i+1, step.AgentType, step.Description)
	}

	return state, nil
}

func (peg *PlanExecuteGraph) executorNode(ctx context.Context, state PEGraphState) (PEGraphState, error) {
	if state.Plan == nil || state.CurrentStepIndex >= len(state.Plan.Steps) {
		return state, nil
	}

	step := state.Plan.Steps[state.CurrentStepIndex]
	log.Printf("⚙️ [执行] 步骤 %d/%d: %s", state.CurrentStepIndex+1, len(state.Plan.Steps), step.Description)
	startTime := time.Now()

	systemPrompt := peg.systemPrompts[step.AgentType]
	userPrompt := buildAgentUserPrompt(step, state)

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	}

	response, err := peg.llm.Generate(ctx, messages)

	stepResult := StepResult{
		StepID:      step.ID,
		StepOrder:   step.Order,
		AgentType:   step.AgentType,
		Action:      step.Action,
		Input:       userPrompt,
		StartedAt:   startTime,
		CompletedAt: time.Now(),
		DurationMs:  time.Since(startTime).Milliseconds(),
	}

	if err != nil {
		stepResult.Status = "failed"
		stepResult.Error = err.Error()
		log.Printf("❌ [执行] 步骤失败: %v", err)
	} else {
		stepResult.Status = "success"
		var result interface{}
		if err := json.Unmarshal([]byte(response.Content), &result); err != nil {
			result = map[string]string{"result": response.Content}
		}
		stepResult.Output = result
		log.Printf("✅ [执行] 步骤完成")
	}

	state.StepResults = append(state.StepResults, stepResult)
	state.CurrentStepIndex++

	return state, nil
}

func (peg *PlanExecuteGraph) stepCheckNode(ctx context.Context, state PEGraphState) (PEGraphState, error) {
	return state, nil
}

func (peg *PlanExecuteGraph) synthesisNode(ctx context.Context, state PEGraphState) (PEGraphState, error) {
	log.Printf("📝 [综合] 整合执行结果")
	startTime := time.Now()

	if state.RAGContext != "" && state.Intent == agent.IntentKnowledgeQA {
		return peg.synthesisRAGResponse(ctx, state)
	}

	systemPrompt := peg.systemPrompts["synthesis"]

	var stepResultsJSON strings.Builder
	for _, sr := range state.StepResults {
		resultJSON, _ := json.Marshal(sr.Output)
		stepResultsJSON.WriteString(fmt.Sprintf("步骤 %d (%s): %s\n结果: %s\n\n",
			sr.StepOrder, sr.AgentType, sr.Action, string(resultJSON)))
	}

	userPrompt := fmt.Sprintf(`请基于以下执行结果生成最终回答：

用户问题：%s

执行步骤结果：
%s

请生成完整、连贯的回答。`, state.OriginalMessage, stepResultsJSON.String())

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	}

	response, err := peg.llm.Generate(ctx, messages)
	if err != nil {
		state.FinalAnswer = "抱歉，生成回答时出现问题。"
		return state, err
	}

	state.FinalAnswer = response.Content
	state.Metadata["synthesis_duration_ms"] = time.Since(startTime).Milliseconds()

	log.Printf("📝 [综合] 完成，回答长度: %d", len(state.FinalAnswer))
	return state, nil
}

func (peg *PlanExecuteGraph) synthesisRAGResponse(ctx context.Context, state PEGraphState) (PEGraphState, error) {
	systemPrompt := `你是小V的知识问答助手。基于提供的相关知识回答用户问题。`

	userPrompt := fmt.Sprintf(`【相关知识】
%s

【用户问题】
%s

请基于以上知识回答问题。如果知识不足以回答，请坦诚说明。`,
		state.RAGContext, state.OriginalMessage)

	messages := []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	}

	response, err := peg.llm.Generate(ctx, messages)
	if err != nil {
		state.FinalAnswer = "抱歉，生成回答时出现问题。"
		return state, err
	}

	state.FinalAnswer = response.Content
	return state, nil
}

func (peg *PlanExecuteGraph) outputNode(ctx context.Context, state PEGraphState) (PlanExecuteOutput, error) {
	output := PlanExecuteOutput{
		SessionID: state.SessionID,
		Reply:     state.FinalAnswer,
		Intent:    string(state.Intent),
		Agent:     "plan_execute",
		Plan:      state.Plan,
		Steps:     state.StepResults,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  state.Metadata,
	}

	if peg.memoryManager != nil {
		assistantMemory := memory.Memory{
			ID:        uuid.New().String(),
			SessionID: state.SessionID,
			Content:   state.FinalAnswer,
			Type:      memory.MemoryTypeAssistant,
			CreatedAt: time.Now(),
			Metadata: map[string]interface{}{
				"user_id": state.UserID,
				"intent":  string(state.Intent),
			},
		}
		peg.memoryManager.Store(ctx, assistantMemory)
	}

	return output, nil
}

// 辅助函数
func buildAgentUserPrompt(step PlanStep, state PEGraphState) string {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf("【任务】%s\n", step.Description))
	prompt.WriteString(fmt.Sprintf("【动作】%s\n", step.Action))
	prompt.WriteString(fmt.Sprintf("【用户问题】%s\n", state.OriginalMessage))

	if state.VideoID != "" {
		prompt.WriteString(fmt.Sprintf("【视频ID】%s\n", state.VideoID))
	}

	if len(state.StepResults) > 0 {
		prompt.WriteString("\n【前置步骤结果】\n")
		for _, sr := range state.StepResults {
			if sr.Status == "success" {
				resultJSON, _ := json.Marshal(sr.Output)
				prompt.WriteString(fmt.Sprintf("- %s: %s\n", sr.Action, string(resultJSON)))
			}
		}
	}

	prompt.WriteString(fmt.Sprintf("\n【期望输出】%s\n", step.ExpectedOutput))
	return prompt.String()
}

func parseExecutionPlan(content string) (*ExecutionPlan, error) {
	startIdx := strings.Index(content, "{")
	endIdx := strings.LastIndex(content, "}")
	if startIdx == -1 || endIdx == -1 {
		return nil, fmt.Errorf("未找到JSON内容")
	}

	jsonStr := content[startIdx : endIdx+1]

	var plan struct {
		Goal        string     `json:"goal"`
		Description string     `json:"description"`
		Steps       []PlanStep `json:"steps"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, err
	}

	return &ExecutionPlan{
		ID:          uuid.New().String(),
		Goal:        plan.Goal,
		Description: plan.Description,
		Steps:       plan.Steps,
		TotalSteps:  len(plan.Steps),
		CreatedAt:   time.Now(),
	}, nil
}

func generateDefaultPlan(intent agent.IntentType, message string) *ExecutionPlan {
	plan := &ExecutionPlan{
		ID:         uuid.New().String(),
		Goal:       message,
		CreatedAt:  time.Now(),
		TotalSteps: 1,
	}

	switch intent {
	case agent.IntentVideoAnalysis:
		plan.Description = "视频分析计划"
		plan.Steps = []PlanStep{
			{ID: "step_1", Order: 1, Description: "分析视频内容", AgentType: "analysis", Action: "analyze_video", ExpectedOutput: "分析报告"},
		}
	case agent.IntentContentCreation:
		plan.Description = "内容创作计划"
		plan.Steps = []PlanStep{
			{ID: "step_1", Order: 1, Description: "创作内容", AgentType: "creation", Action: "create_content", ExpectedOutput: "创作内容"},
		}
	default:
		plan.Description = "通用处理计划"
		plan.Steps = []PlanStep{
			{ID: "step_1", Order: 1, Description: "处理问题", AgentType: "analysis", Action: "process", ExpectedOutput: "处理结果"},
		}
	}

	return plan
}

func extractVideoID(message string) string {
	bvPattern := regexp.MustCompile(`[Bb][Vv][a-zA-Z0-9]{10}`)
	if match := bvPattern.FindString(message); match != "" {
		return strings.ToUpper(match)
	}
	numPattern := regexp.MustCompile(`\d{8,}`)
	if match := numPattern.FindString(message); match != "" {
		return match
	}
	return ""
}

// Execute 执行图编排
func (peg *PlanExecuteGraph) Execute(ctx context.Context, input PlanExecuteInput) (*PlanExecuteOutput, error) {
	if input.SessionID == "" {
		input.SessionID = fmt.Sprintf("session_%d", time.Now().UnixMilli())
	}

	log.Printf("🚀 [PlanExecuteGraph] 开始执行")

	output, err := peg.graph.Invoke(ctx, input)
	if err != nil {
		log.Printf("❌ [PlanExecuteGraph] 执行失败: %v", err)
		return nil, err
	}

	log.Printf("✅ [PlanExecuteGraph] 执行完成")
	return &output, nil
}
