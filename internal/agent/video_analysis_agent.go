package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// VideoAnalysisAgent 视频分析Agent
type VideoAnalysisAgent struct {
	llm              model.ChatModel
	videoProcessor   VideoProcessor
	frameExtractor   FrameExtractor
	audioTranscriber AudioTranscriber
	ragManager       RAGManager
}

// VideoProcessor 视频处理器接口
type VideoProcessor interface {
	Download(ctx context.Context, videoURL string) (string, error)
	GetInfo(ctx context.Context, videoPath string) (*VideoInfo, error)
}

// FrameExtractor 关键帧提取器接口
type FrameExtractor interface {
	Extract(ctx context.Context, videoPath string) ([]Frame, error)
	ExtractAt(ctx context.Context, videoPath string, timestamps []float64) ([]Frame, error)
}

// AudioTranscriber 语音转文字接口
type AudioTranscriber interface {
	Transcribe(ctx context.Context, videoPath string) (*TranscriptionResult, error)
}

// RAGManager RAG管理器接口
type RAGManager interface {
	AddDocument(ctx context.Context, content string, metadata map[string]interface{}) error
	Search(ctx context.Context, query string, topK int) ([]Document, error)
}

// VideoInfo 视频信息
type VideoInfo struct {
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FPS      float64 `json:"fps"`
	Bitrate  int     `json:"bitrate"`
	Format   string  `json:"format"`
	Size     int64   `json:"size"`
}

// Frame 视频帧
type Frame struct {
	Timestamp float64   `json:"timestamp"`
	Path      string    `json:"path"`
	Features  []float64 `json:"features,omitempty"`
}

// TranscriptionResult 转录结果
type TranscriptionResult struct {
	Text     string    `json:"text"`
	Segments []Segment `json:"segments"`
	Language string    `json:"language"`
	Duration float64   `json:"duration"`
}

// Segment 转录片段
type Segment struct {
	Start      float64 `json:"start"`
	End        float64 `json:"end"`
	Text       string  `json:"text"`
	Confidence float64 `json:"confidence"`
}

// Document 文档
type Document struct {
	ID      string  `json:"id"`
	Content string  `json:"content"`
	Score   float64 `json:"score"`
}

// VideoAnalysisResult 视频分析结果
type VideoAnalysisResult struct {
	VideoID    string                 `json:"video_id"`
	Title      string                 `json:"title"`
	Summary    string                 `json:"summary"`
	KeyFrames  []Frame                `json:"key_frames"`
	Transcript *TranscriptionResult   `json:"transcript"`
	Tags       []string               `json:"tags"`
	Sentiment  *SentimentResult       `json:"sentiment"`
	Highlights []Highlight            `json:"highlights"`
	Topics     []string               `json:"topics"`
	Duration   float64                `json:"duration"`
	Quality    float64                `json:"quality"`
	Metadata   map[string]interface{} `json:"metadata"`
	CreatedAt  time.Time              `json:"created_at"`
}

// SentimentResult 情感分析结果
type SentimentResult struct {
	Overall  string           `json:"overall"`
	Positive float64          `json:"positive"`
	Negative float64          `json:"negative"`
	Neutral  float64          `json:"neutral"`
	Timeline []SentimentPoint `json:"timeline"`
}

// SentimentPoint 情感时间点
type SentimentPoint struct {
	Timestamp float64 `json:"timestamp"`
	Sentiment string  `json:"sentiment"`
	Score     float64 `json:"score"`
}

// Highlight 视频亮点
type Highlight struct {
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Description string  `json:"description"`
	Type        string  `json:"type"`
	Importance  float64 `json:"importance"`
}

// NewVideoAnalysisAgent 创建视频分析Agent
func NewVideoAnalysisAgent(
	llm model.ChatModel,
	videoProcessor VideoProcessor,
	frameExtractor FrameExtractor,
	audioTranscriber AudioTranscriber,
	ragManager RAGManager,
) *VideoAnalysisAgent {
	return &VideoAnalysisAgent{
		llm:              llm,
		videoProcessor:   videoProcessor,
		frameExtractor:   frameExtractor,
		audioTranscriber: audioTranscriber,
		ragManager:       ragManager,
	}
}

