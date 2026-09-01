package model

// Usage 是供应商实际返回的对话模型用量。没有返回 usage 时，消息保持 nil，
// 调用方不能用字符数或本地估算值冒充供应商数据。
type Usage struct {
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	ReasoningTokens int    `json:"reasoning_tokens,omitempty"`
	CacheHitTokens  int    `json:"cache_hit_tokens,omitempty"`
	CacheMissTokens int    `json:"cache_miss_tokens,omitempty"`
	TotalTokens     int    `json:"total_tokens"`
	Source          string `json:"source,omitempty"`
}

func (u Usage) Empty() bool {
	return u.InputTokens == 0 && u.OutputTokens == 0 && u.ReasoningTokens == 0 &&
		u.CacheHitTokens == 0 && u.CacheMissTokens == 0 && u.TotalTokens == 0
}

func (u *Usage) Add(other Usage) {
	if u == nil {
		return
	}
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.ReasoningTokens += other.ReasoningTokens
	u.CacheHitTokens += other.CacheHitTokens
	u.CacheMissTokens += other.CacheMissTokens
	u.TotalTokens += other.TotalTokens
	if u.Source == "" {
		u.Source = other.Source
	}
	if u.TotalTokens == 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens + u.ReasoningTokens
	}
}

// UsageSummary 是会话标题栏使用的累计值，只聚合已由对话模型上报 usage 的回复。
type UsageSummary struct {
	InputTokens     int     `json:"input_tokens"`
	OutputTokens    int     `json:"output_tokens"`
	ReasoningTokens int     `json:"reasoning_tokens,omitempty"`
	CacheHitTokens  int     `json:"cache_hit_tokens,omitempty"`
	CacheMissTokens int     `json:"cache_miss_tokens,omitempty"`
	TotalTokens     int     `json:"total_tokens"`
	ReplyCount      int     `json:"reply_count"`
	MissingCount    int     `json:"missing_count,omitempty"`
	CacheHitRate    float64 `json:"cache_hit_rate,omitempty"`
	Status          string  `json:"status"`
}

// ModelLimit 是单个模型的上下文上限。供应商未配置时由 ChatDock 使用估算默认值。
type ModelLimit struct {
	ContextWindowTokens int `json:"context_window_tokens"`
	OutputReserveTokens int `json:"output_reserve_tokens"`
}
