package model

import "time"

type Message struct {
	ID               string             `json:"id,omitempty"`
	Role             string             `json:"role"`
	Content          string             `json:"content"`
	Reasoning        string             `json:"reasoning,omitempty"`
	Attachments      []Attachment       `json:"attachments,omitempty"`
	ModelAttachments []AttachmentRecord `json:"-"`
	CreatedAt        time.Time          `json:"created_at"`
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
	Prompt      string
	SessionID   string
	MessageID   string
	StoragePath string
	SHA256      string
	TextContent string
}

type Session struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Pinned    bool      `json:"pinned"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Messages  []Message `json:"messages"`
}

type SessionSummary struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Pinned    bool      `json:"pinned"`
	Preview   string    `json:"preview,omitempty"`
	LastRole  string    `json:"last_role,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Count     int       `json:"count"`
}

type RenameSessionRequest struct {
	Title string `json:"title"`
}

type PinSessionRequest struct {
	Pinned bool `json:"pinned"`
}

type ChatRequest struct {
	SessionID     string   `json:"session_id"`
	Message       string   `json:"message"`
	AttachmentIDs []string `json:"attachment_ids,omitempty"`
}

type ChatResponse struct {
	Answer  string   `json:"answer"`
	Session *Session `json:"session"`
}