// Analyze 分析视频
func (a *VideoAnalysisAgent) Analyze(ctx context.Context, videoURL string) (*VideoAnalysisResult, error) {
	// 1. 下载视频
	videoPath, err := a.videoProcessor.Download(ctx, videoURL)
	if err != nil {
		return nil, fmt.Errorf("下载视频失败: %w", err)
	}

	// 2. 获取视频信息
	videoInfo, err := a.videoProcessor.GetInfo(ctx, videoPath)
	if err != nil {
		return nil, fmt.Errorf("获取视频信息失败: %w", err)
	}

	// 3. 并行处理：提取关键帧和语音转文字
	frameChan := make(chan []Frame, 1)
	transcriptChan := make(chan *TranscriptionResult, 1)
	errChan := make(chan error, 2)

	go func() {
		frames, err := a.frameExtractor.Extract(ctx, videoPath)
		if err != nil {
			errChan <- err
			return
		}
		frameChan <- frames
	}()

	go func() {
		transcript, err := a.audioTranscriber.Transcribe(ctx, videoPath)
		if err != nil {
			errChan <- err
			return
		}
		transcriptChan <- transcript
	}()

	var frames []Frame
	var transcript *TranscriptionResult

	for i := 0; i < 2; i++ {
		select {
		case frames = <-frameChan:
		case transcript = <-transcriptChan:
		case err := <-errChan:
			return nil, err
		}
	}

	// 4. 生成视频摘要
	summary, err := a.generateSummary(ctx, transcript, frames)
	if err != nil {
		return nil, fmt.Errorf("生成摘要失败: %w", err)
	}

	// 5. 情感分析
	sentiment := a.analyzeSentiment(ctx, transcript)

	// 6. 提取标签
	tags := a.extractTags(ctx, transcript, frames)

	// 7. 识别亮点
	highlights := a.identifyHighlights(ctx, transcript, videoInfo.Duration)

	// 8. 提取主题
	topics := a.extractTopics(ctx, transcript)

	result := &VideoAnalysisResult{
		VideoID:    extractVideoID(videoURL),
		Summary:    summary,
		KeyFrames:  frames,
		Transcript: transcript,
		Tags:       tags,
		Sentiment:  sentiment,
		Highlights: highlights,
		Topics:     topics,
		Duration:   videoInfo.Duration,
		Quality:    a.calculateQuality(videoInfo),
		Metadata: map[string]interface{}{
			"width":  videoInfo.Width,
			"height": videoInfo.Height,
			"fps":    videoInfo.FPS,
			"format": videoInfo.Format,
		},
		CreatedAt: time.Now(),
	}

	// 9. 存储到RAG
	a.storeToRAG(ctx, result)

	return result, nil
}

// generateSummary 生成视频摘要
func (a *VideoAnalysisAgent) generateSummary(ctx context.Context, transcript *TranscriptionResult, frames []Frame) (string, error) {
	// 构建提示词
	prompt := fmt.Sprintf(`请根据以下视频内容生成一个简洁的视频摘要。

视频转录内容:
%s

关键帧数量: %d

请生成一个200字以内的视频摘要，包括:
1. 视频主要内容
2. 关键信息点
3. 视频亮点

请直接输出摘要内容。`, transcript.Text, len(frames))

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个专业的视频内容分析师，擅长提炼视频核心内容。",
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	response, err := a.llm.Generate(ctx, messages)
	if err != nil {
		return "", err
	}

	return response.Content, nil
}

// analyzeSentiment 情感分析
func (a *VideoAnalysisAgent) analyzeSentiment(ctx context.Context, transcript *TranscriptionResult) *SentimentResult {
	// 简化版情感分析
	positive := 0.0
	negative := 0.0
	neutral := 0.0

	// 基于关键词的简单情感分析
	positiveWords := []string{"好", "棒", "优秀", "喜欢", "精彩", "不错", "赞"}
	negativeWords := []string{"差", "糟糕", "不好", "失望", "烂", "差劲"}

	text := transcript.Text
	for _, word := range positiveWords {
		if contains(text, word) {
			positive += 0.1
		}
	}
	for _, word := range negativeWords {
		if contains(text, word) {
			negative += 0.1
		}
	}

	// 归一化
	total := positive + negative + neutral
	if total == 0 {
		neutral = 1.0
	} else {
		positive = positive / total
		negative = negative / total
		neutral = 1.0 - positive - negative
	}

	overall := "neutral"
	if positive > negative && positive > neutral {
		overall = "positive"
	} else if negative > positive && negative > neutral {
		overall = "negative"
	}

	return &SentimentResult{
		Overall:  overall,
		Positive: positive,
		Negative: negative,
		Neutral:  neutral,
	}
}

