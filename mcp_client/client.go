// Package mcp_client 提供MCP客户端实现
// 参考: https://github.com/cloudwego/eino-ext/tree/main/components/tool/mcp
package mcp_client

import (
	"context"
	"fmt"
	"log"
	"net/http"

	eino_mcp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
) // Client MCP客户端接口
type Client interface {
	// GetTools 获取所有可用的MCP工具
	GetTools(ctx context.Context) ([]tool.BaseTool, error)
	// GetTool 获取指定名称的工具
	GetTool(ctx context.Context, name string) (tool.BaseTool, error)
	// Close 关闭客户端连接
	Close() error
}

// Config MCP客户端配置
type Config struct {
	// 传输方式: "stdio" 或 "sse"
	Transport string
	// Server配置
	Server ServerConfig
}

// ServerConfig MCP Server配置
type ServerConfig struct {
	// 命令路径（stdio模式使用）
	Command string
	// 参数（stdio模式使用）
	Args []string
	// 环境变量（stdio模式使用）
	Env []string
	// SSE URL（sse模式使用）
	URL string
	// 自定义HTTP头
	Headers map[string]string
}

// NewClient 创建MCP客户端
func NewClient(conf *Config) (Client, error) {
	switch conf.Transport {
	case "stdio":
		return NewStdioClient(&conf.Server)
	case "sse":
		return NewSSEClient(&conf.Server)
	default:
		return nil, fmt.Errorf("不支持的传输方式: %s", conf.Transport)
	}
}

// StdioClient Stdio MCP客户端
type StdioClient struct {
	cli   client.MCPClient
	tools []tool.BaseTool
	conf  *ServerConfig
}

// NewStdioClient 创建Stdio MCP客户端
// 通过启动子进程运行MCP Server
func NewStdioClient(conf *ServerConfig) (*StdioClient, error) {
	log.Printf("🔌 [MCP Client] 启动Stdio模式 | Command: %s %v", conf.Command, conf.Args)

	// 创建stdio客户端
	cli, err := client.NewStdioMCPClient(conf.Command, conf.Env, conf.Args...)
	if err != nil {
		return nil, fmt.Errorf("创建Stdio MCP客户端失败: %w", err)
	}

	// 初始化
	ctx := context.Background()
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "xiaov-agent",
		Version: "1.0.0",
	}

	_, err = cli.Initialize(ctx, initReq)
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("MCP初始化失败: %w", err)
	}

	log.Printf("✅ [MCP Client] Stdio连接成功")

	return &StdioClient{
		cli:  cli,
		conf: conf,
	}, nil
}

// GetTools 获取所有工具
func (c *StdioClient) GetTools(ctx context.Context) ([]tool.BaseTool, error) {
	if c.tools != nil {
		return c.tools, nil
	}

	headers := http.Header{}
	if c.conf.Headers != nil {
		for k, v := range c.conf.Headers {
			headers.Set(k, v)
		}
	}

	tools, err := eino_mcp.GetTools(ctx, &eino_mcp.Config{
		Cli:           c.cli,
		CustomHeaders: c.conf.Headers,
	})
	if err != nil {
		return nil, fmt.Errorf("获取工具列表失败: %w", err)
	}

	c.tools = tools
	log.Printf("✅ [MCP Client] Stdio模式加载 %d 个工具", len(c.tools))
	return c.tools, nil
}

// GetTool 获取指定工具
func (c *StdioClient) GetTool(ctx context.Context, name string) (tool.BaseTool, error) {
	tools, err := c.GetTools(ctx)
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

// Close 关闭客户端
func (c *StdioClient) Close() error {
	return c.cli.Close()
}

// SSEClient SSE MCP客户端
type SSEClient struct {
	cli   client.MCPClient
	tools []tool.BaseTool
	conf  *ServerConfig
}

// NewSSEClient 创建SSE MCP客户端
// 连接到远程SSE MCP Server
func NewSSEClient(conf *ServerConfig) (*SSEClient, error) {
	log.Printf("🔌 [MCP Client] 启动SSE模式 | URL: %s", conf.URL)

	// 创建SSE客户端
	cli, err := client.NewSSEMCPClient(conf.URL)
	if err != nil {
		return nil, fmt.Errorf("创建SSE MCP客户端失败: %w", err)
	}

	// 启动SSE连接
	ctx := context.Background()
	if err := cli.Start(ctx); err != nil {
		cli.Close()
		return nil, fmt.Errorf("启动SSE连接失败: %w", err)
	}

	// 初始化
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "xiaov-agent",
		Version: "1.0.0",
	}

	_, err = cli.Initialize(ctx, initReq)
	if err != nil {
		cli.Close()
		return nil, fmt.Errorf("MCP初始化失败: %w", err)
	}

	log.Printf("✅ [MCP Client] SSE连接成功")

	return &SSEClient{
		cli:  cli,
		conf: conf,
	}, nil
}

// GetTools 获取所有工具
func (c *SSEClient) GetTools(ctx context.Context) ([]tool.BaseTool, error) {
	if c.tools != nil {
		return c.tools, nil
	}

	tools, err := eino_mcp.GetTools(ctx, &eino_mcp.Config{
		Cli:           c.cli,
		CustomHeaders: c.conf.Headers,
	})
	if err != nil {
		return nil, fmt.Errorf("获取工具列表失败: %w", err)
	}

	c.tools = tools
	log.Printf("✅ [MCP Client] SSE模式加载 %d 个工具", len(c.tools))
	return c.tools, nil
}

// GetTool 获取指定工具
func (c *SSEClient) GetTool(ctx context.Context, name string) (tool.BaseTool, error) {
	tools, err := c.GetTools(ctx)
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

// Close 关闭客户端
func (c *SSEClient) Close() error {
	return c.cli.Close()
}
