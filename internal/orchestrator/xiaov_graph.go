package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"video_agent/internal/agent"
	"video_agent/internal/mcp"
	"video_agent/internal/memory"
)

// XiaovInput 小V助手输入
type XiaovInput struct {
	SessionID string `json:"session_id"`
	UserID    string `json:"user_id"`
	Message   string `json:"message"`
}

// XiaovOutput 小V助手输出
type XiaovOutput struct {
	SessionID string                 `json:"session_id"`
	Reply     string                 `json:"reply"`
	Intent    string                 `json:"intent"`
	Agent     string                 `json:"agent"`
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// XiaovGraph 小V助手图编排器
type XiaovGraph struct {
	graph                compose.Runnable[XiaovInput, XiaovOutput]
	llm                  model.ChatModel
	intentAgent          *agent.IntentRecognitionAgent
	memoryManager        *memory.MemoryManager
	mcpManager           *mcp.Manager                // 远程MCP管理器
	videoAnalysisAgentV3 *agent.VideoAnalysisAgentV3 // V3视频分析Agent（用于流式处理）
}

// NewXiaovGraph 创建小V助手图编排器（MCP模式）
// mcpConfig: MCP配置
func NewXiaovGraph(
	llm model.ChatModel,
	intentAgent *agent.IntentRecognitionAgent,
	memoryManager *memory.MemoryManager,
	mcpConfig *mcp.ManagerConfig,
) (*XiaovGraph, error) {
	// 创建远程MCP管理器
	mcpManager, err := mcp.NewManager(mcpConfig)
	if err != nil {
		return nil, fmt.Errorf("创建MCP管理器失败: %w", err)
	}

	log.Printf("✅ [XiaovGraph] MCP模式初始化成功")

	// 创建视频分析Agent V3（基于ReAct Agent，LLM自动选择工具）
	videoAnalysisAgentV3, err := agent.NewVideoAnalysisAgentV3(llm, mcpManager)
	if err != nil {
		return nil, fmt.Errorf("创建视频分析Agent V3失败: %w", err)
	}

	xg := &XiaovGraph{
		llm:                  llm,
		intentAgent:          intentAgent,
		memoryManager:        memoryManager,
		mcpManager:           mcpManager,
		videoAnalysisAgentV3: videoAnalysisAgentV3,
	}

	if err := xg.buildGraph(videoAnalysisAgentV3); err != nil {
		return nil, err
	}

	return xg, nil
}

// buildGraph 构建图编排（使用MCP V3 Agent）
func (xg *XiaovGraph) buildGraph(videoAgentV3 *agent.VideoAnalysisAgentV3) error {
	ctx := context.Background()

	// 创建图
	g := compose.NewGraph[XiaovInput, XiaovOutput]()

	// 1. 意图识别节点
	intentNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovInput, error) {
		log.Printf("🔄 [图编排] 进入节点: intent (意图识别) | SessionID: %s", input.SessionID)
		log.Printf("📝 [图编排] 用户输入: %s", input.Message)

		// 识别意图
		intent, err := xg.intentAgent.Recognize(ctx, input.Message)
		if err != nil {
			log.Printf("⚠️ [图编排] 意图识别失败: %v, 使用通用对话", err)
			intent = &agent.Intent{
				Type:       agent.IntentGeneralChat,
				Confidence: 1.0,
				RawQuery:   input.Message,
			}
		}

		log.Printf("🎯 [图编排] 意图识别结果: type=%s, confidence=%.2f", intent.Type, intent.Confidence)

		// 存储用户消息到记忆
		userMemory := memory.Memory{
			ID:        uuid.New().String(),
			SessionID: input.SessionID,
			Content:   input.Message,
			Type:      memory.MemoryTypeUser,
			CreatedAt: time.Now(),
			Metadata: map[string]interface{}{
				"user_id": input.UserID,
				"intent":  string(intent.Type),
			},
		}
		xg.memoryManager.Store(ctx, userMemory)

		// 将意图存储在Message字段中传递
		intentJSON, _ := json.Marshal(intent)
		input.Message = string(intentJSON) + "|||" + input.Message

		log.Printf("➡️ [图编排] 离开节点: intent -> router")
		return input, nil
	})

	// 2. 分支路由节点
	routerNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovInput, error) {
		log.Printf("🔄 [图编排] 进入节点: router (分支路由) | SessionID: %s", input.SessionID)

		// 解析意图
		var intent agent.Intent
		parts := splitMessage(input.Message)
		if len(parts) == 2 {
			json.Unmarshal([]byte(parts[0]), &intent)
			input.Message = parts[1]
		}

		log.Printf("🎯 [图编排] 路由决策: intent_type=%s", intent.Type)
		input.SessionID = input.SessionID + "#" + string(intent.Type)

		log.Printf("➡️ [图编排] 离开节点: router -> [分支选择]")
		return input, nil
	})

	// 3. 知识库Agent节点
	knowledgeNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: knowledge (知识库Agent)")
		reply := xg.handleKnowledgeBase(ctx, input)
		return xg.buildOutput(input, reply, "knowledge_base"), nil
	})

	// 4. 创作分析Agent节点（使用V3 Agent）
	creationNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: creation (创作分析Agent)")
		reply := xg.handleVideoAnalysisWithMCP(ctx, input, videoAgentV3)
		return xg.buildOutput(input, reply, "content_creation"), nil
	})

	// 5. 视频分析Agent节点（使用V3 Agent）
	videoNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: video (视频分析Agent-V3) | SessionID: %s", input.SessionID)
		log.Printf("📝 [图编排] 处理消息: %s", input.Message)

		// 使用V3 Agent（ReAct Agent，LLM自动选择工具）
		reply := xg.handleVideoAnalysisWithMCP(ctx, input, videoAgentV3)

		log.Printf("✅ [图编排] 视频分析处理完成 | 回复长度: %d", len(reply))
		log.Printf("➡️ [图编排] 离开节点: video -> END")
		return xg.buildOutput(input, reply, "video_analysis"), nil
	})

	// 6. 通用对话Agent节点
	generalNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: general (通用对话Agent)")
		reply := xg.handleGeneralChat(ctx, input)
		return xg.buildOutput(input, reply, "general_chat"), nil
	})

	// 添加节点
	g.AddLambdaNode("intent", intentNode)
	g.AddLambdaNode("router", routerNode)
	g.AddLambdaNode("knowledge", knowledgeNode)
	g.AddLambdaNode("creation", creationNode)
	g.AddLambdaNode("video", videoNode)
	g.AddLambdaNode("general", generalNode)

	// 添加边
	g.AddEdge(compose.START, "intent")
	g.AddEdge("intent", "router")

	// 添加分支
	g.AddBranch("router", compose.NewGraphBranch(
		func(ctx context.Context, input XiaovInput) (string, error) {
			intentType := extractIntentFromSessionID(input.SessionID)

			var targetNode string
			switch agent.IntentType(intentType) {
			case agent.IntentKnowledgeBase, agent.IntentKnowledgeQA:
				targetNode = "knowledge"
			case agent.IntentContentCreation, agent.IntentTopicAnalysis:
				targetNode = "creation"
			case agent.IntentVideoAnalysis:
				targetNode = "video"
			default:
				targetNode = "general"
			}

			log.Printf("🔀 [图编排] 分支路由决策: intent=%s -> target_node=%s", intentType, targetNode)
			return targetNode, nil
		},
		map[string]bool{
			"knowledge": true,
			"creation":  true,
			"video":     true,
			"general":   true,
		},
	))

	// 所有Agent节点都连接到END
	g.AddEdge("knowledge", compose.END)
	g.AddEdge("creation", compose.END)
	g.AddEdge("video", compose.END)
	g.AddEdge("general", compose.END)

	// 编译图
	runnable, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("编译图失败: %w", err)
	}

	xg.graph = runnable
	return nil
}

