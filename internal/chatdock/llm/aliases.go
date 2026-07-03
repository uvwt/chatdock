package llm

import (
	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
	"strings"
)

type ModelConfig = model.ModelConfig
type Message = model.Message
type Skill = model.Skill
type AttachmentRecord = model.AttachmentRecord
type MCPTool = mcp.MCPTool

func imageContentBlocks(attachments []AttachmentRecord) []map[string]any {
	return model.ImageContentBlocks(attachments)
}

const (
	ContextModeAuto     = model.ContextModeAuto
	ContextModeCompact  = model.ContextModeCompact
	ContextModeExpanded = model.ContextModeExpanded
	ContextModeCustom   = model.ContextModeCustom
)

func NormalizeModelConfig(cfg ModelConfig) ModelConfig {
	return model.NormalizeModelConfig(cfg)
}

func normalizeContextMode(mode string) string {
	switch strings.TrimSpace(mode) {
	case ContextModeCompact:
		return ContextModeCompact
	case ContextModeExpanded:
		return ContextModeExpanded
	case ContextModeCustom:
		return ContextModeCustom
	default:
		return ContextModeAuto
	}
}

func compactJSON(value any) string { return mcp.CompactJSON(value) }

func toolFullName(serverName, toolName string) string { return mcp.ToolFullName(serverName, toolName) }

func normalizeJSONSchema(schema map[string]any) map[string]any {
	return mcp.NormalizeJSONSchema(schema)
}
