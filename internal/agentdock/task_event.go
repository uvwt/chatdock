package agentdock

import (
	"encoding/json"
	"strings"

	"chatdock/internal/model"
)

func LooksLikeTaskManageEvent(event model.MessageEvent) bool {
	text := strings.ToLower(strings.TrimSpace(event.Meta + " " + event.Text))
	return strings.Contains(text, "task_manage")
}

func SessionTaskIDFromEvent(event model.MessageEvent) string {
	details := mapValue(event.Details)
	data := mapValue(details["data"])
	tool := firstNonEmpty(stringValue(details["tool"]), stringValue(data["tool"]))
	outerArgs := mapValue(details["arguments"])
	if len(outerArgs) == 0 {
		outerArgs = mapValue(data["arguments"])
	}
	outerResult := mapValue(details["result"])
	if len(outerResult) == 0 {
		outerResult = mapValue(data["result"])
	}

	actualTool := tool
	actualArgs := outerArgs
	actualResult := outerResult
	if tool == "chatdock_tool_execute" {
		actualTool = firstNonEmpty(stringValue(outerArgs["name"]), stringValue(outerResult["tool"]))
		actualArgs = mapValue(outerArgs["arguments"])
		actualResult = mapValue(outerResult["result"])
	}
	if !isTaskManageTool(actualTool) || !isSessionTaskAction(stringValue(actualArgs["action"])) {
		return ""
	}
	return firstNonEmpty(
		stringValue(actualResult["task_id"]),
		stringValue(mapValue(actualResult["task_summary"])["id"]),
		stringValue(actualArgs["task_id"]),
	)
}

func isTaskManageTool(name string) bool {
	name = strings.TrimSpace(name)
	return name == "task_manage" || strings.HasSuffix(name, ".task_manage") || strings.HasSuffix(name, "__task_manage")
}

func isSessionTaskAction(action string) bool {
	switch strings.TrimSpace(action) {
	case "create", "checkpoint", "block", "resume", "final_review", "complete":
		return true
	default:
		return false
	}
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
