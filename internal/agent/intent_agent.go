package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// IntentType 意图类型
type IntentType string

const (
	IntentVideoAnalysis      IntentType = "video_analysis"
	IntentRecommendation     IntentType = "recommendation"
	IntentKnowledgeQA        IntentType = "knowledge_qa"
	IntentWeeklyReport       IntentType = "weekly_report"
	IntentTopicAnalysis      IntentType = "topic_analysis"
	IntentKnowledgeBase      IntentType = "knowledge_base"
	IntentContentCreation    IntentType = "content_creation"
	IntentUserProfile        IntentType = "user_profile"
	IntentDanmakuAnalysis    IntentType = "danmaku_analysis"
	IntentTrendTracking      IntentType = "trend_tracking"
	IntentCompetitorAnalysis IntentType = "competitor_analysis"
	IntentGeneralChat        IntentType = "general_chat"
)

// Intent 意图结构
type Intent struct {
	Type       IntentType             `json:"type"`
	Confidence float64                `json:"confidence"`
	Entities   []Entity               `json:"entities"`
	RawQuery   string                 `json:"raw_query"`
	Metadata   map[string]interface{} `json:"metadata"`
}

// Entity 实体结构
type Entity struct {
	Type  string `json:"type"`
	Value string `json:"value"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// IntentRecognitionAgent 意图识别Agent
type IntentRecognitionAgent struct {
	llm        model.ChatModel
	confidence float64
}

// NewIntentRecognitionAgent 创建意图识别Agent
func NewIntentRecognitionAgent(llm model.ChatModel) *IntentRecognitionAgent {
	return &IntentRecognitionAgent{
		llm:        llm,
		confidence: 0.7,
	}
}

// Recognize 识别用户意图
func (a *IntentRecognitionAgent) Recognize(ctx context.Context, query string) (*Intent, error) {
	prompt := fmt.Sprintf(`你是一个视频平台AI助手的意图识别专家。

请分析以下用户查询，识别其意图类型和提取相关实体。

用户查询: "%s"

支持的意图类型:
1. video_analysis - 视频分析相关（如"分析这个视频"、"这个视频讲了什么"）
2. recommendation - 视频推荐相关（如"推荐类似视频"、"给我推荐一些"）
3. knowledge_qa - 知识问答相关（如"什么是XX"、"如何XX"）
4. weekly_report - 周报分析相关（如"查看我的周报"、"这周数据怎么样"）
5. topic_analysis - 选题分析相关（如"这个选题怎么样"、"帮我分析选题"）
6. knowledge_base - 知识库管理相关（如"上传文档"、"搜索知识库"）
7. content_creation - 内容创作相关（如"帮我写标题"、"生成文案"）
8. user_profile - 用户画像相关（如"我的兴趣是什么"、"分析我的偏好"）
9. danmaku_analysis - 弹幕分析相关（如"分析弹幕"、"观众反馈如何"）
10. trend_tracking - 热点追踪相关（如"最近有什么热点"、"趋势如何"）
11. competitor_analysis - 竞品分析相关（如"分析竞品"、"对比XX账号"）
12. general_chat - 通用对话（如"你好"、"谢谢"）

请以JSON格式返回，格式如下:
{
  "type": "意图类型",
  "confidence": 0.95,
  "entities": [
    {"type": "实体类型", "value": "实体值", "start": 0, "end": 5}
  ],
  "metadata": {}
}`, query)

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个专业的意图识别助手，请准确识别用户意图。",
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	// 为意图识别设置独立的超时（10秒），避免阻塞整个流程
	llmCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	response, err := a.llm.Generate(llmCtx, messages)
	if err != nil {
		// 回退到规则分类
		return a.ruleBasedClassify(query), nil
	}

	var intent Intent
	if err := json.Unmarshal([]byte(response.Content), &intent); err != nil {
		return a.ruleBasedClassify(query), nil
	}

	intent.RawQuery = query

	// 如果置信度太低，使用规则分类
	if intent.Confidence < a.confidence {
		ruleIntent := a.ruleBasedClassify(query)
		if ruleIntent.Confidence > intent.Confidence {
			return ruleIntent, nil
		}
	}

	return &intent, nil
}

// ruleBasedClassify 基于规则的意图分类
func (a *IntentRecognitionAgent) ruleBasedClassify(query string) *Intent {
	query = strings.ToLower(query)

	// 视频分析相关关键词
	videoAnalysisKeywords := []string{"分析视频", "视频分析", "这个视频", "视频内容", "视频讲了", "视频总结"}
	for _, kw := range videoAnalysisKeywords {
		if strings.Contains(query, kw) {
			return &Intent{
				Type:       IntentVideoAnalysis,
				Confidence: 0.8,
				RawQuery:   query,
				Entities:   a.extractEntities(query),
			}
		}
	}

	// 推荐相关关键词
	recommendKeywords := []string{"推荐", "类似", "相似", "想看", "找一找"}
	for _, kw := range recommendKeywords {
		if strings.Contains(query, kw) {
			return &Intent{
				Type:       IntentRecommendation,
				Confidence: 0.8,
				RawQuery:   query,
				Entities:   a.extractEntities(query),
			}
		}
	}

	// 周报相关关键词
	weeklyKeywords := []string{"周报", "这周", "本周数据", "数据分析", "我的数据"}
	for _, kw := range weeklyKeywords {
		if strings.Contains(query, kw) {
			return &Intent{
				Type:       IntentWeeklyReport,
				Confidence: 0.8,
				RawQuery:   query,
				Entities:   a.extractEntities(query),
			}
		}
	}

	// 选题分析关键词
	topicKeywords := []string{"选题", "主题", "这个题材", "内容方向"}
	for _, kw := range topicKeywords {
		if strings.Contains(query, kw) {
			return &Intent{
				Type:       IntentTopicAnalysis,
				Confidence: 0.8,
				RawQuery:   query,
				Entities:   a.extractEntities(query),
			}
		}
	}

	// 知识问答关键词
	knowledgeKeywords := []string{"什么是", "怎么", "如何", "为什么", "介绍一下"}
	for _, kw := range knowledgeKeywords {
		if strings.Contains(query, kw) {
			return &Intent{
				Type:       IntentKnowledgeQA,
				Confidence: 0.7,
				RawQuery:   query,
				Entities:   a.extractEntities(query),
			}
		}
	}

	// 默认通用对话
	return &Intent{
		Type:       IntentGeneralChat,
		Confidence: 0.5,
		RawQuery:   query,
		Entities:   a.extractEntities(query),
	}
}

// extractEntities 提取实体
func (a *IntentRecognitionAgent) extractEntities(query string) []Entity {
	var entities []Entity

	// 提取视频ID (BV号)
	if idx := strings.Index(query, "BV"); idx != -1 && idx+10 <= len(query) {
		entities = append(entities, Entity{
			Type:  "video_id",
			Value: query[idx : idx+12],
			Start: idx,
			End:   idx + 12,
		})
	}

	// 提取URL
	if strings.Contains(query, "http") {
		start := strings.Index(query, "http")
		end := start
		for end < len(query) && query[end] != ' ' {
			end++
		}
		entities = append(entities, Entity{
			Type:  "url",
			Value: query[start:end],
			Start: start,
			End:   end,
		})
	}

	return entities
}
