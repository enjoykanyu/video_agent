// Package mcp 提供企业级MCP管理功能 所有工具调用通过MCP Server
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"video_agent/mcp_client"

	"github.com/cloudwego/eino/components/tool"
)

// Manager MCP管理器 - 纯远程MCP模式
type Manager struct {
	// MCP客户端（必须）
	client mcp_client.Client

	// 缓存的工具列表
	tools       []tool.BaseTool
	toolsMu     sync.RWMutex
	toolsLoaded bool

	// 配置
	config *ManagerConfig
}

// ManagerConfig MCP管理器配置
type ManagerConfig struct {
	// 远程MCP Server配置（必填）
	RemoteConfig *mcp_client.Config
}

// NewManager 创建MCP管理器（纯远程模式）
func NewManager(config *ManagerConfig) (*Manager, error) {
	if config.RemoteConfig == nil {
		return nil, fmt.Errorf("远程MCP配置不能为空")
	}

	// 创建MCP客户端
	client, err := mcp_client.NewClient(config.RemoteConfig)
	if err != nil {
		return nil, fmt.Errorf("连接远程MCP Server失败: %w", err)
	}

	log.Printf("✅ [MCP Manager] 远程MCP连接成功 | Transport: %s", config.RemoteConfig.Transport)

	return &Manager{
		client: client,
		config: config,
	}, nil
}

// GetTools 从远程MCP Server获取所有可用工具
func (m *Manager) GetTools(ctx context.Context) ([]tool.BaseTool, error) {
	// 检查缓存
	m.toolsMu.RLock()
	if m.toolsLoaded {
		tools := m.tools
		m.toolsMu.RUnlock()
		return tools, nil
	}
	m.toolsMu.RUnlock()

	// 从远程MCP获取工具
	tools, err := m.client.GetTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("从远程MCP获取工具失败: %w", err)
	}

	m.toolsMu.Lock()
	m.tools = tools
	m.toolsLoaded = true
	m.toolsMu.Unlock()

	log.Printf("✅ [MCP Manager] 从远程MCP加载 %d 个工具", len(tools))
	return tools, nil
}

// GetInvokableTools 获取所有可调用的工具
func (m *Manager) GetInvokableTools(ctx context.Context) ([]tool.InvokableTool, error) {
	tools, err := m.GetTools(ctx)
	if err != nil {
		return nil, err
	}

	invokableTools := make([]tool.InvokableTool, 0, len(tools))
	for _, t := range tools {
		if invokable, ok := t.(tool.InvokableTool); ok {
			invokableTools = append(invokableTools, invokable)
		}
	}

	return invokableTools, nil
}

// GetTool 从远程MCP获取指定名称的工具
func (m *Manager) GetTool(ctx context.Context, name string) (tool.BaseTool, error) {
	tools, err := m.GetTools(ctx)
	if err != nil {
		return nil, err
	}

	for _, t := range tools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		if info.Name == name {
			return t, nil
		}
	}

	return nil, fmt.Errorf("工具未找到: %s", name)
}

// ExecuteTool 通过远程MCP执行工具调用
func (m *Manager) ExecuteTool(ctx context.Context, toolName string, params map[string]interface{}) (interface{}, error) {
	log.Printf("🔧 [MCP Manager] 执行远程工具: %s | Params: %v", toolName, params)

	t, err := m.client.GetTool(ctx, toolName)
	if err != nil {
		return nil, fmt.Errorf("获取工具失败: %w", err)
	}

	// 转换为InvokableTool执行
	invokable, ok := t.(tool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("工具不支持调用: %s", toolName)
	}

	paramsJSON, _ := json.Marshal(params)
	result, err := invokable.InvokableRun(ctx, string(paramsJSON))
	if err != nil {
		return nil, fmt.Errorf("远程工具执行失败: %w", err)
	}

	log.Printf("✅ [MCP Manager] 远程工具执行成功: %s", toolName)
	return result, nil
}

// RefreshTools 刷新工具列表（当MCP Server更新工具时调用）
func (m *Manager) RefreshTools(ctx context.Context) error {
	m.toolsMu.Lock()
	m.toolsLoaded = false
	m.tools = nil
	m.toolsMu.Unlock()

	_, err := m.GetTools(ctx)
	return err
}

// Close 关闭MCP管理器
func (m *Manager) Close() error {
	if m.client != nil {
		return m.client.Close()
	}
	return nil
}
