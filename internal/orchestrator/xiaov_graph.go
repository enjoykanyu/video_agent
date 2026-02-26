package orchestrator

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"github.com/google/uuid"

	"video_agent/internal/agent"
	"video_agent/internal/mcp"
	"video_agent/internal/memory"
)

// =============================================================================
// 类型定义
// =============================================================================

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

// GraphState 图编排状态，在节点间传递
type GraphState struct {
	SessionID        string                 `json:"session_id"`
	UserID           string                 `json:"user_id"`
	OriginalMessage  string                 `json:"original_message"`
	Intent           agent.IntentType       `json:"intent"`
	IntentConfidence float64                `json:"intent_confidence"`
	VideoID          string                 `json:"video_id,omitempty"`
	SelectedTools    []ToolSelection        `json:"selected_tools"`
	ToolResults      []ToolExecutionResult  `json:"tool_results"`
	AnalysisResult   string                 `json:"analysis_result"`
	FinalReply       string                 `json:"final_reply"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// ToolSelection 工具选择结果
type ToolSelection struct {
	Name       string                 `json:"name"`
	Params     map[string]interface{} `json:"params"`
	Reason     string                 `json:"reason"`
	Confidence float64                `json:"confidence"`
}

// ToolExecutionResult 工具执行结果
type ToolExecutionResult struct {
	ToolName   string      `json:"tool_name"`
	Params     interface{} `json:"params"`
	Result     interface{} `json:"result"`
	Error      string      `json:"error,omitempty"`
	StartTime  time.Time   `json:"start_time"`
	EndTime    time.Time   `json:"end_time"`
	DurationMs int64       `json:"duration_ms"`
}

// XiaovGraph 小V助手图编排器
type XiaovGraph struct {
	graph         compose.Runnable[XiaovInput, XiaovOutput]
	llm           model.ChatModel
	intentAgent   *agent.IntentRecognitionAgent
	memoryManager *memory.MemoryManager
	mcpManager    *mcp.Manager
}

// =============================================================================
// 构造函数
// =============================================================================

// NewXiaovGraph 创建小V助手图编排器
func NewXiaovGraph(
	llm model.ChatModel,
	intentAgent *agent.IntentRecognitionAgent,
	memoryManager *memory.MemoryManager,
	mcpConfig *mcp.ManagerConfig,
) (*XiaovGraph, error) {
	mcpManager, err := mcp.NewManager(mcpConfig)
	if err != nil {
		return nil, fmt.Errorf("创建MCP管理器失败: %w", err)
	}

	log.Printf("✅ [XiaovGraph] 企业级图编排器初始化成功")

	xg := &XiaovGraph{
		llm:           llm,
		intentAgent:   intentAgent,
		memoryManager: memoryManager,
		mcpManager:    mcpManager,
	}

	if err := xg.buildGraph(); err != nil {
		return nil, err
	}

	return xg, nil
}

// =============================================================================
// 图编排构建
// =============================================================================

// buildGraph 构建图编排
func (xg *XiaovGraph) buildGraph() error {
	ctx := context.Background()

	// 创建状态图，使用 GraphState 作为状态传递
	g := compose.NewGraph[XiaovInput, XiaovOutput]()

	// 1. 意图识别节点
	intentNode := compose.InvokableLambda(func(ctx context.Context, input XiaovInput) (GraphState, error) {
		return xg.intentRecognitionNode(ctx, input)
	})

	// 2. 工具选择节点（动态选择 MCP Tool）
	toolSelectionNode := compose.InvokableLambda(func(ctx context.Context, state GraphState) (GraphState, error) {
		return xg.toolSelectionNode(ctx, state)
	})

	// 3. MCP Tool 调用节点
	toolExecutionNode := compose.InvokableLambda(func(ctx context.Context, state GraphState) (GraphState, error) {
		return xg.toolExecutionNode(ctx, state)
	})

	// 4. 分析 Agent 节点
	analysisNode := compose.InvokableLambda(func(ctx context.Context, state GraphState) (GraphState, error) {
		return xg.analysisNode(ctx, state)
	})

	// 5. 输出总结节点
	summaryNode := compose.InvokableLambda(func(ctx context.Context, state GraphState) (XiaovOutput, error) {
		return xg.summaryNode(ctx, state)
	})

	// 添加节点到图
	g.AddLambdaNode("intent", intentNode)
	g.AddLambdaNode("tool_selection", toolSelectionNode)
	g.AddLambdaNode("tool_execution", toolExecutionNode)
	g.AddLambdaNode("analysis", analysisNode)
	g.AddLambdaNode("summary", summaryNode)

	// 添加边：顺序执行
	g.AddEdge(compose.START, "intent")
	g.AddEdge("intent", "tool_selection")
	g.AddEdge("tool_selection", "tool_execution")
	g.AddEdge("tool_execution", "analysis")
	g.AddEdge("analysis", "summary")
	g.AddEdge("summary", compose.END)

	// 编译图
	runnable, err := g.Compile(ctx)
	if err != nil {
		return fmt.Errorf("编译图编排失败: %w", err)
	}

	xg.graph = runnable
	return nil
}

// =============================================================================
// 节点实现
// =============================================================================

// intentRecognitionNode 意图识别节点
func (xg *XiaovGraph) intentRecognitionNode(ctx context.Context, input XiaovInput) (GraphState, error) {
	log.Printf("🎯 [节点1:意图识别] SessionID: %s", input.SessionID)
	startTime := time.Now()

	// 识别意图
	intent, err := xg.intentAgent.Recognize(ctx, input.Message)
	if err != nil {
		log.Printf("⚠️ [意图识别] 失败: %v, 使用通用对话", err)
		intent = &agent.Intent{
			Type:       agent.IntentGeneralChat,
			Confidence: 1.0,
			RawQuery:   input.Message,
		}
	}

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

	// 提取视频ID（如果有）
	videoID := xg.extractVideoID(input.Message)

	state := GraphState{
		SessionID:        input.SessionID,
		UserID:           input.UserID,
		OriginalMessage:  input.Message,
		Intent:           intent.Type,
		IntentConfidence: intent.Confidence,
		VideoID:          videoID,
		Metadata: map[string]interface{}{
			"intent_recognition_duration_ms": time.Since(startTime).Milliseconds(),
		},
	}

	log.Printf("🎯 [意图识别] 结果: type=%s, confidence=%.2f, videoID=%s, 耗时: %v",
		intent.Type, intent.Confidence, videoID, time.Since(startTime))

	return state, nil
}

// toolSelectionNode 工具选择节点
func (xg *XiaovGraph) toolSelectionNode(ctx context.Context, state GraphState) (GraphState, error) {
	log.Printf("🔧 [节点2:工具选择] SessionID: %s, Intent: %s", state.SessionID, state.Intent)
	startTime := time.Now()

	// 通用对话不需要工具
	if state.Intent == agent.IntentGeneralChat {
		log.Printf("🔧 [工具选择] 通用对话，跳过工具选择")
		state.Metadata["tool_selection_skipped"] = true
		return state, nil
	}

	// 获取可用的 MCP 工具列表
	availableTools, err := xg.mcpManager.GetTools(ctx)
	if err != nil {
		log.Printf("⚠️ [工具选择] 获取工具列表失败: %v", err)
		state.Metadata["tool_selection_error"] = err.Error()
		return state, nil
	}

	// 构建工具描述
	toolsDesc := xg.buildToolsDescription(availableTools)

	// 调用 LLM 进行工具选择
	selectedTools, err := xg.selectToolsWithLLM(ctx, state, toolsDesc)
	if err != nil {
		log.Printf("⚠️ [工具选择] LLM选择失败: %v", err)
		state.Metadata["tool_selection_error"] = err.Error()
	} else {
		state.SelectedTools = selectedTools
	}

	state.Metadata["tool_selection_duration_ms"] = time.Since(startTime).Milliseconds()
	state.Metadata["selected_tools_count"] = len(state.SelectedTools)

	log.Printf("🔧 [工具选择] 完成，选择 %d 个工具，耗时: %v",
		len(state.SelectedTools), time.Since(startTime))

	return state, nil
}

// toolExecutionNode MCP Tool 调用节点
func (xg *XiaovGraph) toolExecutionNode(ctx context.Context, state GraphState) (GraphState, error) {
	log.Printf("⚙️ [节点3:工具执行] SessionID: %s", state.SessionID)
	startTime := time.Now()

	// 通用对话或没有选中工具，跳过执行
	if state.Intent == agent.IntentGeneralChat || len(state.SelectedTools) == 0 {
		log.Printf("⚙️ [工具执行] 跳过工具执行")
		state.Metadata["tool_execution_skipped"] = true
		return state, nil
	}

	toolResults := make([]ToolExecutionResult, 0, len(state.SelectedTools))

	for _, toolSelection := range state.SelectedTools {
		toolStartTime := time.Now()

		log.Printf("⚙️ [工具执行] 调用工具: %s | 参数: %v", toolSelection.Name, toolSelection.Params)

		// 执行工具
		result, err := xg.mcpManager.ExecuteTool(ctx, toolSelection.Name, toolSelection.Params)

		toolEndTime := time.Now()
		durationMs := toolEndTime.Sub(toolStartTime).Milliseconds()

		toolResult := ToolExecutionResult{
			ToolName:   toolSelection.Name,
			Params:     toolSelection.Params,
			Result:     result,
			StartTime:  toolStartTime,
			EndTime:    toolEndTime,
			DurationMs: durationMs,
		}

		if err != nil {
			toolResult.Error = err.Error()
			log.Printf("❌ [工具执行] 工具 %s 失败: %v", toolSelection.Name, err)
		} else {
			log.Printf("✅ [工具执行] 工具 %s 成功，耗时: %dms", toolSelection.Name, durationMs)
		}

		toolResults = append(toolResults, toolResult)
	}

	state.ToolResults = toolResults
	state.Metadata["tool_execution_duration_ms"] = time.Since(startTime).Milliseconds()
	state.Metadata["tool_execution_count"] = len(toolResults)

	log.Printf("⚙️ [工具执行] 完成，执行 %d 个工具，耗时: %v",
		len(toolResults), time.Since(startTime))

	return state, nil
}

// analysisNode 分析 Agent 节点
func (xg *XiaovGraph) analysisNode(ctx context.Context, state GraphState) (GraphState, error) {
	log.Printf("📝 [节点4:分析Agent] SessionID: %s, Intent: %s", state.SessionID, state.Intent)
	startTime := time.Now()

	// 通用对话不需要分析
	if state.Intent == agent.IntentGeneralChat {
		log.Printf("📝 [分析Agent] 通用对话，跳过分析")
		state.Metadata["analysis_skipped"] = true
		return state, nil
	}

	// 根据意图类型选择不同的分析策略
	var analysisResult string
	var err error

	switch state.Intent {
	case agent.IntentVideoAnalysis:
		analysisResult, err = xg.performVideoAnalysis(ctx, state)
	case agent.IntentContentCreation:
		analysisResult, err = xg.performContentCreationAnalysis(ctx, state)
	case agent.IntentWeeklyReport:
		analysisResult, err = xg.performWeeklyReportAnalysis(ctx, state)
	case agent.IntentTopicAnalysis:
		analysisResult, err = xg.performTopicAnalysis(ctx, state)
	default:
		analysisResult, err = xg.performGenericAnalysis(ctx, state)
	}

	if err != nil {
		log.Printf("⚠️ [分析Agent] 分析失败: %v，使用降级方案", err)
		analysisResult = xg.buildFallbackAnalysis(state)
	}

	state.AnalysisResult = analysisResult
	state.Metadata["analysis_duration_ms"] = time.Since(startTime).Milliseconds()

	log.Printf("📝 [分析Agent] 完成，耗时: %v", time.Since(startTime))

	return state, nil
}

// summaryNode 输出总结节点
func (xg *XiaovGraph) summaryNode(ctx context.Context, state GraphState) (XiaovOutput, error) {
	log.Printf("📤 [节点5:输出总结] SessionID: %s", state.SessionID)
	startTime := time.Now()

	var finalReply string
	var agentType string

	switch state.Intent {
	case agent.IntentGeneralChat:
		finalReply, _ = xg.generateGeneralChatResponse(ctx, state)
		agentType = "general_chat"
	default:
		if state.AnalysisResult != "" {
			finalReply = state.AnalysisResult
		} else {
			finalReply = xg.buildFallbackResponse(state)
		}
		agentType = string(state.Intent)
	}

	// 存储助手回复到记忆
	assistantMemory := memory.Memory{
		ID:        uuid.New().String(),
		SessionID: state.SessionID,
		Content:   finalReply,
		Type:      memory.MemoryTypeAssistant,
		CreatedAt: time.Now(),
		Metadata: map[string]interface{}{
			"user_id": state.UserID,
			"intent":  string(state.Intent),
			"agent":   agentType,
		},
	}
	xg.memoryManager.Store(ctx, assistantMemory)

	output := XiaovOutput{
		SessionID: state.SessionID,
		Reply:     finalReply,
		Intent:    string(state.Intent),
		Agent:     agentType,
		Timestamp: time.Now().UnixMilli(),
		Metadata:  state.Metadata,
	}

	log.Printf("📤 [输出总结] 完成，总耗时: %v", time.Since(startTime))

	return output, nil
}

// =============================================================================
// 工具方法
// =============================================================================

// buildToolsDescription 构建工具描述
func (xg *XiaovGraph) buildToolsDescription(tools []tool.BaseTool) string {
	desc := ""
	for i, t := range tools {
		info, err := t.Info(context.Background())
		if err != nil {
			continue
		}
		desc += fmt.Sprintf("%d. %s - %s\n", i+1, info.Name, info.Desc)
	}
	return desc
}

// selectToolsWithLLM 使用 LLM 选择工具
func (xg *XiaovGraph) selectToolsWithLLM(ctx context.Context, state GraphState, toolsDesc string) ([]ToolSelection, error) {
	selectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	prompt := fmt.Sprintf(`你是一位工具选择专家。根据用户的需求，选择需要调用的MCP工具。

用户意图: %s
用户消息: %s
视频ID: %s

可用工具：
%s

请输出需要调用的工具列表（严格JSON格式）：
{
  "tools": [
    {
      "name": "工具名称",
      "params": {"参数名": "参数值"},
      "reason": "选择原因",
      "confidence": 0.95
    }
  ]
}

重要提示：
1. name 必须从可用工具列表中选择
2. params 中的参数名必须是工具定义中的实际参数名
3. 如果用户提供了视频ID（%s），请在参数中使用它
4. 只选择真正需要的工具`,
		state.Intent, state.OriginalMessage, state.VideoID, toolsDesc, state.VideoID)

	messages := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}

	response, err := xg.llm.Generate(selectCtx, messages)
	if err != nil {
		return nil, fmt.Errorf("LLM工具选择失败: %w", err)
	}

	return xg.parseToolSelectionResponse(response.Content)
}

// parseToolSelectionResponse 解析工具选择响应
func (xg *XiaovGraph) parseToolSelectionResponse(content string) ([]ToolSelection, error) {
	startIdx := strings.Index(content, "{")
	endIdx := strings.LastIndex(content, "}")
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil, fmt.Errorf("无法找到JSON内容")
	}

	jsonStr := content[startIdx : endIdx+1]

	var result struct {
		Tools []ToolSelection `json:"tools"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("解析工具选择JSON失败: %w", err)
	}

	// 验证和修复工具选择
	for i := range result.Tools {
		tool := &result.Tools[i]
		if tool.Params == nil {
			tool.Params = make(map[string]interface{})
		}
		// 修复常见的参数名错误
		if val, ok := tool.Params["参数值"]; ok {
			tool.Params["video_id"] = val
			delete(tool.Params, "参数值")
		}
	}

	return result.Tools, nil
}

