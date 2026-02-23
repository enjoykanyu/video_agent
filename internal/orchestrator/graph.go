package orchestrator

import (
	"context"
	"encoding/json"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/google/uuid"

	"video_agent/internal/agent"
	"video_agent/internal/mcp"
	"video_agent/internal/memory"
)

// AssistantState 助手状态
type AssistantState struct {
	SessionID   string                 `json:"session_id"`
	UserID      string                 `json:"user_id"`
	Intent      *agent.Intent          `json:"intent"`
	Context     map[string]interface{} `json:"context"`
	History     []Message              `json:"history"`
	Results     map[string]interface{} `json:"results"`
	Plan        *TaskPlan              `json:"plan"`
	StartTime   time.Time              `json:"start_time"`
	CurrentStep string                 `json:"current_step"`
	Completed   []string               `json:"completed"`
	Failed      []string               `json:"failed"`
}

// Message 消息
type Message struct {
	Role      string    `json:"role"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// TaskPlan 任务计划
type TaskPlan struct {
	ID        string     `json:"id"`
	Goal      string     `json:"goal"`
	Steps     []TaskStep `json:"steps"`
	Status    string     `json:"status"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// TaskStep 任务步骤
type TaskStep struct {
	ID           string      `json:"id"`
	Description  string      `json:"description"`
	Agent        string      `json:"agent"`
	Dependencies []string    `json:"dependencies"`
	Status       string      `json:"status"`
	Result       interface{} `json:"result,omitempty"`
	StartTime    *time.Time  `json:"start_time,omitempty"`
	EndTime      *time.Time  `json:"end_time,omitempty"`
}

// UserInput 用户输入
type UserInput struct {
	SessionID string                 `json:"session_id"`
	UserID    string                 `json:"user_id"`
	Content   string                 `json:"content"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// AssistantResponse 助手响应
type AssistantResponse struct {
	SessionID   string                 `json:"session_id"`
	Content     string                 `json:"content"`
	Intent      string                 `json:"intent"`
	Agent       string                 `json:"agent"`
	Results     map[string]interface{} `json:"results,omitempty"`
	Suggestions []string               `json:"suggestions,omitempty"`
	Timestamp   time.Time              `json:"timestamp"`
	Duration    int64                  `json:"duration_ms"`
}

// GraphOrchestrator 图编排器
type GraphOrchestrator struct {
	// Eino图
	runnable compose.Runnable[UserInput, AssistantResponse]

	// Agent集合
	agents map[agent.IntentType]Agent

	// 意图识别Agent
	intentAgent *agent.IntentRecognitionAgent

	// MCP工具注册中心
	toolRegistry *mcp.Registry

	// 记忆管理器
	memoryManager *memory.MemoryManager

	// LLM模型
	llm model.ChatModel
}

// Agent Agent接口
type Agent interface {
	Execute(ctx context.Context, input interface{}, context map[string]interface{}) (interface{}, error)
}

// NewGraphOrchestrator 创建图编排器
func NewGraphOrchestrator(
	llm model.ChatModel,
	intentAgent *agent.IntentRecognitionAgent,
	toolRegistry *mcp.Registry,
	memoryManager *memory.MemoryManager,
) (*GraphOrchestrator, error) {
	o := &GraphOrchestrator{
		agents:        make(map[agent.IntentType]Agent),
		intentAgent:   intentAgent,
		toolRegistry:  toolRegistry,
		memoryManager: memoryManager,
		llm:           llm,
	}

	// 构建图
	if err := o.buildGraph(); err != nil {
		return nil, err
	}

	return o, nil
}

// RegisterAgent 注册Agent
func (o *GraphOrchestrator) RegisterAgent(intentType agent.IntentType, agent Agent) {
	o.agents[intentType] = agent
}

// buildGraph 构建编排图
func (o *GraphOrchestrator) buildGraph() error {
	ctx := context.Background()

	// 创建图，使用状态管理
	g := compose.NewGraph[UserInput, AssistantResponse](
		compose.WithGenLocalState(func(ctx context.Context) *AssistantState {
			return &AssistantState{
				Context: make(map[string]interface{}),
				Results: make(map[string]interface{}),
				History: make([]Message, 0),
			}
		}),
	)

	// 1. 意图识别节点
	intentNode := compose.InvokableLambda(func(ctx context.Context, input UserInput) (output UserInput, err error) {
		// 识别意图
		intent, err := o.intentAgent.Recognize(ctx, input.Content)
		if err != nil {
			return input, err
		}

		// 存储到记忆
		o.memoryManager.Store(ctx, memory.Memory{
			SessionID:  input.SessionID,
			Type:       memory.MemoryTypeShortTerm,
			Content:    input.Content,
			Importance: 0.5,
		})

		// 将意图存储到输入的Metadata中传递
		if input.Metadata == nil {
			input.Metadata = make(map[string]interface{})
		}
		intentJSON, _ := json.Marshal(intent)
		input.Metadata["intent"] = string(intentJSON)
		input.Metadata["session_id"] = input.SessionID
		input.Metadata["user_id"] = input.UserID
		input.Metadata["start_time"] = time.Now().Format(time.RFC3339)

		return input, nil
	})

	// 2. 上下文加载节点
	contextNode := compose.InvokableLambda(func(ctx context.Context, input UserInput) (output UserInput, err error) {
		// 检索相关记忆
		memories, err := o.memoryManager.Retrieve(ctx, input.Content, input.SessionID, 5)
		if err == nil && len(memories) > 0 {
			if input.Metadata == nil {
				input.Metadata = make(map[string]interface{})
			}
			memoriesJSON, _ := json.Marshal(memories)
			input.Metadata["memories"] = string(memoriesJSON)
		}

		return input, nil
	})

	// 3. 任务规划节点
	planNode := compose.InvokableLambda(func(ctx context.Context, input UserInput) (output UserInput, err error) {
		// 解析意图
		var intent agent.Intent
		if intentJSON, ok := input.Metadata["intent"].(string); ok {
			json.Unmarshal([]byte(intentJSON), &intent)
		}

		// 根据意图创建任务计划
		plan := o.createTaskPlan(&intent)
		planJSON, _ := json.Marshal(plan)

		if input.Metadata == nil {
			input.Metadata = make(map[string]interface{})
		}
		input.Metadata["plan"] = string(planJSON)

		return input, nil
	})

	// 4. Agent执行节点
	agentNode := compose.InvokableLambda(func(ctx context.Context, input UserInput) (output interface{}, err error) {
		// 解析意图
		var intent agent.Intent
		if intentJSON, ok := input.Metadata["intent"].(string); ok {
			json.Unmarshal([]byte(intentJSON), &intent)
		}

		// 获取对应Agent
		agentImpl, exists := o.agents[intent.Type]
		if !exists {
			// 使用通用Agent
			agentImpl = o.agents[agent.IntentGeneralChat]
		}

		// 构建上下文
		contextData := make(map[string]interface{})
		if memoriesJSON, ok := input.Metadata["memories"].(string); ok {
			var memories []memory.Memory
			json.Unmarshal([]byte(memoriesJSON), &memories)
			contextData["memories"] = memories
		}

		// 执行Agent
		result, err := agentImpl.Execute(ctx, input, contextData)
		if err != nil {
			return nil, err
		}

		return result, nil
	})

	// 5. 输出格式化节点
	outputNode := compose.InvokableLambda(func(ctx context.Context, input interface{}) (output AssistantResponse, err error) {
		// 格式化响应
		response := o.formatResponse(input)

		return response, nil
	})

	// 添加节点
	if err := g.AddLambdaNode("intent", intentNode); err != nil {
		return err
	}
	if err := g.AddLambdaNode("context", contextNode); err != nil {
		return err
	}
	if err := g.AddLambdaNode("plan", planNode); err != nil {
		return err
	}
	if err := g.AddLambdaNode("agent", agentNode); err != nil {
		return err
	}
	if err := g.AddLambdaNode("output", outputNode); err != nil {
		return err
	}

	// 添加边
	if err := g.AddEdge(compose.START, "intent"); err != nil {
		return err
	}
	if err := g.AddEdge("intent", "context"); err != nil {
		return err
	}
	if err := g.AddEdge("context", "plan"); err != nil {
		return err
	}
	if err := g.AddEdge("plan", "agent"); err != nil {
		return err
	}
	if err := g.AddEdge("agent", "output"); err != nil {
		return err
	}
	if err := g.AddEdge("output", compose.END); err != nil {
		return err
	}

	// 编译图
	r, err := g.Compile(ctx)
	if err != nil {
		return err
	}

	o.runnable = r
	return nil
}

// Execute 执行编排
func (o *GraphOrchestrator) Execute(ctx context.Context, input UserInput) (*AssistantResponse, error) {
	if input.SessionID == "" {
		input.SessionID = uuid.New().String()
	}

	// 执行图
	response, err := o.runnable.Invoke(ctx, input)
	if err != nil {
		return nil, err
	}

	return &response, nil
}

// createTaskPlan 创建任务计划
func (o *GraphOrchestrator) createTaskPlan(intent *agent.Intent) *TaskPlan {
	plan := &TaskPlan{
		ID:        uuid.New().String(),
		Goal:      intent.RawQuery,
		Status:    "pending",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// 根据意图类型创建步骤
	switch intent.Type {
	case agent.IntentVideoAnalysis:
		plan.Steps = []TaskStep{
			{ID: "1", Description: "下载视频", Agent: "video_processor", Status: "pending"},
			{ID: "2", Description: "提取关键帧", Agent: "frame_extractor", Status: "pending", Dependencies: []string{"1"}},
			{ID: "3", Description: "语音转文字", Agent: "transcriber", Status: "pending", Dependencies: []string{"1"}},
			{ID: "4", Description: "生成摘要", Agent: "summarizer", Status: "pending", Dependencies: []string{"2", "3"}},
		}

	case agent.IntentRecommendation:
		plan.Steps = []TaskStep{
			{ID: "1", Description: "获取用户画像", Agent: "user_profile", Status: "pending"},
			{ID: "2", Description: "搜索相似内容", Agent: "vector_search", Status: "pending", Dependencies: []string{"1"}},
			{ID: "3", Description: "生成推荐列表", Agent: "recommender", Status: "pending", Dependencies: []string{"2"}},
		}

	case agent.IntentKnowledgeQA:
		plan.Steps = []TaskStep{
			{ID: "1", Description: "检索知识库", Agent: "knowledge_retriever", Status: "pending"},
			{ID: "2", Description: "生成答案", Agent: "qa_generator", Status: "pending", Dependencies: []string{"1"}},
		}

	default:
		plan.Steps = []TaskStep{
			{ID: "1", Description: "处理用户请求", Agent: "general_processor", Status: "pending"},
		}
	}

	return plan
}

// formatResponse 格式化响应
func (o *GraphOrchestrator) formatResponse(result interface{}) AssistantResponse {
	content := ""
	if result != nil {
		switch v := result.(type) {
		case string:
			content = v
		case map[string]interface{}:
			if msg, ok := v["message"].(string); ok {
				content = msg
			} else {
				data, _ := json.Marshal(v)
				content = string(data)
			}
		default:
			data, _ := json.Marshal(v)
			content = string(data)
		}
	}

	return AssistantResponse{
		Content:     content,
		Timestamp:   time.Now(),
		Suggestions: []string{},
	}
}

// ClearSession 清除会话
func (o *GraphOrchestrator) ClearSession(ctx context.Context, sessionID string) error {
	return o.memoryManager.ClearSession(ctx, sessionID)
}
