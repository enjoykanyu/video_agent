package rag_selector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	base "video_agent/internal/agent/agents/base"
	prompt "video_agent/internal/agent/prompt"
	"video_agent/internal/agent/state"
	"video_agent/internal/agent/types"

	"github.com/cloudwego/eino/components/model"
)

// RAGSelectorAgentNode RAG知识库选择Agent节点
type RAGSelectorAgentNode struct {
	*base.BaseAgent
	knowledgeBases []KnowledgeBase // 可用的知识库列表
}

// KnowledgeBase 知识库定义
type KnowledgeBase struct {
	ID          string   `json:"id"`          // 知识库ID
	Name        string   `json:"name"`        // 知识库名称
	Description string   `json:"description"` // 知识库描述
	Tags        []string `json:"tags"`        // 知识库标签
	Priority    int      `json:"priority"`    // 优先级（数值越大优先级越高）
}

// RAGSelectionResult RAG选择结果
type RAGSelectionResult struct {
	SelectedKBs []KnowledgeBase `json:"selected_kbs"` // 选中的知识库
	Reason      string          `json:"reason"`       // 选择理由
	Query       string          `json:"query"`        // 优化后的查询
	Confidence  float64         `json:"confidence"`   // 选择置信度
}

// NewRAGSelectorAgentNode 创建RAG知识库选择Agent节点
func NewRAGSelectorAgentNode(llm model.ChatModel, te *base.ToolExecutor, knowledgeBases []KnowledgeBase) *RAGSelectorAgentNode {
	// 如果没有传入知识库，使用默认知识库
	if len(knowledgeBases) == 0 {
		knowledgeBases = getDefaultKnowledgeBases()
	}

	return &RAGSelectorAgentNode{
		BaseAgent:      base.NewBaseAgent(types.AgentTypeRAGSelector, llm, te, prompt.RAGSelectorAgentPrompt),
		knowledgeBases: knowledgeBases,
	}
}