// extractVideoID 从消息中提取视频ID
func (xg *XiaovGraph) extractVideoID(message string) string {
	// 匹配 BV 号
	bvPattern := regexp.MustCompile(`[Bb][Vv][a-zA-Z0-9]{10}`)
	if match := bvPattern.FindString(message); match != "" {
		return strings.ToUpper(match)
	}

	// 匹配纯数字视频ID
	numPattern := regexp.MustCompile(`\d{8,}`)
	if match := numPattern.FindString(message); match != "" {
		return match
	}

	return ""
}

// =============================================================================
// 分析方法
// =============================================================================

// performVideoAnalysis 执行视频分析
func (xg *XiaovGraph) performVideoAnalysis(ctx context.Context, state GraphState) (string, error) {
	// 提取实际的视频数据，而不是整个 ToolResults 结构
	videoData := xg.extractVideoDataFromToolResults(state.ToolResults)
	videoDataJSON, _ := json.MarshalIndent(videoData, "", "  ")

	// 优化提示词，明确数据位置和格式
	prompt := fmt.Sprintf(`请基于以下视频数据进行分析和总结。

视频数据（JSON格式）：
%s

请从上述数据中提取以下字段并生成报告：
- view_count: 播放量
- like_count: 点赞数
- comment_count: 评论数
- title: 视频标题
- description: 视频描述
- author.username: 作者名称

严格按以下格式输出：
【摘要】基于标题和描述的一句话概括（30字内）
【数据】播放量%d,点赞%d,评论%d（必须使用上述JSON中的准确数字）
【情感】positive/negative/neutral（基于内容判断）
【要点】1.标题特点 2.内容主题 3.数据表现
【建议】1.优化建议 2.推广建议`, string(videoDataJSON), videoData["view_count"], videoData["like_count"], videoData["comment_count"])

	messages := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}

	// 设置超时时间为 5 分钟，比 Ollama 的 6 分钟超时稍短
	// 给模型足够时间处理，同时确保能捕获超时错误
	analysisCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	response, err := xg.llm.Generate(analysisCtx, messages)
	if err != nil {
		return "", fmt.Errorf("视频分析失败: %w", err)
	}

	return response.Content, nil
}

