package chatdock

import (
	"context"
	"fmt"
	"strings"
	"time"

	"chatdock/internal/chatdock/mcp"
	"chatdock/internal/chatdock/model"
)

const (
	builtinToolListScheduledTasks   = "chatdock_scheduled_tasks_list"
	builtinToolCreateScheduledTask  = "chatdock_scheduled_task_create"
	builtinToolUpdateScheduledTask  = "chatdock_scheduled_task_update"
	builtinToolDeleteScheduledTask  = "chatdock_scheduled_task_delete"
	builtinToolRunScheduledTask     = "chatdock_scheduled_task_run"
	builtinToolEnableScheduledTask  = "chatdock_scheduled_task_set_enabled"
	builtinToolServerScheduledTasks = "chatdock"
)

func builtinScheduledTaskTools() []mcp.MCPTool {
	requestProps := map[string]any{
		"title":            map[string]any{"type": "string", "description": "任务标题，必须简短明确"},
		"prompt":           map[string]any{"type": "string", "description": "任务触发时发给模型的用户提示词"},
		"enabled":          map[string]any{"type": "boolean", "description": "是否启用任务，创建时默认 true"},
		"schedule_type":    map[string]any{"type": "string", "enum": []string{"once", "daily", "interval"}, "description": "once=指定时间一次，daily=每天固定时间，interval=按分钟间隔"},
		"run_at":           map[string]any{"type": "string", "description": "once 使用，RFC3339 或本地时间 yyyy-MM-ddTHH:mm / yyyy-MM-dd HH:mm"},
		"time_of_day":      map[string]any{"type": "string", "description": "daily 使用，HH:MM"},
		"interval_minutes": map[string]any{"type": "integer", "description": "interval 使用，间隔分钟，1 到 525600"},
	}
	return []mcp.MCPTool{
		{
			Server:      builtinToolServerScheduledTasks,
			Name:        "scheduled_tasks_list",
			FullName:    builtinToolListScheduledTasks,
			Title:       "查询定时任务",
			Description: "查询当前工作空间的定时任务。可传 id 精确查询，或传 query 按标题/提示词模糊过滤。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "query": map[string]any{"type": "string"}}},
		},
		{
			Server:      builtinToolServerScheduledTasks,
			Name:        "scheduled_task_create",
			FullName:    builtinToolCreateScheduledTask,
			Title:       "创建定时任务",
			Description: "在当前工作空间创建定时任务。创建前应确认标题、任务提示词和触发规则。",
			InputSchema: map[string]any{"type": "object", "properties": requestProps, "required": []string{"title", "prompt", "schedule_type"}},
		},
		{
			Server:      builtinToolServerScheduledTasks,
			Name:        "scheduled_task_update",
			FullName:    builtinToolUpdateScheduledTask,
			Title:       "修改定时任务",
			Description: "按 id 修改当前工作空间的定时任务。只传需要修改的字段；未传字段会保留原值。",
			InputSchema: map[string]any{"type": "object", "properties": mergeSchemaProps(map[string]any{"id": map[string]any{"type": "string", "description": "要修改的任务 id"}}, requestProps), "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerScheduledTasks,
			Name:        "scheduled_task_delete",
			FullName:    builtinToolDeleteScheduledTask,
			Title:       "删除定时任务",
			Description: "按 id 删除当前工作空间的定时任务。删除前应确保用户明确要求删除。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "要删除的任务 id"}}, "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerScheduledTasks,
			Name:        "scheduled_task_run",
			FullName:    builtinToolRunScheduledTask,
			Title:       "立即运行定时任务",
			Description: "按 id 手动运行当前工作空间的定时任务，会复用该任务绑定的会话上下文。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "description": "要运行的任务 id"}}, "required": []string{"id"}},
		},
		{
			Server:      builtinToolServerScheduledTasks,
			Name:        "scheduled_task_set_enabled",
			FullName:    builtinToolEnableScheduledTask,
			Title:       "启停定时任务",
			Description: "按 id 启用或停用当前工作空间的定时任务。",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string"}, "enabled": map[string]any{"type": "boolean"}}, "required": []string{"id", "enabled"}},
		},
	}
}

func mergeSchemaProps(base map[string]any, extra map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(extra))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range extra {
		out[k] = v
	}
	return out
}

func isBuiltinScheduledTaskTool(name string) bool {
	switch name {
	case builtinToolListScheduledTasks, builtinToolCreateScheduledTask, builtinToolUpdateScheduledTask, builtinToolDeleteScheduledTask, builtinToolRunScheduledTask, builtinToolEnableScheduledTask:
		return true
	default:
		return false
	}
}