// Execute 执行RAG知识库选择
func (a *RAGSelectorAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[RAGSelectorAgent] executing for query: %s", state.OriginalQuery)

	// 1. 分析用户查询意图
	intent := a.analyzeIntent(state.OriginalQuery)
	log.Printf("[RAGSelectorAgent] detected intent: %s", intent)

	// 2. 根据意图选择知识库
	selection := a.selectKnowledgeBases(state.OriginalQuery, intent)
	log.Printf("[RAGSelectorAgent] selected %d knowledge bases", len(selection.SelectedKBs))

	// 3. 优化查询语句
	optimizedQuery := a.optimizeQuery(state.OriginalQuery, intent)

	// 4. 构建结果
	result := &RAGSelectionResult{
		SelectedKBs: selection.SelectedKBs,
		Reason:      selection.Reason,
		Query:       optimizedQuery,
		Confidence:  selection.Confidence,
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")

	return &types.AgentResult{
		AgentType: types.AgentTypeRAGSelector,
		Content:   string(resultJSON),
		ToolsUsed: []string{"rag_selector"},
	}, nil
}

// Route 路由到下一个Agent
func (a *RAGSelectorAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	log.Printf("[RAGSelectorAgent] routing to next agent")

	// RAG选择后，通常进入RAG检索节点
	return types.AgentTypeRAG, nil
}

// IntentType 意图类型
type IntentType string

const (
	IntentProduct   IntentType = "product"   // 产品相关
	IntentTechnical IntentType = "technical" // 技术相关
	IntentBusiness  IntentType = "business"  // 业务相关
	IntentGeneral   IntentType = "general"   // 通用查询
	IntentFAQ       IntentType = "faq"       // 常见问题
)

// analyzeIntent 分析用户查询意图
func (a *RAGSelectorAgentNode) analyzeIntent(query string) IntentType {
	query = strings.ToLower(query)

	// 产品相关关键词
	productKeywords := []string{"产品", "功能", "特性", "怎么用", "使用", "操作", "界面", "设置"}
	for _, kw := range productKeywords {
		if strings.Contains(query, kw) {
			return IntentProduct
		}
	}

	// 技术相关关键词
	techKeywords := []string{"技术", "架构", "代码", "api", "接口", "开发", "实现", "原理", "算法"}
	for _, kw := range techKeywords {
		if strings.Contains(query, kw) {
			return IntentTechnical
		}
	}

	// 业务相关关键词
	businessKeywords := []string{"业务", "流程", "规则", "政策", "价格", "费用", "合同", "协议"}
	for _, kw := range businessKeywords {
		if strings.Contains(query, kw) {
			return IntentBusiness
		}
	}

	// FAQ相关关键词
	faqKeywords := []string{"怎么", "如何", "为什么", "是什么", "怎么办", "问题", "故障", "错误"}
	for _, kw := range faqKeywords {
		if strings.Contains(query, kw) {
			return IntentFAQ
		}
	}

	return IntentGeneral
}

// SelectionResult 选择结果
type SelectionResult struct {
	SelectedKBs []KnowledgeBase
	Reason      string
	Confidence  float64
}

// selectKnowledgeBases 根据意图选择知识库
func (a *RAGSelectorAgentNode) selectKnowledgeBases(query string, intent IntentType) SelectionResult {
	var selectedKBs []KnowledgeBase
	var reason string
	var confidence float64

	switch intent {
	case IntentProduct:
		// 产品意图：选择产品文档和用户手册
		for _, kb := range a.knowledgeBases {
			if containsTag(kb.Tags, "product") || containsTag(kb.Tags, "user_guide") {
				selectedKBs = append(selectedKBs, kb)
			}
		}
		reason = "用户查询涉及产品功能和使用，选择产品文档和用户手册知识库"
		confidence = 0.9

	case IntentTechnical:
		// 技术意图：选择技术文档和API文档
		for _, kb := range a.knowledgeBases {
			if containsTag(kb.Tags, "technical") || containsTag(kb.Tags, "api") || containsTag(kb.Tags, "development") {
				selectedKBs = append(selectedKBs, kb)
			}
		}
		reason = "用户查询涉及技术实现和开发，选择技术文档和API文档知识库"
		confidence = 0.9

	case IntentBusiness:
		// 业务意图：选择业务文档和政策文档
		for _, kb := range a.knowledgeBases {
			if containsTag(kb.Tags, "business") || containsTag(kb.Tags, "policy") {
				selectedKBs = append(selectedKBs, kb)
			}
		}
		reason = "用户查询涉及业务流程和政策，选择业务文档知识库"
		confidence = 0.85

	case IntentFAQ:
		// FAQ意图：选择FAQ知识库和通用文档
		for _, kb := range a.knowledgeBases {
			if containsTag(kb.Tags, "faq") || containsTag(kb.Tags, "general") {
				selectedKBs = append(selectedKBs, kb)
			}
		}
		reason = "用户查询常见问题，选择FAQ知识库"
		confidence = 0.8

	default:
		// 通用意图：选择所有通用知识库，按优先级排序
		for _, kb := range a.knowledgeBases {
			if containsTag(kb.Tags, "general") || kb.Priority >= 5 {
				selectedKBs = append(selectedKBs, kb)
			}
		}
		reason = "通用查询，选择高优先级和通用知识库"
		confidence = 0.7
	}

	// 如果没有匹配到，选择优先级最高的3个
	if len(selectedKBs) == 0 {
		selectedKBs = a.getTopPriorityKBs(3)
		reason = "未匹配到特定知识库，选择优先级最高的知识库"
		confidence = 0.6
	}

	// 按优先级排序
	selectedKBs = sortByPriority(selectedKBs)

	return SelectionResult{
		SelectedKBs: selectedKBs,
		Reason:      reason,
		Confidence:  confidence,
	}
}

// optimizeQuery 优化查询语句
func (a *RAGSelectorAgentNode) optimizeQuery(query string, intent IntentType) string {
	// 根据意图添加优化关键词
	switch intent {
	case IntentProduct:
		if !strings.Contains(query, "功能") && !strings.Contains(query, "使用") {
			query = query + " 功能使用说明"
		}
	case IntentTechnical:
		if !strings.Contains(query, "技术") && !strings.Contains(query, "实现") {
			query = query + " 技术实现"
		}
	case IntentBusiness:
		if !strings.Contains(query, "流程") && !strings.Contains(query, "规则") {
			query = query + " 业务流程"
		}
	}

	return query
}

// getTopPriorityKBs 获取优先级最高的N个知识库
func (a *RAGSelectorAgentNode) getTopPriorityKBs(n int) []KnowledgeBase {
	sorted := make([]KnowledgeBase, len(a.knowledgeBases))
	copy(sorted, a.knowledgeBases)

	// 按优先级排序（降序）
	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Priority < sorted[j].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	if n > len(sorted) {
		n = len(sorted)
	}
	return sorted[:n]
}

// containsTag 检查标签是否包含
func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// sortByPriority 按优先级排序
func sortByPriority(kbs []KnowledgeBase) []KnowledgeBase {
	result := make([]KnowledgeBase, len(kbs))
	copy(result, kbs)

	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[i].Priority < result[j].Priority {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// getDefaultKnowledgeBases 获取默认知识库列表
func getDefaultKnowledgeBases() []KnowledgeBase {
	return []KnowledgeBase{
		{
			ID:          "product_docs",
			Name:        "产品文档",
			Description: "产品功能介绍、使用说明、操作指南",
			Tags:        []string{"product", "user_guide", "general"},
			Priority:    10,
		},
		{
			ID:          "technical_docs",
			Name:        "技术文档",
			Description: "技术架构、开发文档、API接口说明",
			Tags:        []string{"technical", "api", "development"},
			Priority:    9,
		},
		{
			ID:          "faq_kb",
			Name:        "FAQ知识库",
			Description: "常见问题解答、故障排查、使用技巧",
			Tags:        []string{"faq", "general", "troubleshooting"},
			Priority:    8,
		},
		{
			ID:          "business_docs",
			Name:        "业务文档",
			Description: "业务流程、政策规则、合作协议",
			Tags:        []string{"business", "policy"},
			Priority:    7,
		},
		{
			ID:          "video_knowledge",
			Name:        "视频知识库",
			Description: "视频创作、运营技巧、平台规则",
			Tags:        []string{"video", "creation", "operation"},
			Priority:    6,
		},
	}
}

// AddKnowledgeBase 动态添加知识库
func (a *RAGSelectorAgentNode) AddKnowledgeBase(kb KnowledgeBase) {
	a.knowledgeBases = append(a.knowledgeBases, kb)
	log.Printf("[RAGSelectorAgent] added knowledge base: %s", kb.Name)
}

// RemoveKnowledgeBase 移除知识库
func (a *RAGSelectorAgentNode) RemoveKnowledgeBase(kbID string) {
	var newKBs []KnowledgeBase
	for _, kb := range a.knowledgeBases {
		if kb.ID != kbID {
			newKBs = append(newKBs, kb)
		}
	}
	a.knowledgeBases = newKBs
	log.Printf("[RAGSelectorAgent] removed knowledge base: %s", kbID)
}

// GetKnowledgeBases 获取所有知识库
func (a *RAGSelectorAgentNode) GetKnowledgeBases() []KnowledgeBase {
	return a.knowledgeBases
}

// ParseSelectionResult 解析选择结果
func ParseSelectionResult(content string) (*RAGSelectionResult, error) {
	var result RAGSelectionResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("parse selection result failed: %w", err)
	}
	return &result, nil
}