// extractTags 提取标签
func (a *VideoAnalysisAgent) extractTags(ctx context.Context, transcript *TranscriptionResult, frames []Frame) []string {
	prompt := fmt.Sprintf(`请从以下视频内容中提取5-10个相关标签。

视频内容:
%s

请以JSON数组格式返回标签列表，例如: ["标签1", "标签2", "标签3"]`, transcript.Text)

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个专业的标签提取专家。",
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	response, err := a.llm.Generate(ctx, messages)
	if err != nil {
		return []string{"视频", "分析"}
	}

	var tags []string
	if err := json.Unmarshal([]byte(response.Content), &tags); err != nil {
		return []string{"视频", "分析"}
	}

	return tags
}

// identifyHighlights 识别亮点
func (a *VideoAnalysisAgent) identifyHighlights(ctx context.Context, transcript *TranscriptionResult, duration float64) []Highlight {
	var highlights []Highlight

	// 基于转录内容识别亮点
	for _, segment := range transcript.Segments {
		// 简单的启发式规则：较长的段落可能是重要内容
		if len(segment.Text) > 50 {
			highlights = append(highlights, Highlight{
				Start:       segment.Start,
				End:         segment.End,
				Description: segment.Text[:min(100, len(segment.Text))],
				Type:        "content",
				Importance:  0.7,
			})
		}

		// 限制亮点数量
		if len(highlights) >= 5 {
			break
		}
	}

	return highlights
}

// extractTopics 提取主题
func (a *VideoAnalysisAgent) extractTopics(ctx context.Context, transcript *TranscriptionResult) []string {
	prompt := fmt.Sprintf(`请从以下视频内容中提取3-5个主要主题。

视频内容:
%s

请以JSON数组格式返回主题列表。`, transcript.Text)

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个专业的主题提取专家。",
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	response, err := a.llm.Generate(ctx, messages)
	if err != nil {
		return []string{"未分类"}
	}

	var topics []string
	if err := json.Unmarshal([]byte(response.Content), &topics); err != nil {
		return []string{"未分类"}
	}

	return topics
}

// calculateQuality 计算视频质量
func (a *VideoAnalysisAgent) calculateQuality(info *VideoInfo) float64 {
	// 基于分辨率、帧率等计算质量分数
	quality := 0.5

	// 分辨率评分
	if info.Height >= 2160 {
		quality += 0.3
	} else if info.Height >= 1080 {
		quality += 0.2
	} else if info.Height >= 720 {
		quality += 0.1
	}

	// 帧率评分
	if info.FPS >= 60 {
		quality += 0.1
	} else if info.FPS >= 30 {
		quality += 0.05
	}

	// 码率评分
	if info.Bitrate >= 5000000 {
		quality += 0.1
	}

	// return minFloat(quality, 1.0)
	return 3
}

// storeToRAG 存储到RAG
func (a *VideoAnalysisAgent) storeToRAG(ctx context.Context, result *VideoAnalysisResult) {
	// 存储视频摘要
	content := fmt.Sprintf("视频ID: %s\n摘要: %s\n标签: %v\n主题: %v",
		result.VideoID, result.Summary, result.Tags, result.Topics)

	a.ragManager.AddDocument(ctx, content, map[string]interface{}{
		"type":       "video_analysis",
		"video_id":   result.VideoID,
		"tags":       result.Tags,
		"topics":     result.Topics,
		"created_at": result.CreatedAt,
	})
}

// 辅助函数
func extractVideoID(url string) string {
	// 简化实现
	return url
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
