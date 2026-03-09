package agent

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// MCPClientManager 管理所有MCP Server连接
type MCPClientManager struct {
	mu       sync.RWMutex
	clients  map[string]*MCPClientEntry // key: server UID
	tools    []tool.BaseTool
	toolMap  map[string]tool.BaseTool // key: tool name -> tool instance
	toolInfo []*schema.ToolInfo
}

type MCPClientEntry struct {
	Server    MCPServer
	Client    interface{}
	Tools     []tool.BaseTool
	ToolInfos []*schema.ToolInfo
	Connected bool
}

func NewMCPClientManager() *MCPClientManager {
	return &MCPClientManager{
		clients: make(map[string]*MCPClientEntry),
		toolMap: make(map[string]tool.BaseTool),
	}
}

// RefreshConnections 刷新所有MCP Server连接，获取最新的工具列表
func (m *MCPClientManager) RefreshConnections(ctx context.Context, servers []MCPServer) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// 关闭所有旧连接
	for _, entry := range m.clients {
		if entry.Client != nil {
			if closer, ok := entry.Client.(interface{ Close() }); ok {
				closer.Close()
			}
		}
	}

	m.clients = make(map[string]*MCPClientEntry)
	m.tools = nil
	m.toolMap = make(map[string]tool.BaseTool)
	m.toolInfo = nil

	for _, server := range servers {
		if server.Status == 0 {
			log.Printf("[MCP] skip disabled server: %s (%s)", server.Name, server.URL)
			continue
		}

		entry, err := m.connectServer(ctx, server)
		if err != nil {
			log.Printf("[MCP] connect server failed: %s (%s): %v", server.Name, server.URL, err)
			continue
		}

		m.clients[server.UID] = entry

		// 汇总所有工具
		for _, t := range entry.Tools {
			info, err := t.Info(ctx)
			if err != nil {
				continue
			}
			m.tools = append(m.tools, t)
			m.toolInfo = append(m.toolInfo, info)
			m.toolMap[info.Name] = t
		}

		log.Printf("[MCP] connected server: %s (%s), tools: %d",
			server.Name, server.URL, len(entry.Tools))
	}

	log.Printf("[MCP] total tools loaded: %d", len(m.tools))
	return nil
}

func (m *MCPClientManager) connectServer(ctx context.Context, server MCPServer) (*MCPClientEntry, error) {
	cli, err := client.NewSSEMCPClient(server.URL)
	if err != nil {
		return nil, fmt.Errorf("create SSE client: %w", err)
	}

	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	if err := cli.Start(connectCtx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("start client: %w", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "video-assistant-client",
		Version: "1.0.0",
	}

	_, err = cli.Initialize(connectCtx, initRequest)
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}

	// 验证连通性
	pingCtx, pingCancel := context.WithTimeout(ctx, 5*time.Second)
	defer pingCancel()
	if err := cli.Ping(pingCtx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	// 获取工具列表
	mcpTools, err := mcpp.GetTools(ctx, &mcpp.Config{Cli: cli})
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("get tools: %w", err)
	}

	var toolInfos []*schema.ToolInfo
	for _, t := range mcpTools {
		info, err := t.Info(ctx)
		if err != nil {
			continue
		}
		toolInfos = append(toolInfos, info)
	}

	return &MCPClientEntry{
		Server:    server,
		Client:    cli,
		Tools:     mcpTools,
		ToolInfos: toolInfos,
		Connected: true,
	}, nil
}

// GetTools 获取所有可用工具
func (m *MCPClientManager) GetTools() []tool.BaseTool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]tool.BaseTool, len(m.tools))
	copy(cp, m.tools)
	return cp
}

// GetToolInfos 获取所有工具信息（用于绑定到LLM）
func (m *MCPClientManager) GetToolInfos() []*schema.ToolInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cp := make([]*schema.ToolInfo, len(m.toolInfo))
	copy(cp, m.toolInfo)
	return cp
}

// GetToolByName 根据名称获取特定工具
func (m *MCPClientManager) GetToolByName(name string) (tool.BaseTool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.toolMap[name]
	return t, ok
}

// HealthCheck 健康检查
func (m *MCPClientManager) HealthCheck(ctx context.Context) map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]bool)
	for uid, entry := range m.clients {
		checkCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := entry.Client.(interface{ Ping(context.Context) error }).Ping(checkCtx)
		cancel()
		result[uid] = err == nil
	}
	return result
}

// Close 关闭所有连接
func (m *MCPClientManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range m.clients {
		if entry.Client != nil {
			if closer, ok := entry.Client.(interface{ Close() }); ok {
				closer.Close()
			}
		}
	}
	m.clients = make(map[string]*MCPClientEntry)
}
