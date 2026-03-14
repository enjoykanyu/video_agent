package creative_analysis

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

// CreativeAnalysisAgentNode 创作分析Agent节点
type CreativeAnalysisAgentNode struct {
	*base.BaseAgent
}

// NewCreativeAnalysisAgentNode 创建创作分析Agent节点
func NewCreativeAnalysisAgentNode(llm model.ChatModel, te *base.ToolExecutor) *CreativeAnalysisAgentNode {
	return &CreativeAnalysisAgentNode{
		BaseAgent: base.NewBaseAgent(types.AgentTypeCreativeAnalysis, llm, te, prompt.CreativeAnalysisAgentPrompt),
	}
}

// Execute 执行创作分析
func (a *CreativeAnalysisAgentNode) Execute(ctx context.Context, state *state.GraphState) (*types.AgentResult, error) {
	log.Printf("[CreativeAnalysisAgent] executing for query: %s", state.OriginalQuery)

	// 提取领域信息
	field := a.extractField(state.OriginalQuery)
	log.Printf("[CreativeAnalysisAgent] detected field: %s", field)

	// 如果有分析结果，可以结合使用
	if analysisResult, ok := state.GetAgentResult(types.AgentTypeAnalysis); ok {
		log.Printf("[CreativeAnalysisAgent] using analysis data, length: %d", len(analysisResult.Content))
	}

	result, err := a.ExecuteWithToolLoop(ctx, state)
	if err != nil {
		return result, err
	}

	// 后处理：确保输出格式符合要求
	result = a.postProcess(result, field)
	return result, nil
}

// Route 路由到下一个Agent
func (a *CreativeAnalysisAgentNode) Route(ctx context.Context, state *state.GraphState, result *types.AgentResult) (types.AgentType, error) {
	log.Printf("[CreativeAnalysisAgent] routing to next agent")
	return a.DefaultRoute(ctx, state, result)
}

// extractField 从查询中提取领域信息
func (a *CreativeAnalysisAgentNode) extractField(query string) string {
	// 常见领域关键词映射
	fieldKeywords := map[string][]string{
		"科技":     {"科技", "数码", "手机", "电脑", "AI", "人工智能", "编程", "代码"},
		"美食":     {"美食", "烹饪", "菜谱", "餐厅", "吃", "料理"},
		"旅游":     {"旅游", "旅行", "景点", "攻略", "酒店", "机票"},
		"时尚":     {"时尚", "穿搭", "美妆", "护肤", "潮流"},
		"游戏":     {"游戏", "电竞", "手游", "网游", "Steam"},
		"教育":     {"教育", "学习", "考试", "课程", "知识"},
		"娱乐":     {"娱乐", "明星", "综艺", "电影", "电视剧", "八卦"},
		"体育":     {"体育", "足球", "篮球", "健身", "运动"},
		"财经":     {"财经", "股票", "基金", "理财", "经济", "投资"},
		"汽车":     {"汽车", "电动车", "新能源", "驾驶"},
		"宠物":     {"宠物", "猫", "狗", "萌宠"},
		"家居":     {"家居", "装修", "家具", "生活"},
		"职场":     {"职场", "工作", "面试", "简历", "升职加薪"},
		"情感":     {"情感", "恋爱", "婚姻", "家庭", "心理"},
		"健康":     {"健康", "养生", "医疗", "健身", "减肥"},
	}

	for field, keywords := range fieldKeywords {
		for _, keyword := range keywords {
			if strings.Contains(query, keyword) {
				return field
			}
		}
	}

	return "综合"
}

// postProcess 后处理结果
func (a *CreativeAnalysisAgentNode) postProcess(result *types.AgentResult, field string) *types.AgentResult {
	if result == nil {
		return result
	}

	// 如果内容为空，生成默认分析
	if strings.TrimSpace(result.Content) == "" {
		result.Content = fmt.Sprintf(`## %s领域创作选题分析

### 🔥 热门选题趋势

1. **选题一：待分析**
   - 热度指数：⭐⭐⭐⭐⭐
   - 受众群体：广泛
   - 创作建议：结合当前热点

2. **选题二：待分析**
   - 热度指数：⭐⭐⭐⭐
   - 受众群体：垂直领域
   - 创作建议：深度内容

### 📊 数据洞察

- 该领域近期整体热度：高
- 建议创作频率：每周2-3篇
- 最佳发布时间：晚上8-10点

### 💡 创作建议

1. 关注热点事件，快速响应
2. 注重内容质量，提供价值
3. 互动引导，提升参与度
`, field)
	}

	// 添加领域标签
	if !strings.Contains(result.Content, "领域") {
		result.Content = fmt.Sprintf("## %s领域创作分析\n\n%s", field, result.Content)
	}

	return result
}

