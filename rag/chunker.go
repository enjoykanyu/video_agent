package rag

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
)

// ChunkStrategy 分块策略类型
type ChunkStrategy string

const (
	// ChunkStrategyFixed 固定长度分块
	ChunkStrategyFixed ChunkStrategy = "fixed"
	// ChunkStrategyRecursive 递归分块（按分隔符层级）
	ChunkStrategyRecursive ChunkStrategy = "recursive"
	// ChunkStrategySemantic 语义分块（基于Embedding相似度）
	ChunkStrategySemantic ChunkStrategy = "semantic"
	// ChunkStrategyHybrid 混合分块（递归+语义）
	ChunkStrategyHybrid ChunkStrategy = "hybrid"
)

// ChunkConfig 分块配置
type ChunkConfig struct {
	// Strategy 分块策略
	Strategy ChunkStrategy `json:"strategy"`

	// ChunkSize 目标块大小（字符数或token数）
	ChunkSize int `json:"chunk_size"`

	// ChunkOverlap 块间重叠大小
	ChunkOverlap int `json:"chunk_overlap"`

	// Separators 分隔符列表（按优先级），用于递归分块
	Separators []string `json:"separators"`

	// SemanticThreshold 语义相似度阈值（用于语义分块）
	SemanticThreshold float64 `json:"semantic_threshold"`

	// MinChunkSize 最小块大小
	MinChunkSize int `json:"min_chunk_size"`

	// MaxChunkSize 最大块大小
	MaxChunkSize int `json:"max_chunk_size"`
}

// DefaultChunkConfig 默认分块配置
func DefaultChunkConfig() *ChunkConfig {
	return &ChunkConfig{
		Strategy:          ChunkStrategyRecursive,
		ChunkSize:         512,
		ChunkOverlap:      50,
		Separators:        []string{"\n\n", "\n", "。", "；", " ", ""},
		SemanticThreshold: 0.85,
		MinChunkSize:      100,
		MaxChunkSize:      1024,
	}
}

// Chunk 文档分块结果
type Chunk struct {
	// ID 块ID
	ID string `json:"id"`

	// Content 块内容
	Content string `json:"content"`

	// Index 块索引（在原文档中的顺序）
	Index int `json:"index"`

	// StartPos 在原文档中的起始位置
	StartPos int `json:"start_pos"`

	// EndPos 在原文档中的结束位置
	EndPos int `json:"end_pos"`

	// Metadata 元数据
	Metadata map[string]interface{} `json:"metadata"`

	// Embedding 向量（语义分块时使用）
	Embedding []float64 `json:"embedding,omitempty"`
}

// Chunker 文档分块器接口
type Chunker interface {
	// Chunk 对文档进行分块
	Chunk(ctx context.Context, doc *schema.Document) ([]*Chunk, error)

	// ChunkBatch 批量分块
	ChunkBatch(ctx context.Context, docs []*schema.Document) ([]*Chunk, error)
}

// baseChunker 基础分块器
type baseChunker struct {
	config *ChunkConfig
}

// NewChunker 创建分块器
func NewChunker(config *ChunkConfig) Chunker {
	if config == nil {
		config = DefaultChunkConfig()
	}
	return &baseChunker{config: config}
}

// Chunk 对文档进行分块
func (c *baseChunker) Chunk(ctx context.Context, doc *schema.Document) ([]*Chunk, error) {
	switch c.config.Strategy {
	case ChunkStrategyFixed:
		return c.chunkFixed(doc)
	case ChunkStrategyRecursive:
		return c.chunkRecursive(doc)
	case ChunkStrategySemantic:
		return c.chunkSemantic(ctx, doc)
	case ChunkStrategyHybrid:
		return c.chunkHybrid(ctx, doc)
	default:
		return c.chunkRecursive(doc)
	}
}

// ChunkBatch 批量分块
func (c *baseChunker) ChunkBatch(ctx context.Context, docs []*schema.Document) ([]*Chunk, error) {
	var allChunks []*Chunk
	for _, doc := range docs {
		chunks, err := c.Chunk(ctx, doc)
		if err != nil {
			return nil, fmt.Errorf("chunk document %s failed: %w", doc.ID, err)
		}
		allChunks = append(allChunks, chunks...)
	}
	return allChunks, nil
}

