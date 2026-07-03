package chatdock

import "chatdock/internal/chatdock/model"

type ServerConfig = model.ServerConfig
type AuthStatusResponse = model.AuthStatusResponse
type AuthLoginRequest = model.AuthLoginRequest
type AuthLoginResponse = model.AuthLoginResponse
type ModelConfig = model.ModelConfig
type PublicModelConfig = model.PublicModelConfig
type PromptSpace = model.PromptSpace
type CreatePromptRequest = model.CreatePromptRequest
type SelectPromptRequest = model.SelectPromptRequest
type PromptResponse = model.PromptResponse
type MCPConfigResponse = model.MCPConfigResponse
type SaveMCPConfigRequest = model.SaveMCPConfigRequest
type MCPToolsResponse = model.MCPToolsResponse
type Skill = model.Skill
type SaveSkillRequest = model.SaveSkillRequest
type SkillResponse = model.SkillResponse
type ScheduledTask = model.ScheduledTask
type ScheduledTaskRequest = model.ScheduledTaskRequest
type ScheduledTaskResponse = model.ScheduledTaskResponse
type ScheduledTaskRun = model.ScheduledTaskRun
type ScheduledTaskRunResponse = model.ScheduledTaskRunResponse
type Message = model.Message
type Attachment = model.Attachment
type AttachmentRecord = model.AttachmentRecord
type FileUploadResponse = model.FileUploadResponse
type Session = model.Session
type SessionSummary = model.SessionSummary
type RenameSessionRequest = model.RenameSessionRequest
type PinSessionRequest = model.PinSessionRequest
type ChatRequest = model.ChatRequest
type ChatResponse = model.ChatResponse

const (
	ContextModeAuto     = model.ContextModeAuto
	ContextModeCompact  = model.ContextModeCompact
	ContextModeExpanded = model.ContextModeExpanded
	ContextModeCustom   = model.ContextModeCustom
)

var ErrSessionNotFound = model.ErrSessionNotFound

func NewID() string { return model.NewID() }

func DefaultModelConfig() ModelConfig { return model.DefaultModelConfig() }

func NormalizeModelConfig(cfg ModelConfig) ModelConfig { return model.NormalizeModelConfig(cfg) }

func ToPublicModelConfig(cfg ModelConfig) PublicModelConfig { return model.ToPublicModelConfig(cfg) }

func buildUserContentForModel(content string, attachments []AttachmentRecord) string {
	return model.BuildUserContentForModel(content, attachments)
}