// extractVideoDataFromToolResults 从工具执行结果中提取视频数据
func (xg *XiaovGraph) extractVideoDataFromToolResults(toolResults []ToolExecutionResult) map[string]interface{} {
	result := map[string]interface{}{
		"view_count":    0,
		"like_count":    0,
		"comment_count": 0,
		"title":         "",
		"description":   "",
		"author":        "",
	}

	for _, tr := range toolResults {
		if tr.Result == nil {
			continue
		}

		// 工具返回的数据可能是嵌套结构，需要提取实际的 JSON 数据
		var dataMap map[string]interface{}

		// 先将 Result 转为字符串
		var jsonStr string
		switch v := tr.Result.(type) {
		case string:
			jsonStr = v
		case []byte:
			jsonStr = string(v)
		default:
			// 其他类型，尝试直接序列化
			if bytes, err := json.Marshal(tr.Result); err == nil {
				jsonStr = string(bytes)
			} else {
				log.Printf("⚠️ 序列化工具结果失败: %v", err)
				continue
			}
		}

		// 解析外层 JSON
		var outerMap map[string]interface{}
		if err := json.Unmarshal([]byte(jsonStr), &outerMap); err != nil {
			log.Printf("⚠️ 解析工具结果 JSON 失败: %v, 原始数据: %s", err, jsonStr[:min(len(jsonStr), 200)])
			continue
		}

		// 检查是否是 MCP 格式的响应（包含 content 字段）
		if contentArr, ok := outerMap["content"].([]interface{}); ok && len(contentArr) > 0 {
			if contentMap, ok := contentArr[0].(map[string]interface{}); ok {
				if text, ok := contentMap["text"].(string); ok {
					// text 字段包含 base64 编码的 JSON
					decodedBytes, err := base64.StdEncoding.DecodeString(text)
					if err != nil {
						log.Printf("⚠️ Base64 解码失败: %v", err)
						continue
					}
					// 解析解码后的 JSON
					if err := json.Unmarshal(decodedBytes, &dataMap); err != nil {
						log.Printf("⚠️ 解析解码后的 JSON 失败: %v", err)
						continue
					}
				}
			}
		} else {
			// 直接使用外层 map
			dataMap = outerMap
		}

		if dataMap == nil {
			continue
		}
		log.Printf("提取后的 dataMap: %+v", dataMap)

		// 检查是否有 code/data 结构
		if innerData, ok := dataMap["data"].(map[string]interface{}); ok {
			// 检查是否有 video 字段
			if video, ok := innerData["video"].(map[string]interface{}); ok {
				// 提取视频字段（JSON数字会被解析为float64）
				if v, ok := video["view_count"]; ok {
					result["view_count"] = toInt(v)
				}
				if v, ok := video["like_count"]; ok {
					result["like_count"] = toInt(v)
				}
				if v, ok := video["comment_count"]; ok {
					result["comment_count"] = toInt(v)
				}
				if v, ok := video["title"]; ok {
					result["title"] = toString(v)
				}
				if v, ok := video["description"]; ok {
					result["description"] = toString(v)
				}
				if author, ok := video["author"].(map[string]interface{}); ok {
					if v, ok := author["username"]; ok {
						result["author"] = toString(v)
					}
				}
				log.Printf("✅ 成功提取视频数据: view_count=%v, like_count=%v, comment_count=%v",
					result["view_count"], result["like_count"], result["comment_count"])
			} else {
				log.Printf("⚠️ 未找到 video 字段，data 内容: %+v", innerData)
			}
		} else {
			log.Printf("⚠️ 未找到 data 字段，尝试直接解析: %+v", dataMap)
			// 尝试直接解析（可能没有 code/data 包装）
			if video, ok := dataMap["video"].(map[string]interface{}); ok {
				if v, ok := video["view_count"]; ok {
					result["view_count"] = toInt(v)
				}
				if v, ok := video["like_count"]; ok {
					result["like_count"] = toInt(v)
				}
				if v, ok := video["comment_count"]; ok {
					result["comment_count"] = toInt(v)
				}
				if v, ok := video["title"]; ok {
					result["title"] = toString(v)
				}
				if v, ok := video["description"]; ok {
					result["description"] = toString(v)
				}
			}
		}
	}

	return result
}