// Execute 执行图编排
func (xg *XiaovGraph) Execute(ctx context.Context, input XiaovInput) (*XiaovOutput, error) {
	if input.SessionID == "" {
		input.SessionID = uuid.New().String()
	}

	log.Printf("🚀 [图编排] ========== 开始执行图编排 ==========")
	log.Printf("🚀 [图编排] SessionID: %s | UserID: %s", input.SessionID, input.UserID)
	log.Printf("🚀 [图编排] 用户消息: %s", input.Message)
	log.Printf("🚀 [图编排] 图结构: START -> intent -> router -> [分支] -> Agent -> END")

	startTime := time.Now()
	output, err := xg.graph.Invoke(ctx, input)
	elapsed := time.Since(startTime)

	if err != nil {
		log.Printf("❌ [图编排] ========== 图编排执行失败 ==========")
		log.Printf("❌ [图编排] 错误: %v | 耗时: %v", err, elapsed)
		return nil, err
	}

	log.Printf("✅ [图编排] ========== 图编排执行完成 ==========")
	log.Printf("✅ [图编排] 意图: %s | Agent: %s | 耗时: %v", output.Intent, output.Agent, elapsed)
	log.Printf("✅ [图编排] 回复长度: %d", len(output.Reply))

	return &output, nil
}

// StreamAnalyzeVideo 流式分析视频（用于ChatStream接口）
// 返回流式读取器，可以实时获取分析结果
func (xg *XiaovGraph) StreamAnalyzeVideo(ctx context.Context, input XiaovInput) (*schema.StreamReader[*schema.Message], error) {
	// 从消息中提取视频ID
	videoID := xg.extractVideoID(input.Message)
	if videoID == "" {
		return nil, fmt.Errorf("请提供要分析的视频ID，例如：\"分析视频 BV123456\"")
	}

	log.Printf("🎬 [图编排-流式] 开始流式分析视频 | VideoID: %s", videoID)

	// 调用V3 Agent的流式分析方法
	streamReader, err := xg.videoAnalysisAgentV3.StreamAnalyze(ctx, videoID, input.Message)
	if err != nil {
		log.Printf("❌ [图编排-流式] 流式分析失败: %v", err)
		return nil, fmt.Errorf("视频流式分析失败: %w", err)
	}

	return streamReader, nil
}

