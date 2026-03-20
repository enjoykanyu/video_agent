package rag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/cloudwego/eino/components/embedding"
)

// OllamaEmbedder 自定义的 Ollama 嵌入器，确保返回 Float32 向量
type OllamaEmbedder struct {
	baseURL string
	model   string
	timeout time.Duration
}

// OllamaEmbedderConfig Ollama 嵌入器配置
type OllamaEmbedderConfig struct {
	BaseURL string
	Model   string
	Timeout time.Duration
}

// NewOllamaEmbedder 创建自定义 Ollama 嵌入器
func NewOllamaEmbedder(config *OllamaEmbedderConfig) (*OllamaEmbedder, error) {
	if config.BaseURL == "" {
		config.BaseURL = "http://localhost:11434"
	}
	if config.Model == "" {
		config.Model = "qwen3-embedding:0.6b"
	}
	if config.Timeout == 0 {
		config.Timeout = 10 * time.Second
	}

	return &OllamaEmbedder{
		baseURL: config.BaseURL,
		model:   config.Model,
		timeout: config.Timeout,
	}, nil
}

// EmbedStrings 实现 embedding.Embedder 接口
func (e *OllamaEmbedder) EmbedStrings(ctx context.Context, texts []string, opts ...embedding.Option) ([][]float64, error) {
	results := make([][]float64, len(texts))

	for i, text := range texts {
		embedding, err := e.getEmbedding(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("failed to embed text %d: %w", i, err)
		}
		results[i] = embedding
	}

	return results, nil
}

// ollamaEmbeddingRequest Ollama API 请求结构
type ollamaEmbeddingRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaEmbeddingResponse Ollama API 响应结构
type ollamaEmbeddingResponse struct {
	Embedding []float64 `json:"embedding"`
}

// getEmbedding 调用 Ollama API 获取嵌入向量
func (e *OllamaEmbedder) getEmbedding(ctx context.Context, text string) ([]float64, error) {
	reqBody := ollamaEmbeddingRequest{
		Model:  e.model,
		Prompt: text,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", e.baseURL+"/api/embeddings", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: e.timeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var result ollamaEmbeddingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Embedding, nil
}

// Ensure OllamaEmbedder 实现了 embedding.Embedder 接口
var _ embedding.Embedder = (*OllamaEmbedder)(nil)
