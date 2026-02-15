package agent

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// RecommendationAgent 推荐Agent
type RecommendationAgent struct {
	llm          model.ChatModel
	userProfile  UserProfileService
	contentModel ContentModel
	vectorStore  VectorStore
	videoStore   VideoStore
}

// UserProfileService 用户画像服务接口
type UserProfileService interface {
	Get(ctx context.Context, userID string) (*UserProfile, error)
	Update(ctx context.Context, userID string, profile *UserProfile) error
}

// ContentModel 内容模型接口
type ContentModel interface {
	GetSimilar(ctx context.Context, videoID string, limit int) ([]string, error)
	GetTrending(ctx context.Context, limit int) ([]string, error)
}

// VectorStore 向量存储接口
type VectorStore interface {
	Search(ctx context.Context, vector []float64, topK int) ([]SearchResult, error)
}

// VideoStore 视频存储接口
type VideoStore interface {
	Get(ctx context.Context, videoID string) (*Video, error)
	GetByIDs(ctx context.Context, ids []string) ([]*Video, error)
}

// UserProfile 用户画像
type UserProfile struct {
	UserID       string             `json:"user_id"`
	Interests    []string           `json:"interests"`
	WatchHistory []WatchRecord      `json:"watch_history"`
	Preferences  map[string]float64 `json:"preferences"`
	ActiveHours  []int              `json:"active_hours"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

// WatchRecord 观看记录
type WatchRecord struct {
	VideoID   string    `json:"video_id"`
	Duration  float64   `json:"duration"`
	Progress  float64   `json:"progress"`
	WatchedAt time.Time `json:"watched_at"`
}

// Video 视频信息
type Video struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags"`
	Category    string    `json:"category"`
	Duration    float64   `json:"duration"`
	ViewCount   int64     `json:"view_count"`
	LikeCount   int64     `json:"like_count"`
	CreatedAt   time.Time `json:"created_at"`
	Embedding   []float64 `json:"embedding,omitempty"`
}

// SearchResult 搜索结果
type SearchResult struct {
	ID    string  `json:"id"`
	Score float64 `json:"score"`
}

// RecommendationRequest 推荐请求
type RecommendationRequest struct {
	UserID   string   `json:"user_id"`
	Context  string   `json:"context"`
	VideoID  string   `json:"video_id,omitempty"`
	Limit    int      `json:"limit"`
	Strategy []string `json:"strategy"`
	Filters  *Filters `json:"filters,omitempty"`
}

// Filters 过滤条件
type Filters struct {
	Categories  []string   `json:"categories,omitempty"`
	MinDuration float64    `json:"min_duration,omitempty"`
	MaxDuration float64    `json:"max_duration,omitempty"`
	DateRange   *DateRange `json:"date_range,omitempty"`
}

// DateRange 日期范围
type DateRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// RecommendationResult 推荐结果
type RecommendationResult struct {
	Videos      []RecommendedVideo `json:"videos"`
	Strategy    string             `json:"strategy"`
	Explanation string             `json:"explanation"`
	TotalCount  int                `json:"total_count"`
}

