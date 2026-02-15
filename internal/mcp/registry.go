package mcp

import (
	"context"
	"fmt"
	"sync"
)

// Tool 工具接口
type Tool interface {
	Name() string
	Description() string
	Parameters() map[string]interface{}
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// Registry MCP工具注册中心
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// NewRegistry 创建新的注册中心
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register 注册工具
func (r *Registry) Register(tool Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool already registered: %s", name)
	}

	r.tools[name] = tool
	return nil
}

// Get 获取工具
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	return tool, nil
}

// List 列出所有工具
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

// Execute 执行工具
func (r *Registry) Execute(ctx context.Context, toolName string, params map[string]interface{}) (interface{}, error) {
	tool, err := r.Get(toolName)
	if err != nil {
		return nil, err
	}

	return tool.Execute(ctx, params)
}

// RegisterDefaultTools 注册默认工具集
func (r *Registry) RegisterDefaultTools() error {
	// 视频处理工具
	if err := r.Register(&VideoAnalysisTool{}); err != nil {
		return err
	}
	if err := r.Register(&FrameExtractionTool{}); err != nil {
		return err
	}
	if err := r.Register(&AudioTranscriptionTool{}); err != nil {
		return err
	}

	// 搜索工具
	if err := r.Register(&VectorSearchTool{}); err != nil {
		return err
	}
	if err := r.Register(&KeywordSearchTool{}); err != nil {
		return err
	}

	// 存储工具
	if err := r.Register(&MinIOStorageTool{}); err != nil {
		return err
	}
	if err := r.Register(&RedisCacheTool{}); err != nil {
		return err
	}

	// 数据工具
	if err := r.Register(&DataPipelineTool{}); err != nil {
		return err
	}
	if err := r.Register(&AnalyticsTool{}); err != nil {
		return err
	}

	return nil
}