// chunkFixed 固定长度分块
func (c *baseChunker) chunkFixed(doc *schema.Document) ([]*Chunk, error) {
	content := doc.Content
	if content == "" {
		return nil, nil
	}

	var chunks []*Chunk
	chunkSize := c.config.ChunkSize
	overlap := c.config.ChunkOverlap

	// 计算实际步长（考虑重叠）
	step := chunkSize - overlap
	if step <= 0 {
		step = chunkSize
	}

	for i := 0; i < len(content); i += step {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}

		chunk := &Chunk{
			ID:       fmt.Sprintf("%s_chunk_%d", doc.ID, len(chunks)),
			Content:  content[i:end],
			Index:    len(chunks),
			StartPos: i,
			EndPos:   end,
			Metadata: c.inheritMetadata(doc, len(chunks)),
		}
		chunks = append(chunks, chunk)

		// 如果已经到结尾，退出
		if end == len(content) {
			break
		}
	}

	return chunks, nil
}

// chunkRecursive 递归分块
func (c *baseChunker) chunkRecursive(doc *schema.Document) ([]*Chunk, error) {
	content := doc.Content
	if content == "" {
		return nil, nil
	}

	// 使用默认分隔符
	separators := c.config.Separators
	if len(separators) == 0 {
		separators = []string{"\n\n", "\n", "。", "；", " ", ""}
	}

	chunks := c.splitBySeparators(content, separators, doc.ID, 0)

	// 添加元数据
	for i, chunk := range chunks {
		chunk.Metadata = c.inheritMetadata(doc, i)
	}

	return chunks, nil
}

// splitBySeparators 递归按分隔符分割
func (c *baseChunker) splitBySeparators(text string, separators []string, docID string, startIndex int) []*Chunk {
	// 如果没有分隔符了，直接按固定长度分块
	if len(separators) == 0 {
		return c.splitFixed(text, docID, startIndex)
	}

	separator := separators[0]
	remainingSeparators := separators[1:]

	// 按当前分隔符分割
	parts := strings.Split(text, separator)
	var chunks []*Chunk
	currentIndex := startIndex
	currentPos := 0

	for _, part := range parts {
		if part == "" {
			currentPos += len(separator)
			continue
		}

		partLen := len(part)

		// 如果部分大小在合理范围内，作为一个块
		if partLen >= c.config.MinChunkSize && partLen <= c.config.MaxChunkSize {
			chunk := &Chunk{
				ID:       fmt.Sprintf("%s_chunk_%d", docID, currentIndex),
				Content:  part,
				Index:    currentIndex,
				StartPos: currentPos,
				EndPos:   currentPos + partLen,
			}
			chunks = append(chunks, chunk)
			currentIndex++
		} else if partLen > c.config.MaxChunkSize {
			// 如果部分太大，使用下一个分隔符递归分割
			subChunks := c.splitBySeparators(part, remainingSeparators, docID, currentIndex)
			// 更新位置信息
			for _, subChunk := range subChunks {
				subChunk.StartPos += currentPos
				subChunk.EndPos += currentPos
			}
			chunks = append(chunks, subChunks...)
			currentIndex += len(subChunks)
		}
		// 如果部分太小，跳过（会与下一部分合并）

		currentPos += partLen + len(separator)
	}

	// 合并过小的块
	chunks = c.mergeSmallChunks(chunks)

	return chunks
}

// splitFixed 固定长度分割（递归的最后手段）
func (c *baseChunker) splitFixed(text, docID string, startIndex int) []*Chunk {
	var chunks []*Chunk
	chunkSize := c.config.ChunkSize
	overlap := c.config.ChunkOverlap
	step := chunkSize - overlap
	if step <= 0 {
		step = chunkSize
	}

	for i := 0; i < len(text); i += step {
		end := i + chunkSize
		if end > len(text) {
			end = len(text)
		}

		chunk := &Chunk{
			ID:       fmt.Sprintf("%s_chunk_%d", docID, startIndex+len(chunks)),
			Content:  text[i:end],
			Index:    startIndex + len(chunks),
			StartPos: i,
			EndPos:   end,
		}
		chunks = append(chunks, chunk)

		if end == len(text) {
			break
		}
	}

	return chunks
}