// handleKnowledgeBase 处理知识库意图
func (xg *XiaovGraph) handleKnowledgeBase(ctx context.Context, input XiaovInput) string {
	// TODO: 调用RAG知识库检索
	return fmt.Sprintf("【知识库模式】收到您的问题：%s。正在检索知识库...", input.Message)
}

// handleGeneralChat 处理通用对话意图
func (xg *XiaovGraph) handleGeneralChat(ctx context.Context, input XiaovInput) string {
	// 获取历史记忆
	memories, _ := xg.memoryManager.GetSessionHistory(ctx, input.SessionID, 10)

	// 构建对话历史
	var history string
	for _, mem := range memories {
		if mem.Type == memory.MemoryTypeUser {
			history += fmt.Sprintf("用户: %s\n", mem.Content)
		} else {
			history += fmt.Sprintf("助手: %s\n", mem.Content)
		}
	}

	// 调用LLM生成回复
	prompt := fmt.Sprintf(`你是小V助手，一个专业的视频内容分析助手。

对话历史：
%s

用户当前问题：%s

请给出友好、专业的回复。`, history, input.Message)

	messages := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}

	response, err := xg.llm.Generate(ctx, messages)
	if err != nil {
		log.Printf("❌ [通用对话] LLM调用失败: %v", err)
		return "抱歉，我暂时无法回答，请稍后再试。"
	}

	// 存储助手回复到记忆
	assistantMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: input.SessionID,
		Content:   response.Content,
		Type:      memory.MemoryTypeAssistant,
		CreatedAt: time.Now(),
	}
	xg.memoryManager.Store(ctx, assistantMemory)

	return response.Content
}

// handleVideoAnalysisWithMCP 使用V3 Agent处理视频分析（LLM自动选择工具）
func (xg *XiaovGraph) handleVideoAnalysisWithMCP(ctx context.Context, input XiaovInput, videoAgentV3 *agent.VideoAnalysisAgentV3) string {
	// 从消息中提取视频ID
	videoID := xg.extractVideoID(input.Message)
	if videoID == "" {
		return "请提供要分析的视频ID，例如：\"分析视频 BV123456\""
	}

	log.Printf("🎬 [视频分析-MCP] 使用V3 Agent分析视频 | VideoID: %s", videoID)

	// 调用V3 Agent（ReAct Agent自动选择工具）
	analysis, err := videoAgentV3.Analyze(ctx, videoID, input.Message)
	if err != nil {
		log.Printf("❌ [视频分析-MCP] V3 Agent分析失败: %v", err)
		return fmt.Sprintf("视频分析失败：%s", err.Error())
	}

	return analysis
}

// extractVideoID 从消息中提取视频ID
func (xg *XiaovGraph) extractVideoID(message string) string {
	// 匹配
	bvPattern := regexp.MustCompile(`[a-zA-Z0-9]{10}`)
	if match := bvPattern.FindString(message); match != "" {
		return match
	}

	// 匹配
	avPattern := regexp.MustCompile(`[Aa][Vv]\d+`)
	if match := avPattern.FindString(message); match != "" {
		return match
	}

	// 匹配URL中的视频ID
	urlPattern := regexp.MustCompile(`(?:bilibili\.com/video/)([Bb][Vv][a-zA-Z0-9]{10})`)
	matches := urlPattern.FindStringSubmatch(message)
	if len(matches) > 1 {
		return matches[1]
	}

	return ""
}

// buildOutput 构建输出
func (xg *XiaovGraph) buildOutput(input XiaovInput, reply string, agentType string) XiaovOutput {
	// 从SessionID中提取意图
	intentType := extractIntentFromSessionID(input.SessionID)

	return XiaovOutput{
		SessionID: input.SessionID,
		Reply:     reply,
		Intent:    intentType,
		Agent:     agentType,
		Timestamp: time.Now().Unix(),
		Metadata: map[string]interface{}{
			"user_id": input.UserID,
		},
	}
}

// splitMessage 分割消息（意图JSON|||原始消息）
func splitMessage(message string) []string {
	// 查找分隔符位置
	idx := 0
	for i := 0; i < len(message)-2; i++ {
		if message[i] == '|' && message[i+1] == '|' && message[i+2] == '|' {
			idx = i
			break
		}
	}

	if idx == 0 {
		return []string{message}
	}

	return []string{message[:idx], message[idx+3:]}
}

// extractIntentFromSessionID 从SessionID中提取意图类型
func extractIntentFromSessionID(sessionID string) string {
	// SessionID格式: uuid#intent_type
	parts := splitMessage(sessionID)
	if len(parts) > 0 {
		sessionID = parts[0]
	}

	// 查找#分隔符
	for i := len(sessionID) - 1; i >= 0; i-- {
		if sessionID[i] == '#' {
			return sessionID[i+1:]
		}
	}
	return "general_chat"
}
