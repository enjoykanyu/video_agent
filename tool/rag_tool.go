package tool

import (
	"context"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"video_agent/rag"
)

type RAGTool struct {
	ragManager *rag.RAGManager
	topK       int
}

func NewRAGTool(ragManager *rag.RAGManager, topK int) *RAGTool {
	return &RAGTool{
		ragManager: ragManager,
		topK:       topK,
	}
}

func (rt *RAGTool) SearchDocuments(ctx context.Context, query string) (string, error) {
	documents, err := rt.ragManager.SearchSimilarDocuments(query, rt.topK)
	if err != nil {
		return "", fmt.Errorf("failed to search documents: %w", err)
	}

	if len(documents) == 0 {
		return "未找到相关文档", nil
	}

	var results []string
	for i, doc := range documents {
		result := fmt.Sprintf("文档 %d:\n内容: %s\n", i+1, doc.Content)
		if len(doc.Metadata) > 0 {
			result += fmt.Sprintf("元数据: %v\n", doc.Metadata)
		}
		results = append(results, result)
	}

	return strings.Join(results, "\n---\n"), nil
}

func (rt *RAGTool) AddDocument(ctx context.Context, content string, metadata map[string]interface{}) error {
	return rt.ragManager.AddDocument(content, metadata)
}

// 创建RAG搜索工具节点
func CreateRAGSearchNode(ragManager *rag.RAGManager, topK int) compose.Lambda {
	ragTool := NewRAGTool(ragManager, topK)

	return *compose.InvokableLambda(func(ctx context.Context, input map[string]string) (output string, err error) {
		query, exists := input["query"]
		if !exists {
			return "", fmt.Errorf("missing query parameter")
		}

		return ragTool.SearchDocuments(ctx, query)
	})
}

// 创建增强的RAG节点，直接处理消息
func CreateEnhancedRAGNode(ragManager *rag.RAGManager, topK int) compose.Lambda {
	ragTool := NewRAGTool(ragManager, topK)

	return *compose.InvokableLambda(func(ctx context.Context, messages []*schema.Message) (output []*schema.Message, err error) {
		if len(messages) == 0 {
			return messages, nil
		}

		// 获取最后一条用户消息
		var userQuery string
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == schema.User {
				userQuery = messages[i].Content
				break
			}
		}

		if userQuery == "" {
			return messages, nil
		}

		// 搜索相关文档
		contextDocs, err := ragTool.SearchDocuments(ctx, userQuery)
		if err != nil {
			return messages, fmt.Errorf("failed to search documents: %w", err)
		}

		// 如果没有找到相关文档，返回原消息
		if contextDocs == "未找到相关文档" {
			return messages, nil
		}

		// 创建增强的系统消息
		enhancedSystemMsg := &schema.Message{
			Role:    schema.System,
			Content: fmt.Sprintf("基于以下检索到的文档内容回答问题：\n\n%s\n\n请根据这些文档信息提供准确、相关的回答。如果文档中没有相关信息，请明确说明。", contextDocs),
		}

		// 构建新的消息列表，在系统消息和用户消息之间插入上下文
		var enhancedMessages []*schema.Message

		// 添加原始系统消息（如果有）
		if len(messages) > 0 && messages[0].Role == schema.System {
			enhancedMessages = append(enhancedMessages, messages[0])
		}

		// 添加增强的系统消息
		enhancedMessages = append(enhancedMessages, enhancedSystemMsg)

		// 添加其余的消息
		startIdx := 0
		if len(messages) > 0 && messages[0].Role == schema.System {
			startIdx = 1
		}

		for i := startIdx; i < len(messages); i++ {
			enhancedMessages = append(enhancedMessages, messages[i])
		}

		return enhancedMessages, nil
	})
}