// performContentCreationAnalysis 执行创作分析
func (xg *XiaovGraph) performContentCreationAnalysis(ctx context.Context, state GraphState) (string, error) {
	return xg.performVideoAnalysis(ctx, state)
}

// performWeeklyReportAnalysis 执行周报分析
func (xg *XiaovGraph) performWeeklyReportAnalysis(ctx context.Context, state GraphState) (string, error) {
	return xg.performVideoAnalysis(ctx, state)
}

// performTopicAnalysis 执行选题分析
func (xg *XiaovGraph) performTopicAnalysis(ctx context.Context, state GraphState) (string, error) {
	return xg.performVideoAnalysis(ctx, state)
}

// performGenericAnalysis 执行通用分析
func (xg *XiaovGraph) performGenericAnalysis(ctx context.Context, state GraphState) (string, error) {
	return xg.performVideoAnalysis(ctx, state)
}

// buildFallbackAnalysis 构建降级分析结果
func (xg *XiaovGraph) buildFallbackAnalysis(state GraphState) string {
	toolResultsJSON, _ := json.MarshalIndent(state.ToolResults, "", "  ")
	return fmt.Sprintf(`## 分析报告（简化版）

由于AI分析服务暂时繁忙，为您提供基于原始数据的简要分析：

### 原始数据
%s

### 说明
- 以上是从服务获取的原始数据
- 如需深度分析，请稍后重试`, string(toolResultsJSON))
}