// mergeSmallChunks 合并过小的块
func (c *baseChunker) mergeSmallChunks(chunks []*Chunk) []*Chunk {
	if len(chunks) <= 1 {
		return chunks
	}

	var merged []*Chunk
	var current *Chunk

	for _, chunk := range chunks {
		if current == nil {
			current = chunk
			continue
		}

		// 如果当前块太小，尝试合并
		if utf8.RuneCountInString(current.Content) < c.config.MinChunkSize {
			// 合并到下一个块
			current.Content += "\n" + chunk.Content
			current.EndPos = chunk.EndPos
			// 如果合并后还是太小，继续
			if utf8.RuneCountInString(current.Content) < c.config.MinChunkSize {
				continue
			}
		}

		merged = append(merged, current)
		current = chunk
	}

	// 处理最后一个块
	if current != nil {
		if len(merged) > 0 && utf8.RuneCountInString(current.Content) < c.config.MinChunkSize {
			// 与最后一个合并的块合并
			last := merged[len(merged)-1]
			last.Content += "\n" + current.Content
			last.EndPos = current.EndPos
		} else {
			merged = append(merged, current)
		}
	}

	return merged
}

// chunkSemantic 语义分块（基于Embedding相似度）
func (c *baseChunker) chunkSemantic(ctx context.Context, doc *schema.Document) ([]*Chunk, error) {
	// 先使用递归分块得到初始块
	initialChunks, err := c.chunkRecursive(doc)
	if err != nil {
		return nil, err
	}

	if len(initialChunks) <= 1 {
		return initialChunks, nil
	}

	// 获取 embedder
	embedder, err := NewOllamaEmbedder(&OllamaEmbedderConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3-embedding:0.6b",
		Timeout: 10,
	})
	if err != nil {
		// 如果 embedding 失败，返回递归分块结果
		return initialChunks, nil
	}

	// 计算每个块的 embedding
	contents := make([]string, len(initialChunks))
	for i, chunk := range initialChunks {
		contents[i] = chunk.Content
	}

	embeddings, err := embedder.EmbedStrings(ctx, contents)
	if err != nil {
		return initialChunks, nil
	}

	for i, chunk := range initialChunks {
		chunk.Embedding = embeddings[i]
	}

	// 基于语义相似度合并相邻块
	mergedChunks := c.mergeBySemanticSimilarity(initialChunks, c.config.SemanticThreshold)

	return mergedChunks, nil
}

// mergeBySemanticSimilarity 基于语义相似度合并块
func (c *baseChunker) mergeBySemanticSimilarity(chunks []*Chunk, threshold float64) []*Chunk {
	if len(chunks) <= 1 {
		return chunks
	}

	var merged []*Chunk
	current := chunks[0]

	for i := 1; i < len(chunks); i++ {
		next := chunks[i]

		// 计算相似度
		similarity := cosineSimilarity(current.Embedding, next.Embedding)

		// 如果相似度高且合并后不超过最大大小，则合并
		combinedLen := utf8.RuneCountInString(current.Content) + utf8.RuneCountInString(next.Content)
		if similarity >= threshold && combinedLen <= c.config.MaxChunkSize {
			current.Content += "\n" + next.Content
			current.EndPos = next.EndPos
			// 更新 embedding（简单平均）
			current.Embedding = averageEmbeddings(current.Embedding, next.Embedding)
		} else {
			merged = append(merged, current)
			current = next
		}
	}

	merged = append(merged, current)
	return merged
}

