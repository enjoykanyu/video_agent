package rag_answer

import (
	"context"
	"fmt"
	"log"
	"strings"

	states "video_agent/internal/agent/state"
	"video_agent/internal/agent/types"
	"video_agent/rag"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const RAGAnswerPrompt = `你是一个专业的知识库助手。请根据检索到的知识库内容，用自然、友好的语言回答用户的问题。

【回答规则】
1. 只使用提供的知识库内容回答，不要编造信息
2. 如果知识库内容与问题相关，请综合整理后给出清晰的回答
3. 如果知识库内容与问题不相关或为空，请礼貌地告知用户未找到相关信息
4. 回答要简洁明了，避免冗长
5. 可以适当补充过渡语，使回答更自然

【回答格式】
- 直接回答问题，不需要说"根据知识库..."
- 如果有多条相关信息，可以综合整理
- 结尾可以询问用户是否需要更多帮助`

type RAGAnswerAgent struct {
	llm model.ChatModel
}

func NewRAGAnswerAgent(llm model.ChatModel) *RAGAnswerAgent {
	return &RAGAnswerAgent{llm: llm}
}

func (a *RAGAnswerAgent) Execute(ctx context.Context, state *states.GraphState, ragResult *rag.RAGResult) (*types.AgentResult, error) {
	log.Printf("[RAGAnswer] executing for query: %s", state.OriginalQuery)

	// 构建消息
	messages := []*schema.Message{
		schema.SystemMessage(RAGAnswerPrompt),
	}

	// 添加检索上下文
	if ragResult != nil && ragResult.HasResult && ragResult.TopDocument != nil {
		contextMsg := fmt.Sprintf("【知识库内容】\n%s", ragResult.TopDocument.Content)
		messages = append(messages, schema.SystemMessage(contextMsg))
		log.Printf("[RAGAnswer] using RAG context, score=%.4f, level=%s",
			ragResult.TopDocument.Score, rag.GetSimilarityLevel(ragResult.TopDocument.Score))
	} else {
		messages = append(messages, schema.SystemMessage("【知识库内容】\n未找到相关信息。"))
		log.Printf("[RAGAnswer] no RAG context available")
	}

	// 添加用户问题
	messages = append(messages, schema.UserMessage(state.OriginalQuery))

	// 调用 LLM 生成回答
	resp, err := a.llm.Generate(ctx, messages)
	if err != nil {
		log.Printf("[RAGAnswer] LLM generation failed: %v", err)
		return &types.AgentResult{
			AgentType: types.AgentTypeRAG,
			Content:   "抱歉，生成回答时出现问题，请稍后重试。",
			Error:     err.Error(),
		}, err
	}

	log.Printf("[RAGAnswer] generated answer: %s", truncate(resp.Content, 100))

	return &types.AgentResult{
		AgentType: types.AgentTypeRAG,
		Content:   resp.Content,
	}, nil
}

func (a *RAGAnswerAgent) Route(ctx context.Context, state *states.GraphState, result *types.AgentResult) (types.AgentType, error) {
	return types.AgentTypeSummary, nil
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// GenerateDirectAnswer 直接生成回答（不使用 Agent 模式，简化流程）
func GenerateDirectAnswer(ctx context.Context, llm model.ChatModel, query string, ragResult *rag.RAGResult) string {
	messages := []*schema.Message{
		schema.SystemMessage(RAGAnswerPrompt),
	}

	if ragResult != nil && ragResult.HasResult && ragResult.TopDocument != nil {
		contextMsg := fmt.Sprintf("【知识库内容】\n%s", ragResult.TopDocument.Content)
		messages = append(messages, schema.SystemMessage(contextMsg))
	} else {
		messages = append(messages, schema.SystemMessage("【知识库内容】\n未找到相关信息。请礼貌地告知用户。"))
	}

	messages = append(messages, schema.UserMessage(query))

	resp, err := llm.Generate(ctx, messages)
	if err != nil {
		log.Printf("[RAGAnswer] generation failed: %v", err)
		if ragResult != nil && ragResult.HasResult {
			return fmt.Sprintf("根据知识库：%s", ragResult.TopDocument.Content)
		}
		return "抱歉，我在知识库中没有找到相关信息。您可以尝试换一种方式提问，或联系客服获取帮助。"
	}

	return resp.Content
}

// BuildRAGContext 构建 RAG 上下文（用于其他 Agent）
func BuildRAGContext(ragResult *rag.RAGResult) string {
	if ragResult == nil || !ragResult.HasResult {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("【知识库检索结果】\n")
	if ragResult.TopDocument != nil {
		sb.WriteString(fmt.Sprintf("相关度: %s\n", rag.GetSimilarityLevel(ragResult.TopDocument.Score)))
		sb.WriteString(fmt.Sprintf("内容: %s\n", ragResult.TopDocument.Content))
	}
	return sb.String()
}
