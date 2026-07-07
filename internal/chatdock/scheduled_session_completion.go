package chatdock

import (
	"context"
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

type scheduledSessionCompletionResult struct {
	Answer         string
	Reasoning      string
	AssistantSaved bool
}

func (a *App) completeScheduledSessionWithRecordedEvents(ctx context.Context, sessionID string, cfg model.ModelConfig, history []model.Message) (scheduledSessionCompletionResult, error) {
	var answer strings.Builder
	var reasoning strings.Builder
	var parts messagePartsRecorder
	var checkpointMessageID string
	var lastCheckpoint time.Time
	lastCheckpointChars := 0
	var pendingDelta llm.StreamDelta
	var pendingDeltaChars int
	lastDeltaFlush := time.Now()
	assistantSaved := false

	flushDeltaEvent := func(force bool) error {
		if pendingDelta.Content == "" && pendingDelta.ReasoningContent == "" {
			return nil
		}
		if !force && pendingDeltaChars < 512 && time.Since(lastDeltaFlush) < 250*time.Millisecond {
			return nil
		}
		pendingDelta = llm.StreamDelta{}
		pendingDeltaChars = 0
		lastDeltaFlush = time.Now()
		return nil
	}

	saveCheckpoint := func(force bool) error {
		currentAnswer := answer.String()
		currentReasoning := reasoning.String()
		if !force && len(currentAnswer)-lastCheckpointChars < 512 && time.Since(lastCheckpoint) < time.Second {
			return nil
		}
		if strings.TrimSpace(currentAnswer) == "" && strings.TrimSpace(currentReasoning) == "" && len(parts.parts) == 0 && len(parts.events) == 0 {
			return nil
		}
		_, messageID, err := a.store.UpsertAssistantMessageCheckpoint(sessionID, checkpointMessageID, currentAnswer, currentReasoning, parts.parts, parts.events)
		if err != nil {
			return err
		}
		checkpointMessageID = messageID
		assistantSaved = true
		lastCheckpoint = time.Now()
		lastCheckpointChars = len(currentAnswer)
		return nil
	}

	emit := func(event string, value any) error {
		parts.record(event, value)
		if event == "delta" {
			if delta, ok := value.(llm.StreamDelta); ok {
				if delta.Content != "" {
					answer.WriteString(delta.Content)
					pendingDelta.Content += delta.Content
					pendingDeltaChars += len(delta.Content)
				}
				if delta.ReasoningContent != "" {
					reasoning.WriteString(delta.ReasoningContent)
					pendingDelta.ReasoningContent += delta.ReasoningContent
					pendingDeltaChars += len(delta.ReasoningContent)
				}
				if err := saveCheckpoint(false); err != nil {
					return err
				}
				return flushDeltaEvent(false)
			}
		}
		if err := flushDeltaEvent(true); err != nil {
			return err
		}
		return saveCheckpoint(false)
	}

	finalAnswer, runErr := a.completeWithRecordedTools(ctx, "", sessionID, cfg, history, emit)
	if err := flushDeltaEvent(true); err != nil && runErr == nil {
		runErr = err
	}
	if strings.TrimSpace(finalAnswer) != "" && strings.TrimSpace(finalAnswer) != strings.TrimSpace(answer.String()) {
		answer.Reset()
		answer.WriteString(finalAnswer)
	}
	if runErr != nil && strings.TrimSpace(answer.String()) == "" && (strings.TrimSpace(reasoning.String()) != "" || len(parts.parts) > 0 || len(parts.events) > 0) {
		answer.WriteString("运行失败：" + strings.TrimSpace(runErr.Error()))
	}
	if err := saveCheckpoint(true); err != nil {
		if runErr == nil {
			runErr = err
		}
	}
	return scheduledSessionCompletionResult{Answer: answer.String(), Reasoning: reasoning.String(), AssistantSaved: assistantSaved}, runErr
}
