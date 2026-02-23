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
	graph              compose.Runnable[XiaovInput, XiaovOutput]
	llm                model.ChatModel
	intentAgent        *agent.IntentRecognitionAgent
	memoryManager      *memory.MemoryManager
	toolRegistry       *mcp.Registry
	videoAnalysisAgent *agent.VideoAnalysisAgentV2
}

// NewXiaovGraph 创建小V助手图编排器
// gatewayURL: Gateway服务地址，如 "http://localhost:8080"
func NewXiaovGraph(
	llm model.ChatModel,
	intentAgent *agent.IntentRecognitionAgent,
	memoryManager *memory.MemoryManager,
	gatewayURL string,
) (*XiaovGraph, error) {
	// 创建MCP工具注册表
	toolRegistry := mcp.NewRegistry()
	toolRegistry.RegisterDefaultTools()

	// 注册Gateway网关层视频工具（真实调用Gateway的getVideoDetail）
	if gatewayURL != "" {
		gatewayTool := mcp.NewGatewayVideoTool(gatewayURL)
		if err := toolRegistry.Register(gatewayTool); err != nil {
			log.Printf("⚠️ [XiaovGraph] 注册Gateway工具失败: %v", err)
		} else {
			log.Printf("✅ [XiaovGraph] 注册Gateway视频工具成功 | URL: %s", gatewayURL)
		}
	} else {
		// 如果没有Gateway地址，注册模拟工具
		log.Printf("⚠️ [XiaovGraph] 未配置Gateway地址，使用模拟工具")
		agent.RegisterMCPTools(toolRegistry)
	}

	// 创建视频分析Agent
	videoAnalysisAgent := agent.NewVideoAnalysisAgentV2(llm, toolRegistry)

	xg := &XiaovGraph{
		llm:                llm,
		intentAgent:        intentAgent,
		memoryManager:      memoryManager,
		toolRegistry:       toolRegistry,
		videoAnalysisAgent: videoAnalysisAgent,
	}

	if err := xg.buildGraph(); err != nil {
		return nil, err
	}

	return xg, nil
}

