package tool

import (
	"context"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ListTodoTool struct{}

func (lt *ListTodoTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "list_todo",
		Desc: "List all todo items",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"finished": {
				Desc:     "filter todo items if finished",
				Type:     schema.Boolean,
				Required: false,
			},
		}),
	}, nil
}

func (lt *ListTodoTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// Mock调用逻辑
	return `{"todos": [{"id": "1", "content": "在2024年12月10日之前完成Eino项目演示文稿的准备工作", "started_at": 1717401600, "deadline": 1717488000, "done": false}]}`, nil
}

// AddTodoTool 添加todo项目的工具
type AddTodoTool struct{}

func (at *AddTodoTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "add_todo",
		Desc: "Add a new todo item",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"content": {
				Desc:     "the content of the todo item",
				Type:     schema.String,
				Required: true,
			},
			"deadline": {
				Desc:     "the deadline timestamp for the todo item (optional)",
				Type:     schema.Integer,
				Required: false,
			},
		}),
	}, nil
}

func (at *AddTodoTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// Mock调用逻辑 - 返回添加成功的响应
	return `{"success": true, "message": "Todo item added successfully", "todo": {"id": "2", "content": "学习 Eino", "deadline": 1733836800, "done": false}}`, nil
}

// SearchRepoTool 搜索仓库的工具
type SearchRepoTool struct{}

func (st *SearchRepoTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "search_repo",
		Desc: "Search for a repository by name",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"repo_name": {
				Desc:     "the name of the repository to search for",
				Type:     schema.String,
				Required: true,
			},
		}),
	}, nil
}

func (st *SearchRepoTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	// Mock调用逻辑 - 返回搜索结果
	return `{"repo": {"name": "cloudwego/eino", "url": "https://github.com/cloudwego/eino", "description": "Eino is a framework for building AI agents", "stars": 1234, "language": "Go"}}`, nil
}