// RecommendedVideo 推荐视频
type RecommendedVideo struct {
	VideoID     string  `json:"video_id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Thumbnail   string  `json:"thumbnail"`
	Duration    float64 `json:"duration"`
	ViewCount   int64   `json:"view_count"`
	Score       float64 `json:"score"`
	Reason      string  `json:"reason"`
	Similarity  float64 `json:"similarity"`
	Strategy    string  `json:"strategy"`
}

// NewRecommendationAgent 创建推荐Agent
func NewRecommendationAgent(
	llm model.ChatModel,
	userProfile UserProfileService,
	contentModel ContentModel,
	vectorStore VectorStore,
	videoStore VideoStore,
) *RecommendationAgent {
	return &RecommendationAgent{
		llm:          llm,
		userProfile:  userProfile,
		contentModel: contentModel,
		vectorStore:  vectorStore,
		videoStore:   videoStore,
	}
}

// Recommend 生成推荐
func (a *RecommendationAgent) Recommend(ctx context.Context, req RecommendationRequest) (*RecommendationResult, error) {
	// 设置默认值
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if len(req.Strategy) == 0 {
		req.Strategy = []string{"personalized", "collaborative", "trending"}
	}

	// 获取用户画像
	profile, err := a.userProfile.Get(ctx, req.UserID)
	if err != nil {
		profile = &UserProfile{
			UserID:    req.UserID,
			Interests: []string{},
		}
	}

	// 基于策略生成推荐
	recommendations := make(map[string]*RecommendedVideo)

	for _, strategy := range req.Strategy {
		switch strategy {
		case "collaborative":
			recs := a.collaborativeFilter(ctx, profile, req.Limit/len(req.Strategy)+1)
			for _, rec := range recs {
				rec.Strategy = "collaborative"
				if existing, ok := recommendations[rec.VideoID]; ok {
					existing.Score = math.Max(existing.Score, rec.Score)
				} else {
					recommendations[rec.VideoID] = &rec
				}
			}

		case "content":
			if req.VideoID != "" {
				recs := a.contentBasedFilter(ctx, req.VideoID, req.Limit/len(req.Strategy)+1)
				for _, rec := range recs {
					rec.Strategy = "content"
					if existing, ok := recommendations[rec.VideoID]; ok {
						existing.Score = math.Max(existing.Score, rec.Score)
					} else {
						recommendations[rec.VideoID] = &rec
					}
				}
			}

		case "trending":
			recs := a.trendingFilter(ctx, req.Limit/len(req.Strategy)+1)
			for _, rec := range recs {
				rec.Strategy = "trending"
				if existing, ok := recommendations[rec.VideoID]; ok {
					existing.Score = math.Max(existing.Score, rec.Score)
				} else {
					recommendations[rec.VideoID] = &rec
				}
			}

		case "personalized":
			recs := a.personalizedFilter(ctx, profile, req.Limit/len(req.Strategy)+1)
			for _, rec := range recs {
				rec.Strategy = "personalized"
				if existing, ok := recommendations[rec.VideoID]; ok {
					existing.Score = math.Max(existing.Score, rec.Score)
				} else {
					recommendations[rec.VideoID] = &rec
				}
			}
		}
	}

	// 转换为列表并排序
	var videoList []RecommendedVideo
	for _, rec := range recommendations {
		videoList = append(videoList, *rec)
	}

	sort.Slice(videoList, func(i, j int) bool {
		return videoList[i].Score > videoList[j].Score
	})

	// 限制数量
	if len(videoList) > req.Limit {
		videoList = videoList[:req.Limit]
	}

	// 生成推荐理由
	explanation := a.generateExplanation(ctx, videoList, profile)

	return &RecommendationResult{
		Videos:      videoList,
		Strategy:    req.Strategy[0],
		Explanation: explanation,
		TotalCount:  len(videoList),
	}, nil
}

// collaborativeFilter 协同过滤
func (a *RecommendationAgent) collaborativeFilter(ctx context.Context, profile *UserProfile, limit int) []RecommendedVideo {
	// 基于相似用户的观看历史推荐
	// 简化实现：基于用户兴趣标签匹配
	var recommendations []RecommendedVideo

	// 获取热门视频
	videoIDs, err := a.contentModel.GetTrending(ctx, limit*2)
	if err != nil {
		return recommendations
	}

	videos, err := a.videoStore.GetByIDs(ctx, videoIDs)
	if err != nil {
		return recommendations
	}

	// 基于兴趣匹配
	for _, video := range videos {
		score := a.calculateInterestScore(video, profile.Interests)
		if score > 0.3 {
			recommendations = append(recommendations, RecommendedVideo{
				VideoID:    video.ID,
				Title:      video.Title,
				Duration:   video.Duration,
				ViewCount:  video.ViewCount,
				Score:      score,
				Reason:     "基于您的观看偏好推荐",
				Similarity: score,
			})
		}
	}

	return recommendations
}

// contentBasedFilter 基于内容的过滤
func (a *RecommendationAgent) contentBasedFilter(ctx context.Context, videoID string, limit int) []RecommendedVideo {
	var recommendations []RecommendedVideo

	// 获取相似视频
	similarIDs, err := a.contentModel.GetSimilar(ctx, videoID, limit)
	if err != nil {
		return recommendations
	}

	videos, err := a.videoStore.GetByIDs(ctx, similarIDs)
	if err != nil {
		return recommendations
	}

	for i, video := range videos {
		similarity := 1.0 - float64(i)*0.1
		recommendations = append(recommendations, RecommendedVideo{
			VideoID:    video.ID,
			Title:      video.Title,
			Duration:   video.Duration,
			ViewCount:  video.ViewCount,
			Score:      similarity,
			Reason:     "与当前视频内容相似",
			Similarity: similarity,
		})
	}

	return recommendations
}

// trendingFilter 热门推荐
func (a *RecommendationAgent) trendingFilter(ctx context.Context, limit int) []RecommendedVideo {
	var recommendations []RecommendedVideo

	videoIDs, err := a.contentModel.GetTrending(ctx, limit)
	if err != nil {
		return recommendations
	}

	videos, err := a.videoStore.GetByIDs(ctx, videoIDs)
	if err != nil {
		return recommendations
	}

	for i, video := range videos {
		score := 1.0 - float64(i)*0.05
		recommendations = append(recommendations, RecommendedVideo{
			VideoID:    video.ID,
			Title:      video.Title,
			Duration:   video.Duration,
			ViewCount:  video.ViewCount,
			Score:      score,
			Reason:     "当前热门视频",
			Similarity: score,
		})
	}

	return recommendations
}

// personalizedFilter 个性化推荐
func (a *RecommendationAgent) personalizedFilter(ctx context.Context, profile *UserProfile, limit int) []RecommendedVideo {
	var recommendations []RecommendedVideo

	// 基于用户画像的向量搜索
	if len(profile.WatchHistory) == 0 {
		// 新用户，返回热门推荐
		return a.trendingFilter(ctx, limit)
	}

	// 构建用户兴趣向量
	userVector := a.buildUserVector(profile)

	// 向量搜索
	results, err := a.vectorStore.Search(ctx, userVector, limit*2)
	if err != nil {
		return recommendations
	}

	// 获取视频详情
	var videoIDs []string
	for _, result := range results {
		videoIDs = append(videoIDs, result.ID)
	}

	videos, err := a.videoStore.GetByIDs(ctx, videoIDs)
	if err != nil {
		return recommendations
	}

	// 构建结果
	for i, video := range videos {
		if i < len(results) {
			recommendations = append(recommendations, RecommendedVideo{
				VideoID:    video.ID,
				Title:      video.Title,
				Duration:   video.Duration,
				ViewCount:  video.ViewCount,
				Score:      results[i].Score,
				Reason:     "根据您的兴趣个性化推荐",
				Similarity: results[i].Score,
			})
		}
	}

	return recommendations
}

// calculateInterestScore 计算兴趣匹配分数
func (a *RecommendationAgent) calculateInterestScore(video *Video, interests []string) float64 {
	if len(interests) == 0 {
		return 0.5
	}

	score := 0.0
	for _, interest := range interests {
		for _, tag := range video.Tags {
			if containsString(tag, interest) {
				score += 0.2
			}
		}
		if containsString(video.Category, interest) {
			score += 0.3
		}
	}

	return math.Min(score, 1.0)
}

// buildUserVector 构建用户向量
func (a *RecommendationAgent) buildUserVector(profile *UserProfile) []float64 {
	// 简化实现：基于观看历史构建向量
	// 实际项目中应该使用更复杂的算法
	vector := make([]float64, 128)

	for _, record := range profile.WatchHistory {
		// 根据观看进度加权
		weight := record.Progress / record.Duration
		for i := range vector {
			vector[i] += weight * 0.01
		}
	}

	// 归一化
	norm := 0.0
	for _, v := range vector {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	if norm > 0 {
		for i := range vector {
			vector[i] /= norm
		}
	}

	return vector
}

// generateExplanation 生成推荐理由
func (a *RecommendationAgent) generateExplanation(ctx context.Context, videos []RecommendedVideo, profile *UserProfile) string {
	if len(videos) == 0 {
		return "暂无推荐"
	}

	prompt := fmt.Sprintf(`请根据以下推荐视频生成一个简短的推荐理由说明。

推荐视频:
%s

用户兴趣: %v

请生成50字以内的推荐理由说明。`, formatVideos(videos), profile.Interests)

	messages := []*schema.Message{
		{
			Role:    schema.System,
			Content: "你是一个专业的推荐系统解释专家。",
		},
		{
			Role:    schema.User,
			Content: prompt,
		},
	}

	response, err := a.llm.Generate(ctx, messages)
	if err != nil {
		return "基于您的兴趣为您推荐以下视频"
	}

	return response.Content
}

// 辅助函数
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr))
}

func formatVideos(videos []RecommendedVideo) string {
	result := ""
	for i, video := range videos {
		if i >= 3 {
			break
		}
		result += fmt.Sprintf("- %s (策略: %s)\n", video.Title, video.Strategy)
	}
	return result
}
