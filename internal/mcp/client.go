package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// MCPRequest MCP标准请求格式
type MCPRequest struct {
	JSONRPC string                 `json:"jsonrpc"` // "2.0"
	Method  string                 `json:"method"`  // 工具名称
	Params  map[string]interface{} `json:"params"`
	ID      string                 `json:"id"`
}

// MCPResponse MCP标准响应格式
type MCPResponse struct {
	JSONRPC string                 `json:"jsonrpc"`
	Result  map[string]interface{} `json:"result,omitempty"`
	Error   *MCPError              `json:"error,omitempty"`
	ID      string                 `json:"id"`
}

// MCPError MCP错误格式
type MCPError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// MCPClient MCP协议客户端
type MCPClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewMCPClient 创建MCP客户端
func NewMCPClient(baseURL string) *MCPClient {
	return &MCPClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// CallTool 调用MCP工具（真实HTTP调用）
func (c *MCPClient) CallTool(ctx context.Context, toolName string, params map[string]interface{}) (map[string]interface{}, error) {
	// 构建MCP标准请求
	reqBody := MCPRequest{
		JSONRPC: "2.0",
		Method:  toolName,
		Params:  params,
		ID:      generateRequestID(),
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	// 发送HTTP请求到MCP服务器
	url := c.baseURL + "/mcp/v1/call"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析MCP标准响应
	var mcpResp MCPResponse
	if err := json.Unmarshal(body, &mcpResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	// 检查错误
	if mcpResp.Error != nil {
		return nil, fmt.Errorf("MCP错误 [%d]: %s", mcpResp.Error.Code, mcpResp.Error.Message)
	}

	return mcpResp.Result, nil
}

// generateRequestID 生成请求ID
func generateRequestID() string {
	return fmt.Sprintf("req_%d", time.Now().UnixNano())
}
