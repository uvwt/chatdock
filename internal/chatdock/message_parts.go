package chatdock

import (
	"encoding/json"
	"strings"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

type messagePartsRecorder struct {
	parts  []model.MessagePart
	events []model.MessageEvent
}

func (r *messagePartsRecorder) record(event string, value any) {
	switch event {
	case "delta":
		r.recordDelta(value)
	case "tool_call_start":
		r.recordToolStart(value)
	case "tool_call_result":
		r.recordToolResult(value)
	case "model_fallback":
		r.recordModelFallback(value)
	}
}

func (r *messagePartsRecorder) recordModelFallback(value any) {
	data := mapValue(value)
	toProvider := stringValue(data["to_provider_id"])
	toModel := stringValue(data["to_model"])
	meta := strings.Trim(strings.Join([]string{toProvider, toModel}, " · "), " ·")
	event := model.MessageEvent{
		Kind:    "tool",
		Phase:   "done",
		CallKey: "model_fallback::" + toProvider + "::" + toModel,
		Text:    "切换备用模型",
		Meta:    meta,
		Details: map[string]any{"event": "model_fallback", "data": data},
	}
	r.events = append(r.events, event)
	r.parts = append(r.parts, model.MessagePart{Kind: "tool", CallKey: event.CallKey, Event: cloneEventPointer(event)})
}

func (r *messagePartsRecorder) recordDelta(value any) {
	delta, ok := value.(llm.StreamDelta)
	if !ok {
		return
	}
	if delta.ReasoningContent != "" {
		r.appendTextPart("reasoning", delta.ReasoningContent)
	}
	if delta.Content != "" {
		r.appendTextPart("text", delta.Content)
	}
}

func (r *messagePartsRecorder) appendTextPart(kind string, text string) {
	if text == "" {
		return
	}
	last := len(r.parts) - 1
	if last >= 0 && r.parts[last].Kind == kind {
		r.parts[last].Text += text
		return
	}
	r.parts = append(r.parts, model.MessagePart{Kind: kind, Text: text})
}

func (r *messagePartsRecorder) recordToolStart(value any) {
	data := mapValue(value)
	tool := stringValue(data["tool"])
	if tool == "" {
		return
	}
	args := mapValue(data["arguments"])
	callKey := toolCallKey(tool, args)
	event := model.MessageEvent{
		Kind:    "tool",
		Phase:   "running",
		CallKey: callKey,
		Text:    toolEventText("start", tool, true),
		Meta:    toolEventMeta(tool, args, mapValue(data["result"])),
		Details: map[string]any{"event": "tool_call_start", "tool": tool, "arguments": args, "data": data},
	}
	r.events = append(r.events, event)
	r.parts = append(r.parts, model.MessagePart{Kind: "tool", CallKey: callKey, Event: cloneEventPointer(event)})
}

func (r *messagePartsRecorder) recordToolResult(value any) {
	data := mapValue(value)
	tool := stringValue(data["tool"])
	if tool == "" {
		return
	}
	args := mapValue(data["arguments"])
	result := mapValue(data["result"])
	ok := boolValue(data["ok"])
	callKey := toolCallKey(tool, args)
	next := model.MessageEvent{
		Kind:    "tool",
		Phase:   toolResultPhase(ok),
		CallKey: callKey,
		Text:    toolEventText("result", tool, ok),
		Meta:    toolEventMeta(tool, args, result),
		Details: map[string]any{"event": "tool_call_result", "tool": tool, "ok": ok, "result": data["result"], "error": stringValue(data["error"]), "data": data},
	}
	match := func(event model.MessageEvent) bool {
		if event.Kind != "tool" || event.Phase != "running" {
			return false
		}
		if event.CallKey == callKey {
			return true
		}
		return len(args) == 0 && stringValue(event.Details["tool"]) == tool
	}
	for i := len(r.events) - 1; i >= 0; i-- {
		if match(r.events[i]) {
			next.CallKey = firstMessagePartNonEmpty(r.events[i].CallKey, callKey)
			if oldArgs, ok := r.events[i].Details["arguments"]; ok {
				next.Details["arguments"] = oldArgs
			}
			r.events[i] = next
			break
		}
	}
	updatedPart := false
	for i := len(r.parts) - 1; i >= 0; i-- {
		if r.parts[i].Kind == "tool" && r.parts[i].Event != nil && match(*r.parts[i].Event) {
			next.CallKey = firstMessagePartNonEmpty(r.parts[i].Event.CallKey, callKey)
			if oldArgs, ok := r.parts[i].Event.Details["arguments"]; ok {
				next.Details["arguments"] = oldArgs
			}
			r.parts[i].CallKey = next.CallKey
			r.parts[i].Event = cloneEventPointer(next)
			updatedPart = true
			break
		}
	}
	if !updatedPart {
		r.events = append(r.events, next)
		r.parts = append(r.parts, model.MessagePart{Kind: "tool", CallKey: callKey, Event: cloneEventPointer(next)})
	}
}

func toolResultPhase(ok bool) string {
	if ok {
		return "done"
	}
	return "error"
}

func toolCallKey(tool string, args map[string]any) string {
	raw, _ := json.Marshal(args)
	return tool + "::" + string(raw)
}

func toolEventText(phase string, tool string, ok bool) string {
	if phase == "start" {
		return "正在调用：" + tool
	}
	if ok {
		return "调用完成：" + tool
	}
	return "调用失败：" + tool
}

func toolEventMeta(tool string, args map[string]any, result map[string]any) string {
	if tool == "chatdock_tools_search" {
		if query := firstMessagePartNonEmpty(stringValue(args["query"]), stringValue(result["query"])); query != "" {
			return "关键词：" + query
		}
	}
	return firstMessagePartNonEmpty(stringValue(args["name"]), stringValue(result["tool"]))
}

func mapValue(value any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	if data, ok := value.(map[string]any); ok {
		return data
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	data := map[string]any{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{}
	}
	return data
}

func stringValue(value any) string {
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func boolValue(value any) bool {
	if value == nil {
		return false
	}
	if ok, isBool := value.(bool); isBool {
		return ok
	}
	return false
}

func firstMessagePartNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func cloneEventPointer(event model.MessageEvent) *model.MessageEvent {
	if len(event.Details) > 0 {
		details := make(map[string]any, len(event.Details))
		for key, value := range event.Details {
			details[key] = value
		}
		event.Details = details
	}
	return &event
}
