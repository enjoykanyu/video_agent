package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/google/uuid"
)

// MemoryType 记忆类型
type MemoryType string

const (
	MemoryTypeShortTerm  MemoryType = "short_term"
	MemoryTypeLongTerm   MemoryType = "long_term"
	MemoryTypeWorking    MemoryType = "working"
	MemoryTypeCompressed MemoryType = "compressed"
	MemoryTypeEpisodic   MemoryType = "episodic"
	MemoryTypeSemantic   MemoryType = "semantic"
)

// Memory 记忆结构
type Memory struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"session_id"`
	Type        MemoryType             `json:"type"`
	Content     string                 `json:"content"`
	Embedding   []float64              `json:"embedding,omitempty"`
	Metadata    map[string]interface{} `json:"metadata"`
	Importance  float64                `json:"importance"`
	CreatedAt   time.Time              `json:"created_at"`
	AccessedAt  time.Time              `json:"accessed_at"`
	AccessCount int                    `json:"access_count"`
	TTL         time.Duration          `json:"ttl,omitempty"`
}

// ShortTermMemory 短期记忆
type ShortTermMemory struct {
	store    map[string][]Memory
	maxItems int
	ttl      time.Duration
}

// NewShortTermMemory 创建短期记忆
func NewShortTermMemory(maxItems int, ttl time.Duration) *ShortTermMemory {
	return &ShortTermMemory{
		store:    make(map[string][]Memory),
		maxItems: maxItems,
		ttl:      ttl,
	}
}

// Get 获取短期记忆
func (m *ShortTermMemory) Get(ctx context.Context, sessionID string) []Memory {
	memories, exists := m.store[sessionID]
	if !exists {
		return nil
	}

	// 过滤过期记忆
	var validMemories []Memory
	now := time.Now()
	for _, mem := range memories {
		if now.Sub(mem.CreatedAt) < m.ttl {
			validMemories = append(validMemories, mem)
		}
	}

	// 更新存储
	m.store[sessionID] = validMemories

	return validMemories
}

// Set 设置短期记忆
func (m *ShortTermMemory) Set(ctx context.Context, memory Memory) error {
	memories := m.store[memory.SessionID]

	// 添加新记忆
	memories = append(memories, memory)

	// 限制数量
	if len(memories) > m.maxItems {
		memories = memories[len(memories)-m.maxItems:]
	}

	m.store[memory.SessionID] = memories
	return nil
}

// Clear 清除短期记忆
func (m *ShortTermMemory) Clear(sessionID string) {
	delete(m.store, sessionID)
}

// LongTermMemory 长期记忆
type LongTermMemory struct {
	vectorStore   VectorStore
	metadataStore MetadataStore
	embeddingFunc func(ctx context.Context, text string) ([]float64, error)
}

// VectorStore 向量存储接口
type VectorStore interface {
	Insert(ctx context.Context, id string, vector []float64, metadata map[string]interface{}) error
	Search(ctx context.Context, vector []float64, topK int) ([]SearchResult, error)
	Delete(ctx context.Context, id string) error
}

// MetadataStore 元数据存储接口
type MetadataStore interface {
	Save(ctx context.Context, memory Memory) error
	Get(ctx context.Context, id string) (*Memory, error)
	GetBySession(ctx context.Context, sessionID string) ([]Memory, error)
}

// SearchResult 搜索结果
type SearchResult struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// NewLongTermMemory 创建长期记忆
func NewLongTermMemory(
	vectorStore VectorStore,
	metadataStore MetadataStore,
	embeddingFunc func(ctx context.Context, text string) ([]float64, error),
) *LongTermMemory {
	return &LongTermMemory{
		vectorStore:   vectorStore,
		metadataStore: metadataStore,
		embeddingFunc: embeddingFunc,
	}
}

// Store 存储长期记忆
func (m *LongTermMemory) Store(ctx context.Context, memory Memory) error {
	// 生成嵌入向量
	if len(memory.Embedding) == 0 && m.embeddingFunc != nil {
		embedding, err := m.embeddingFunc(ctx, memory.Content)
		if err != nil {
			return fmt.Errorf("failed to generate embedding: %w", err)
		}
		memory.Embedding = embedding
	}

	// 存储到向量数据库
	if err := m.vectorStore.Insert(ctx, memory.ID, memory.Embedding, memory.Metadata); err != nil {
		return fmt.Errorf("failed to insert to vector store: %w", err)
	}

	// 存储元数据
	if err := m.metadataStore.Save(ctx, memory); err != nil {
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	return nil
}

// Search 搜索长期记忆
func (m *LongTermMemory) Search(ctx context.Context, query string, sessionID string, topK int) ([]Memory, error) {
	// 生成查询向量
	queryVector, err := m.embeddingFunc(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to generate query embedding: %w", err)
	}

	// 向量搜索
	results, err := m.vectorStore.Search(ctx, queryVector, topK*2)
	if err != nil {
		return nil, fmt.Errorf("failed to search vector store: %w", err)
	}

	// 获取完整记忆
	var memories []Memory
	for _, result := range results {
		memory, err := m.metadataStore.Get(ctx, result.ID)
		if err != nil {
			continue
		}

		// 过滤会话
		if sessionID != "" && memory.SessionID != sessionID {
			continue
		}

		memory.AccessedAt = time.Now()
		memory.AccessCount++
		memories = append(memories, *memory)

		if len(memories) >= topK {
			break
		}
	}

	return memories, nil
}

// WorkingMemory 工作记忆
type WorkingMemory struct {
	store   map[string]map[string]interface{}
	maxSize int
}

// NewWorkingMemory 创建工作记忆
func NewWorkingMemory(maxSize int) *WorkingMemory {
	return &WorkingMemory{
		store:   make(map[string]map[string]interface{}),
		maxSize: maxSize,
	}
}

// Get 获取工作记忆
func (m *WorkingMemory) Get(sessionID string, key string) (interface{}, bool) {
	session, exists := m.store[sessionID]
	if !exists {
		return nil, false
	}

	value, exists := session[key]
	return value, exists
}

// Set 设置工作记忆
func (m *WorkingMemory) Set(sessionID string, key string, value interface{}) {
	if _, exists := m.store[sessionID]; !exists {
		m.store[sessionID] = make(map[string]interface{})
	}

	m.store[sessionID][key] = value

	// 限制大小
	if len(m.store[sessionID]) > m.maxSize {
		// 删除最早的项
		for k := range m.store[sessionID] {
			delete(m.store[sessionID], k)
			break
		}
	}
}

// GetAll 获取所有工作记忆
func (m *WorkingMemory) GetAll(sessionID string) map[string]interface{} {
	session, exists := m.store[sessionID]
	if !exists {
		return make(map[string]interface{})
	}

	// 复制返回
	result := make(map[string]interface{})
	for k, v := range session {
		result[k] = v
	}

	return result
}

// Clear 清除工作记忆
func (m *WorkingMemory) Clear(sessionID string) {
	delete(m.store, sessionID)
}

// MemoryManager 记忆管理器
type MemoryManager struct {
	shortTerm  *ShortTermMemory
	longTerm   *LongTermMemory
	working    *WorkingMemory
	compressor *MemoryCompressor
}

// NewMemoryManager 创建记忆管理器
func NewMemoryManager(
	shortTerm *ShortTermMemory,
	longTerm *LongTermMemory,
	working *WorkingMemory,
) *MemoryManager {
	return &MemoryManager{
		shortTerm:  shortTerm,
		longTerm:   longTerm,
		working:    working,
		compressor: NewMemoryCompressor(),
	}
}

// Store 存储记忆
func (m *MemoryManager) Store(ctx context.Context, memory Memory) error {
	// 生成ID
	if memory.ID == "" {
		memory.ID = uuid.New().String()
	}

	if memory.CreatedAt.IsZero() {
		memory.CreatedAt = time.Now()
	}

	// 存储到工作记忆
	if memory.Type == MemoryTypeWorking {
		m.working.Set(memory.SessionID, memory.ID, memory.Content)
		return nil
	}

	// 存储到短期记忆
	if err := m.shortTerm.Set(ctx, memory); err != nil {
		return err
	}

	// 根据重要性存储到长期记忆
	if memory.Importance > 0.7 {
		if err := m.longTerm.Store(ctx, memory); err != nil {
			return err
		}
	}

	return nil
}

// Retrieve 检索记忆
func (m *MemoryManager) Retrieve(ctx context.Context, query string, sessionID string, topK int) ([]Memory, error) {
	var allMemories []Memory

	// 1. 从工作记忆检索
	workingData := m.working.GetAll(sessionID)
	for key, value := range workingData {
		content, _ := value.(string)
		allMemories = append(allMemories, Memory{
			ID:        key,
			SessionID: sessionID,
			Type:      MemoryTypeWorking,
			Content:   content,
		})
	}

	// 2. 从短期记忆检索
	shortTermMemories := m.shortTerm.Get(ctx, sessionID)
	allMemories = append(allMemories, shortTermMemories...)

	// 3. 从长期记忆检索
	longTermMemories, err := m.longTerm.Search(ctx, query, sessionID, topK)
	if err == nil {
		allMemories = append(allMemories, longTermMemories...)
	}

	// 按相关性和重要性排序
	sort.Slice(allMemories, func(i, j int) bool {
		scoreI := m.calculateRelevance(allMemories[i], query) * allMemories[i].Importance
		scoreJ := m.calculateRelevance(allMemories[j], query) * allMemories[j].Importance
		return scoreI > scoreJ
	})

	// 限制数量
	if len(allMemories) > topK {
		allMemories = allMemories[:topK]
	}

	return allMemories, nil
}

// calculateRelevance 计算相关性
func (m *MemoryManager) calculateRelevance(memory Memory, query string) float64 {
	// 简化实现：基于文本匹配
	// 实际项目中应该使用向量相似度
	if memory.Content == query {
		return 1.0
	}

	// 时间衰减
	timeDecay := math.Exp(-time.Since(memory.AccessedAt).Hours() / 24)

	return 0.5 * timeDecay
}

// Compress 压缩记忆
func (m *MemoryManager) Compress(ctx context.Context, sessionID string) error {
	memories := m.shortTerm.Get(ctx, sessionID)
	if len(memories) < 10 {
		return nil
	}

	// 压缩记忆
	summary := m.compressor.Compress(memories)

	// 存储压缩后的记忆
	compressedMemory := Memory{
		ID:         uuid.New().String(),
		SessionID:  sessionID,
		Type:       MemoryTypeCompressed,
		Content:    summary,
		Importance: 0.9,
		CreatedAt:  time.Now(),
	}

	return m.longTerm.Store(ctx, compressedMemory)
}

// ClearSession 清除会话记忆
func (m *MemoryManager) ClearSession(sessionID string) {
	m.shortTerm.Clear(sessionID)
	m.working.Clear(sessionID)
}

// MemoryCompressor 记忆压缩器
type MemoryCompressor struct{}

// NewMemoryCompressor 创建记忆压缩器
func NewMemoryCompressor() *MemoryCompressor {
	return &MemoryCompressor{}
}

// Compress 压缩记忆
func (c *MemoryCompressor) Compress(memories []Memory) string {
	// 简化实现：提取关键信息并总结
	// 实际项目中应该使用LLM进行智能压缩

	var contents []string
	for _, mem := range memories {
		if len(mem.Content) > 100 {
			contents = append(contents, mem.Content[:100]+"...")
		} else {
			contents = append(contents, mem.Content)
		}
	}

	// 生成摘要
	summary := fmt.Sprintf("会话包含 %d 条记忆，主要内容：", len(memories))
	for i, content := range contents {
		if i >= 3 {
			break
		}
		summary += fmt.Sprintf("\n- %s", content)
	}

	return summary
}

// ContextBuilder 上下文构建器
type ContextBuilder struct {
	maxTokens     int
	currentTokens int
	messages      []ContextMessage
}

// ContextMessage 上下文消息
type ContextMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Tokens  int    `json:"tokens"`
}

// NewContextBuilder 创建上下文构建器
func NewContextBuilder(maxTokens int) *ContextBuilder {
	return &ContextBuilder{
		maxTokens: maxTokens,
		messages:  make([]ContextMessage, 0),
	}
}

// AddMessage 添加消息
func (b *ContextBuilder) AddMessage(role, content string) bool {
	tokens := b.estimateTokens(content)

	if b.currentTokens+tokens > b.maxTokens {
		// 需要压缩或删除旧消息
		b.compress()
	}

	if b.currentTokens+tokens > b.maxTokens {
		return false
	}

	b.messages = append(b.messages, ContextMessage{
		Role:    role,
		Content: content,
		Tokens:  tokens,
	})
	b.currentTokens += tokens

	return true
}

// estimateTokens 估算token数
func (b *ContextBuilder) estimateTokens(text string) int {
	// 简化估算：中文字符按1.5个token计算
	return int(float64(len(text)) * 0.75)
}

// compress 压缩上下文
func (b *ContextBuilder) compress() {
	if len(b.messages) <= 2 {
		return
	}

	// 保留系统消息和最近的用户消息
	keepCount := 3
	if len(b.messages) > keepCount {
		// 压缩旧消息
		oldMessages := b.messages[:len(b.messages)-keepCount]
		summary := b.summarizeMessages(oldMessages)

		// 替换为摘要
		b.messages = append([]ContextMessage{{
			Role:    "system",
			Content: "历史对话摘要: " + summary,
			Tokens:  b.estimateTokens(summary),
		}}, b.messages[len(b.messages)-keepCount:]...)

		// 重新计算token数
		b.currentTokens = 0
		for _, msg := range b.messages {
			b.currentTokens += msg.Tokens
		}
	}
}

// summarizeMessages 总结消息
func (b *ContextBuilder) summarizeMessages(messages []ContextMessage) string {
	// 简化实现
	return fmt.Sprintf("之前对话包含 %d 条消息", len(messages))
}

// Build 构建上下文
func (b *ContextBuilder) Build() []ContextMessage {
	return b.messages
}

// ToJSON 转换为JSON
func (b *ContextBuilder) ToJSON() (string, error) {
	data, err := json.Marshal(b.messages)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