// CreativeTopic 创作选题结构
type CreativeTopic struct {
	Title           string   `json:"title"`            // 选题标题
	HeatScore       int      `json:"heat_score"`       // 热度分数(1-100)
	Audience        string   `json:"audience"`         // 目标受众
	Difficulty      string   `json:"difficulty"`       // 创作难度
	EstimatedViews  string   `json:"estimated_views"`  // 预估播放量
	KeyPoints       []string `json:"key_points"`       // 核心要点
	ContentIdeas    []string `json:"content_ideas"`    // 内容创意
	Tags            []string `json:"tags"`             // 推荐标签
}

// CreativeAnalysisResult 创作分析结果
type CreativeAnalysisResult struct {
	Field           string          `json:"field"`            // 领域
	AnalysisTime    string          `json:"analysis_time"`    // 分析时间
	HotTopics       []CreativeTopic `json:"hot_topics"`       // 热门选题
	TrendInsight    string          `json:"trend_insight"`    // 趋势洞察
	Recommendations []string        `json:"recommendations"`  // 综合建议
}

// ParseCreativeResult 解析创作分析结果
func ParseCreativeResult(content string) (*CreativeAnalysisResult, error) {
	// 尝试从JSON解析
	var result CreativeAnalysisResult
	if err := json.Unmarshal([]byte(content), &result); err == nil {
		return &result, nil
	}

	// 如果不是JSON，返回结构化解析结果
	return &CreativeAnalysisResult{
		Field:        "未知领域",
		AnalysisTime: "当前",
		HotTopics:    parseTopicsFromText(content),
		TrendInsight: extractTrendInsight(content),
	}, nil
}

// parseTopicsFromText 从文本中解析选题
func parseTopicsFromText(content string) []CreativeTopic {
	// 简单的文本解析逻辑
	topics := []CreativeTopic{}

	// 按行分割，查找包含"选题"、"热点"等关键词的行
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "选题") || strings.Contains(line, "热点") {
			topic := CreativeTopic{
				Title:      extractTitle(line),
				HeatScore:  extractHeatScore(line),
				Difficulty: "中等",
			}
			topics = append(topics, topic)
		}
	}

	if len(topics) == 0 {
		// 返回默认选题
		topics = append(topics, CreativeTopic{
			Title:      "当前领域热点内容",
			HeatScore:  85,
			Difficulty: "中等",
			Audience:   "广泛受众",
		})
	}

	return topics
}

// extractTitle 提取标题
func extractTitle(line string) string {
	// 移除常见前缀
	prefixes := []string{"选题", "热点", "推荐", "【", "「", "1.", "2.", "3.", "-", "*"}
	title := line
	for _, prefix := range prefixes {
		title = strings.TrimPrefix(title, prefix)
		title = strings.TrimSpace(title)
	}
	return title
}

// extractHeatScore 提取热度分数
func extractHeatScore(line string) int {
	// 根据关键词估算热度
	if strings.Contains(line, "🔥") || strings.Contains(line, "热门") {
		return 90
	}
	if strings.Contains(line, "⭐⭐⭐⭐⭐") {
		return 95
	}
	if strings.Contains(line, "⭐⭐⭐⭐") {
		return 80
	}
	if strings.Contains(line, "⭐⭐⭐") {
		return 60
	}
	return 70
}

// extractTrendInsight 提取趋势洞察
func extractTrendInsight(content string) string {
	// 查找洞察部分
	if idx := strings.Index(content, "洞察"); idx != -1 {
		end := strings.Index(content[idx:], "\n\n")
		if end == -1 {
			end = len(content) - idx
		}
		return strings.TrimSpace(content[idx : idx+end])
	}
	return "该领域目前保持稳定增长趋势，建议持续创作优质内容。"
}