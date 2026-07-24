package model

import "time"

type Message struct {
	ID               string             `json:"id,omitempty"`
	Role             string             `json:"role"`
	Content          string             `json:"content"`
	Reasoning        string             `json:"reasoning,omitempty"`
	Error            *MessageError      `json:"error,omitempty"`
	Parts            []MessagePart      `json:"parts,omitempty"`
	Events           []MessageEvent     `json:"events,omitempty"`
	Attachments      []Attachment       `json:"attachments,omitempty"`
	ModelAttachments []AttachmentRecord `json:"-"`
	CreatedAt        time.Time          `json:"created_at"`
}

// MessageError 将面向用户的提示和底层原始错误分开保存。
// 原始错误只在用户主动展开详情时展示，避免污染消息正文和后续模型上下文。
type MessageError struct {
	Message   string `json:"message"`
	Raw       string `json:"raw,omitempty"`
	Code      string `json:"code,omitempty"`
	RequestID string `json:"request_id,omitempty"`
	Retryable bool   `json:"retryable,omitempty"`
}

type MessagePart struct {
	Kind    string        `json:"kind"`
	Text    string        `json:"text,omitempty"`
	CallKey string        `json:"callKey,omitempty"`
	Event   *MessageEvent `json:"event,omitempty"`
}

type MessageEvent struct {
	ID      string         `json:"id,omitempty"`
	Kind    string         `json:"kind"`
	Phase   string         `json:"phase,omitempty"`
	CallKey string         `json:"callKey,omitempty"`
	Text    string         `json:"text,omitempty"`
	Meta    string         `json:"meta,omitempty"`
	Details map[string]any `json:"details,omitempty"`
}

type Attachment struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	MIMEType  string    `json:"mime_type"`
	Size      int64     `json:"size"`
	Status    string    `json:"status"`
	HasText   bool      `json:"has_text"`
	TextBytes int       `json:"text_bytes,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type FileUploadResponse struct {
	Attachment Attachment `json:"attachment"`
}

// AttachmentRecord 是附件的持久化记录；公开响应只使用 Attachment。
type AttachmentRecord struct {
	Attachment
	SessionID   string
	MessageID   string
	StoragePath string
	SHA256      string
	TextContent string
	ModelURL    string
}

type Session struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id,omitempty"`
	Title      string    `json:"title"`
	Pinned     bool      `json:"pinned"`
	ProviderID string    `json:"provider_id,omitempty"`
	Model      string    `json:"model,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Messages   []Message `json:"messages"`
}

type SessionSummary struct {
	ID         string    `json:"id"`
	ProjectID  string    `json:"project_id,omitempty"`
	Title      string    `json:"title"`
	Pinned     bool      `json:"pinned"`
	ProviderID string    `json:"provider_id,omitempty"`
	Model      string    `json:"model,omitempty"`
	Preview    string    `json:"preview,omitempty"`
	LastRole   string    `json:"last_role,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Count      int       `json:"count"`
}

type RenameSessionRequest struct {
	Title string `json:"title"`
}

type PinSessionRequest struct {
	Pinned bool `json:"pinned"`
}

type PinRequest struct {
	Pinned bool `json:"pinned"`
}

// UpdateSessionModelRequest 保存当前会话最后一次选择的模型。
type UpdateSessionModelRequest struct {
	ProviderID string `json:"provider_id,omitempty"`
	Model      string `json:"model,omitempty"`
}

// EditMessageRequest 指定要就地修改的用户消息；保存后会截断该消息之后的上下文。
type EditMessageRequest struct {
	MessageIndex *int   `json:"message_index,omitempty"`
	MessageID    string `json:"message_id,omitempty"`
	Content      string `json:"content"`
}

// BranchSessionRequest 指定从当前会话的哪条消息创建分支。
// message_index 为空时默认复制整个会话，前端消息操作会传入具体索引。
type BranchSessionRequest struct {
	MessageIndex *int `json:"message_index,omitempty"`
}

type ChatRequest struct {
	SessionID     string   `json:"session_id"`
	Message       string   `json:"message"`
	ProviderID    string   `json:"provider_id,omitempty"`
	Model         string   `json:"model,omitempty"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
	Regenerate    bool     `json:"regenerate,omitempty"`
}

type ChatResponse struct {
	Answer  string   `json:"answer"`
	Session *Session `json:"session"`
}