// buildGraph 构建图编排
func (xg *XiaovGraph) buildGraph() error {
	ctx := context.Background()

	// 创建图
	g := compose.NewGraph[XiaovInput, XiaovOutput]()

	// 1. 意图识别节点
	intentNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovInput, error) {
		log.Printf("🔄 [图编排] 进入节点: intent (意图识别) | SessionID: %s | UserID: %s", input.SessionID, input.UserID)
		log.Printf("📝 [图编排] 用户输入: %s", input.Message)

		// 识别意图
		intent, err := xg.intentAgent.Recognize(ctx, input.Message)
		if err != nil {
			log.Printf("⚠️ [图编排] 意图识别失败: %v, 使用通用对话", err)
			// 意图识别失败，使用通用对话
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

		// 将意图存储在Message字段中传递（临时方案）
		intentJSON, _ := json.Marshal(intent)
		input.Message = string(intentJSON) + "|||" + input.Message

		log.Printf("➡️ [图编排] 离开节点: intent -> router")
		return input, nil
	})

	// 2. 分支路由节点 - 根据意图类型路由到不同处理节点
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

		// 将意图类型编码到SessionID中传递（临时方案）
		input.SessionID = input.SessionID + "#" + string(intent.Type)

		log.Printf("➡️ [图编排] 离开节点: router -> [分支选择]")
		return input, nil
	})

	// 3. 知识库Agent节点
	knowledgeNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: knowledge (知识库Agent) | SessionID: %s", extractSessionID(input.SessionID))
		log.Printf("📝 [图编排] 处理消息: %s", input.Message)

		// 调用知识库处理
		reply := xg.handleKnowledgeBase(ctx, input)

		log.Printf("✅ [图编排] 知识库处理完成 | 回复长度: %d", len(reply))
		log.Printf("➡️ [图编排] 离开节点: knowledge -> END")
		return xg.buildOutput(input, reply, "knowledge_base"), nil
	})

	// 4. 创作分析Agent节点 - 使用视频分析Agent V2
	creationNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: creation (创作分析Agent) | SessionID: %s", extractSessionID(input.SessionID))
		log.Printf("📝 [图编排] 处理消息: %s", input.Message)

		// 调用视频分析Agent V2 (包含MCP工具调用 + LLM分析)
		reply := xg.handleVideoAnalysis(ctx, input)

		log.Printf("✅ [图编排] 创作分析处理完成 | 回复长度: %d", len(reply))
		log.Printf("➡️ [图编排] 离开节点: creation -> END")
		return xg.buildOutput(input, reply, "content_creation"), nil
	})

	// 5. 视频分析Agent节点
	videoNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: video (视频分析Agent) | SessionID: %s", extractSessionID(input.SessionID))
		log.Printf("📝 [图编排] 处理消息: %s", input.Message)

		// 调用视频分析处理
		reply := xg.handleVideoAnalysisWithAgent(ctx, input)

		log.Printf("✅ [图编排] 视频分析处理完成 | 回复长度: %d", len(reply))
		log.Printf("➡️ [图编排] 离开节点: video -> END")
		return xg.buildOutput(input, reply, "video_analysis"), nil
	})

	// 6. 通用对话Agent节点（默认）
	generalNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (XiaovOutput, error) {
		log.Printf("🔄 [图编排] 进入节点: general (通用对话Agent) | SessionID: %s", extractSessionID(input.SessionID))
		log.Printf("📝 [图编排] 处理消息: %s", input.Message)

		// 调用通用对话处理
		reply := xg.handleGeneralChat(ctx, input)

		log.Printf("✅ [图编排] 通用对话处理完成 | 回复长度: %d", len(reply))
		log.Printf("➡️ [图编排] 离开节点: general -> END")
		return xg.buildOutput(input, reply, "general_chat"), nil
	})

	// 添加节点
	g.AddLambdaNode("intent", intentNode)
	g.AddLambdaNode("router", routerNode)
	g.AddLambdaNode("knowledge", knowledgeNode)
	g.AddLambdaNode("creation", creationNode)
	g.AddLambdaNode("video", videoNode)
	g.AddLambdaNode("general", generalNode)

	// 添加边：START -> intent -> router
	g.AddEdge(compose.START, "intent")
	g.AddEdge("intent", "router")

	// 添加分支：router -> 不同Agent
	g.AddBranch("router", compose.NewGraphBranch(
		func(ctx context.Context, input XiaovInput) (string, error) {
			// 从SessionID中解析意图类型
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

// handleKnowledgeBase 处理知识库意图
func (xg *XiaovGraph) handleKnowledgeBase(ctx context.Context, input XiaovInput) string {
	// TODO: 调用RAG知识库检索
	// 临时返回示例回复
	return fmt.Sprintf("【知识库模式】收到您的问题：%s。正在检索知识库...", input.Message)
}

// handleContentCreation 处理创作分析意图
func (xg *XiaovGraph) handleContentCreation(ctx context.Context, input XiaovInput) string {
	// TODO: 调用创作分析Agent
	// 临时返回示例回复
	return fmt.Sprintf("【创作分析模式】收到您的创作需求：%s。正在分析...", input.Message)
}

// handleVideoAnalysis 处理视频分析意图
func (xg *XiaovGraph) handleVideoAnalysis(ctx context.Context, input XiaovInput) string {
	// TODO: 调用视频分析Agent
	// 临时返回示例回复
	return fmt.Sprintf("【视频分析模式】收到视频分析请求：%s。正在处理...", input.Message)
}

// handleVideoAnalysisWithAgent 使用视频分析Agent V2进行专业分析
func (xg *XiaovGraph) handleVideoAnalysisWithAgent(ctx context.Context, input XiaovInput) string {
	// 从用户消息中提取视频ID
	videoID := xg.extractVideoID(input.Message)
	if videoID == "" {
		return "请提供要分析的视频ID，例如：\"分析视频 BV123456\""
	}

	log.Printf("🎬 [视频分析] 提取到视频ID: %s", videoID)

	// 构建分析请求
	req := &agent.VideoAnalysisRequest{
		VideoID:      videoID,
		Query:        input.Message,
		AnalysisType: "all", // 进行全面分析
	}

	// 调用视频分析Agent V2
	resp, err := xg.videoAnalysisAgent.Analyze(ctx, req)
	if err != nil {
		log.Printf("❌ [视频分析] 分析失败: %v", err)
		return fmt.Sprintf("视频分析失败：%s", err.Error())
	}

	// 格式化分析结果
	result := fmt.Sprintf(`【视频分析报告】

📹 视频信息
- 标题：%s
- 视频ID：%s
- 处理耗时：%dms

📝 视频摘要
%s

🔍 详细分析
%s

💭 情感倾向：%s

📌 关键要点：
`, resp.Title, resp.VideoID, resp.ProcessingTime, resp.Summary, resp.Content, resp.Sentiment)

	for i, point := range resp.KeyPoints {
		result += fmt.Sprintf("%d. %s\n", i+1, point)
	}

	result += "\n🏷️ 相关标签："
	for _, tag := range resp.Tags {
		result += fmt.Sprintf(" #%s", tag)
	}

	result += "\n\n💡 优化建议：\n"
	for i, suggestion := range resp.Suggestions {
		result += fmt.Sprintf("%d. %s\n", i+1, suggestion)
	}

	return result
}

// extractVideoID 从消息中提取视频ID
func (xg *XiaovGraph) extractVideoID(message string) string {
	// 匹配
	patterns := []string{
		`[a-zA-Z0-9]{10}`, // "video id: xxx" 或 "videoxxx"
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindString(message); matches != "" {
			return matches
		}
	}

	// 如果没有匹配到，尝试提取最后一个单词作为ID
	words := regexp.MustCompile(`\S+`).FindAllString(message, -1)
	if len(words) > 0 {
		lastWord := words[len(words)-1]
		// 如果最后一个单词看起来像ID（长度适中，包含字母数字）
		if len(lastWord) >= 6 && len(lastWord) <= 20 {
			return lastWord
		}
	}

	return ""
}

// handleGeneralChat 处理通用对话意图
func (xg *XiaovGraph) handleGeneralChat(ctx context.Context, input XiaovInput) string {
	// 构建消息列表
	messages := []*schema.Message{
		schema.SystemMessage("你是小V助手，一个智能AI助手。请根据用户的问题提供有帮助、准确且友好的回答。"),
		schema.UserMessage(input.Message),
	}

	// 调用LLM生成回复
	response, err := xg.llm.Generate(ctx, messages)
	if err != nil {
		return "抱歉，我暂时无法回答您的问题，请稍后再试。"
	}

	return response.Content
}

// buildOutput 构建输出
func (xg *XiaovGraph) buildOutput(input XiaovInput, reply, agentType string) XiaovOutput {
	// 存储助手回复到记忆
	assistantMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: extractSessionID(input.SessionID),
		Content:   reply,
		Type:      memory.MemoryTypeAssistant,
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"user_id": input.UserID,
			"agent":   agentType,
		},
	}
	xg.memoryManager.Store(context.Background(), assistantMemory)

	intentType := extractIntentFromSessionID(input.SessionID)

	return XiaovOutput{
		SessionID: extractSessionID(input.SessionID),
		Reply:     reply,
		Intent:    intentType,
		Agent:     agentType,
		Timestamp: time.Now().UnixMilli(),
		Metadata: map[string]interface{}{
			"user_id": input.UserID,
		},
	}
}

// splitMessage 分割消息
func splitMessage(msg string) []string {
	for i := 0; i < len(msg)-3; i++ {
		if msg[i:i+3] == "|||" {
			return []string{msg[:i], msg[i+3:]}
		}
	}
	return []string{msg}
}

// extractIntentFromSessionID 从SessionID中提取意图
func extractIntentFromSessionID(sessionID string) string {
	for i := len(sessionID) - 1; i >= 0; i-- {
		if sessionID[i] == '#' {
			return sessionID[i+1:]
		}
	}
	return "general_chat"
}

// extractSessionID 提取原始SessionID
func extractSessionID(sessionID string) string {
	for i := len(sessionID) - 1; i >= 0; i-- {
		if sessionID[i] == '#' {
			return sessionID[:i]
		}
	}
	return sessionID
}