// chunkHybrid 混合分块（递归 + 语义）
func (c *baseChunker) chunkHybrid(ctx context.Context, doc *schema.Document) ([]*Chunk, error) {
	// 先递归分块
	chunks, err := c.chunkRecursive(doc)
	if err != nil {
		return nil, err
	}

	// 如果块数少，直接返回
	if len(chunks) <= 2 {
		return chunks, nil
	}

	// 使用语义相似度进一步优化
	embedder, err := NewOllamaEmbedder(&OllamaEmbedderConfig{
		BaseURL: "http://localhost:11434",
		Model:   "qwen3-embedding:0.6b",
		Timeout: 10,
	})
	if err != nil {
		return chunks, nil
	}

	// 计算 embeddings
	contents := make([]string, len(chunks))
	for i, chunk := range chunks {
		contents[i] = chunk.Content
	}

	embeddings, err := embedder.EmbedStrings(ctx, contents)
	if err != nil {
		return chunks, nil
	}

	for i, chunk := range chunks {
		chunk.Embedding = embeddings[i]
	}

	// 检测语义断点，重新划分
	return c.splitBySemanticBoundaries(chunks, c.config.SemanticThreshold)
}

// splitBySemanticBoundaries 基于语义边界重新划分
func (c *baseChunker) splitBySemanticBoundaries(chunks []*Chunk, threshold float64) ([]*Chunk, error) {
	if len(chunks) <= 1 {
		return chunks, nil
	}

	var result []*Chunk
	var currentGroup []*Chunk

	for i, chunk := range chunks {
		if i == 0 {
			currentGroup = []*Chunk{chunk}
			continue
		}

		// 计算与前一个块的相似度
		prev := chunks[i-1]
		similarity := cosineSimilarity(prev.Embedding, chunk.Embedding)

		// 如果相似度低，说明是语义边界，开始新组
		if similarity < threshold {
			// 合并当前组
			if len(currentGroup) > 0 {
				merged := c.mergeChunkGroup(currentGroup)
				result = append(result, merged)
			}
			currentGroup = []*Chunk{chunk}
		} else {
			currentGroup = append(currentGroup, chunk)
		}
	}

	// 处理最后一组
	if len(currentGroup) > 0 {
		merged := c.mergeChunkGroup(currentGroup)
		result = append(result, merged)
	}

	return result, nil
}

// mergeChunkGroup 合并块组
func (c *baseChunker) mergeChunkGroup(chunks []*Chunk) *Chunk {
	if len(chunks) == 1 {
		return chunks[0]
	}

	var content strings.Builder
	for i, chunk := range chunks {
		if i > 0 {
			content.WriteString("\n")
		}
		content.WriteString(chunk.Content)
	}

	first := chunks[0]
	last := chunks[len(chunks)-1]

	return &Chunk{
		ID:       first.ID,
		Content:  content.String(),
		Index:    first.Index,
		StartPos: first.StartPos,
		EndPos:   last.EndPos,
		Metadata: first.Metadata,
	}
}

// inheritMetadata 继承文档元数据
func (c *baseChunker) inheritMetadata(doc *schema.Document, chunkIndex int) map[string]interface{} {
	metadata := make(map[string]interface{})

	// 复制原文档元数据
	for k, v := range doc.MetaData {
		metadata[k] = v
	}

	// 添加分块相关信息
	metadata["chunk_index"] = chunkIndex
	metadata["source_doc_id"] = doc.ID
	metadata["chunk_strategy"] = string(c.config.Strategy)

	return metadata
}

// cosineSimilarity 计算余弦相似度
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := 0; i < len(a); i++ {
		dotProduct += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
}

// averageEmbeddings 平均两个 embedding
func averageEmbeddings(a, b []float64) []float64 {
	result := make([]float64, len(a))
	for i := 0; i < len(a); i++ {
		result[i] = (a[i] + b[i]) / 2
	}
	return result
}

// ChunksToDocuments 将 Chunk 转换为 Document（用于索引）
func ChunksToDocuments(chunks []*Chunk) []*schema.Document {
	docs := make([]*schema.Document, len(chunks))
	for i, chunk := range chunks {
		docs[i] = &schema.Document{
			ID:       chunk.ID,
			Content:  chunk.Content,
			MetaData: chunk.Metadata,
		}
	}
	return docs
}
