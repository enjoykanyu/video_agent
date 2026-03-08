package biz

import (
	"context"
	"fmt"
	"time"

	mcpp "github.com/cloudwego/eino-ext/components/tool/mcp"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func CheckMCPServerHealthByPing(mcpServer MCPServer) (int32, error) {
	cli, err := client.NewSSEMCPClient(mcpServer.URL)
	if err != nil {
		return 0, fmt.Errorf("create MCP client failed: %w", err)
	}
	defer cli.Close()

	ctx := context.Background()

	if err := cli.Start(ctx); err != nil {
		return 0, fmt.Errorf("start MCP client failed: %w", err)
	}

	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "check-healthy-client",
		Version: "1.0.0",
	}

	_, err = cli.Initialize(ctx, initRequest)
	if err != nil {
		return 0, fmt.Errorf("initialize MCP client failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	retry := 3
	for retry > 0 {
		if err := cli.Ping(ctx); err != nil {
			retry--
			time.Sleep(1 * time.Second)
			continue
		}
		return 1, nil
	}

	return 0, fmt.Errorf("MCP server ping failed")
}

func GetHealthyMCPTools(ctx context.Context, mcpServers []MCPServer) ([]tool.BaseTool, []*schema.ToolInfo, error) {
	tools := []tool.BaseTool{}
	toolsInfo := []*schema.ToolInfo{}

	for _, mcpServer := range mcpServers {
		if mcpServer.Status == 0 {
			continue
		}

		cli, err := client.NewSSEMCPClient(mcpServer.URL)
		if err != nil {
			fmt.Printf("create MCP client failed: %v\n", err)
			continue
		}

		if err = cli.Start(ctx); err != nil {
			fmt.Printf("start MCP client failed: %v\n", err)
			cli.Close()
			continue
		}

		initRequest := mcp.InitializeRequest{}
		initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
		initRequest.Params.ClientInfo = mcp.Implementation{
			Name:    "video-assistant",
			Version: "1.0.0",
		}

		_, err = cli.Initialize(ctx, initRequest)
		if err != nil {
			fmt.Printf("initialize MCP client failed: %v\n", err)
			cli.Close()
			continue
		}

		mcpTools, err := mcpp.GetTools(ctx, &mcpp.Config{
			Cli: cli,
		})
		if err != nil {
			fmt.Printf("get MCP tools failed: %v\n", err)
			cli.Close()
			continue
		}

		for _, tool := range mcpTools {
			t, err := tool.Info(ctx)
			if err != nil {
				fmt.Printf("get tool info failed: %v\n", err)
				continue
			}
			tools = append(tools, tool)
			toolsInfo = append(toolsInfo, t)
		}
	}

	return tools, toolsInfo, nil
}