// =============================================================================
// 通用对话方法
// =============================================================================

// generateGeneralChatResponse 生成通用对话回复
func (xg *XiaovGraph) generateGeneralChatResponse(ctx context.Context, state GraphState) (string, error) {
	memories, _ := xg.memoryManager.GetSessionHistory(ctx, state.SessionID, 10)

	var history string
	for _, mem := range memories {
		if mem.Type == memory.MemoryTypeUser {
			history += fmt.Sprintf("用户: %s\n", mem.Content)
		} else {
			history += fmt.Sprintf("助手: %s\n", mem.Content)
		}
	}

	prompt := fmt.Sprintf(`你是小V助手，一个专业的视频内容分析助手。

对话历史：
%s

用户: %s

请生成友好的回复：`, history, state.OriginalMessage)

	messages := []*schema.Message{
		{Role: schema.User, Content: prompt},
	}

	chatCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	response, err := xg.llm.Generate(chatCtx, messages)
	if err != nil {
		return "抱歉，我暂时无法处理您的请求，请稍后再试。", nil
	}

	return response.Content, nil
}

// buildFallbackResponse 构建降级响应
func (xg *XiaovGraph) buildFallbackResponse(state GraphState) string {
	return fmt.Sprintf("抱歉，我无法完成您的请求（意图：%s）。请稍后再试或联系客服。", state.Intent)
}