func (a *App) callBuiltinScheduledTaskTool(ctx context.Context, name string, args map[string]any) (any, error) {
	// 内置工具只操作“当前工作空间”的定时任务，避免模型跨工作空间误删或误改用户数据。
	switch name {
	case builtinToolListScheduledTasks:
		return a.builtinListScheduledTasks(args)
	case builtinToolCreateScheduledTask:
		input, err := scheduledTaskRequestFromArgs(args, nil)
		if err != nil {
			return nil, err
		}
		return a.store.CreateScheduledTask(input)
	case builtinToolUpdateScheduledTask:
		id, err := requiredStringArg(args, "id")
		if err != nil {
			return nil, err
		}
		previous, err := a.findScheduledTask(id)
		if err != nil {
			return nil, err
		}
		input, err := scheduledTaskRequestFromArgs(args, &previous)
		if err != nil {
			return nil, err
		}
		return a.store.UpdateScheduledTask(id, input)
	case builtinToolDeleteScheduledTask:
		id, err := requiredStringArg(args, "id")
		if err != nil {
			return nil, err
		}
		return a.store.DeleteScheduledTask(id)
	case builtinToolRunScheduledTask:
		id, err := requiredStringArg(args, "id")
		if err != nil {
			return nil, err
		}
		return a.executeScheduledTask(ctx, a.store.ActivePrompt(), id, true)
	case builtinToolEnableScheduledTask:
		id, err := requiredStringArg(args, "id")
		if err != nil {
			return nil, err
		}
		enabled, err := requiredBoolArg(args, "enabled")
		if err != nil {
			return nil, err
		}
		previous, err := a.findScheduledTask(id)
		if err != nil {
			return nil, err
		}
		input := requestFromScheduledTask(previous)
		input.Enabled = enabled
		return a.store.UpdateScheduledTask(id, input)
	default:
		return nil, fmt.Errorf("unknown builtin tool: %s", name)
	}
}

func (a *App) builtinListScheduledTasks(args map[string]any) (model.ScheduledTaskResponse, error) {
	result, err := a.store.ListScheduledTasks()
	if err != nil {
		return model.ScheduledTaskResponse{}, err
	}
	id := strings.TrimSpace(stringArg(args, "id"))
	query := strings.ToLower(strings.TrimSpace(stringArg(args, "query")))
	if id == "" && query == "" {
		return result, nil
	}
	filtered := make([]model.ScheduledTask, 0, len(result.Tasks))
	for _, task := range result.Tasks {
		if id != "" && task.ID != id {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(task.Title), query) && !strings.Contains(strings.ToLower(task.Prompt), query) {
			continue
		}
		filtered = append(filtered, task)
	}
	return model.ScheduledTaskResponse{Tasks: filtered}, nil
}

func (a *App) findScheduledTask(id string) (model.ScheduledTask, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return model.ScheduledTask{}, fmt.Errorf("scheduled task id is empty")
	}
	result, err := a.store.ListScheduledTasks()
	if err != nil {
		return model.ScheduledTask{}, err
	}
	for _, task := range result.Tasks {
		if task.ID == id {
			return task, nil
		}
	}
	return model.ScheduledTask{}, fmt.Errorf("scheduled task not found: %s", id)
}

func scheduledTaskRequestFromArgs(args map[string]any, previous *model.ScheduledTask) (model.ScheduledTaskRequest, error) {
	input := model.ScheduledTaskRequest{Enabled: true}
	if previous != nil {
		input = requestFromScheduledTask(*previous)
	}
	if value, ok := optionalStringArg(args, "title"); ok {
		input.Title = value
	}
	if value, ok := optionalStringArg(args, "prompt"); ok {
		input.Prompt = value
	}
	if value, ok := optionalBoolArg(args, "enabled"); ok {
		input.Enabled = value
	}
	if value, ok := optionalStringArg(args, "schedule_type"); ok {
		input.ScheduleType = strings.ToLower(value)
	}
	if value, ok := optionalStringArg(args, "run_at"); ok {
		input.RunAt = value
	}
	if value, ok := optionalStringArg(args, "time_of_day"); ok {
		input.TimeOfDay = value
	}
	if value, ok, err := optionalIntArg(args, "interval_minutes"); err != nil {
		return model.ScheduledTaskRequest{}, err
	} else if ok {
		input.IntervalMinutes = value
	}
	return input, nil
}

func requestFromScheduledTask(task model.ScheduledTask) model.ScheduledTaskRequest {
	input := model.ScheduledTaskRequest{Title: task.Title, Prompt: task.Prompt, Enabled: task.Enabled, ScheduleType: task.ScheduleType, TimeOfDay: task.TimeOfDay, IntervalMinutes: task.IntervalMinutes}
	if task.RunAt != nil {
		input.RunAt = task.RunAt.Format(time.RFC3339)
	}
	return input
}

func requiredStringArg(args map[string]any, key string) (string, error) {
	value := strings.TrimSpace(stringArg(args, key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requiredBoolArg(args map[string]any, key string) (bool, error) {
	value, ok := optionalBoolArg(args, key)
	if !ok {
		return false, fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func optionalStringArg(args map[string]any, key string) (string, bool) {
	if _, ok := args[key]; !ok {
		return "", false
	}
	return strings.TrimSpace(stringArg(args, key)), true
}

func stringArg(args map[string]any, key string) string {
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

func optionalBoolArg(args map[string]any, key string) (bool, bool) {
	value, ok := args[key]
	if !ok || value == nil {
		return false, false
	}
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "true", "1", "yes", "y", "on", "启用", "开启":
			return true, true
		case "false", "0", "no", "n", "off", "停用", "关闭":
			return false, true
		}
	}
	return false, false
}

func optionalIntArg(args map[string]any, key string) (int, bool, error) {
	value, ok := args[key]
	if !ok || value == nil {
		return 0, false, nil
	}
	switch v := value.(type) {
	case int:
		return v, true, nil
	case int64:
		return int(v), true, nil
	case float64:
		return int(v), true, nil
	case string:
		var out int
		if _, err := fmt.Sscanf(strings.TrimSpace(v), "%d", &out); err != nil {
			return 0, true, fmt.Errorf("%s must be integer", key)
		}
		return out, true, nil
	default:
		return 0, true, fmt.Errorf("%s must be integer", key)
	}
}