// =============================================================================
// 公共方法
// =============================================================================

// Execute 执行图编排
func (xg *XiaovGraph) Execute(ctx context.Context, input XiaovInput) (*XiaovOutput, error) {
	if input.SessionID == "" {
		input.SessionID = uuid.New().String()
	}

	log.Printf("🚀 [图编排] ========== 开始执行企业级图编排 ==========")
	log.Printf("🚀 [图编排] SessionID: %s | UserID: %s", input.SessionID, input.UserID)
	log.Printf("🚀 [图编排] 用户消息: %s", input.Message)
	log.Printf("🚀 [图编排] 图结构: START -> 意图识别 -> 工具选择 -> 工具执行 -> 分析 -> 总结 -> END")

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

// min 返回两个整数中的较小值
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// toInt 将任意类型转换为 int
func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int8:
		return int(val)
	case int16:
		return int(val)
	case int32:
		return int(val)
	case int64:
		return int(val)
	case uint:
		return int(val)
	case uint8:
		return int(val)
	case uint16:
		return int(val)
	case uint32:
		return int(val)
	case uint64:
		return int(val)
	case float32:
		return int(val)
	case float64:
		return int(val)
	case string:
		if i, err := fmt.Sscanf(val, "%d", new(int)); err == nil {
			return i
		}
		return 0
	default:
		return 0
	}
}

// toString 将任意类型转换为 string
func toString(v interface{}) string {
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", val)
	}
}
