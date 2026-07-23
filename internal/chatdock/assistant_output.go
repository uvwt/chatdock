package chatdock

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
)

type assistantOutputRecorder struct {
	app       *App
	sessionID string
	jobID     string

	answer    strings.Builder
	reasoning strings.Builder
	parts     messagePartsRecorder

	checkpointMessageID string
	assistantSaved      bool
	messageError        *model.MessageError
	lastCheckpoint      time.Time
	lastCheckpointChars int
	pendingDelta        llm.StreamDelta
	pendingDeltaChars   int
	lastDeltaFlush      time.Time
	flushedContent      bool
	flushedReasoning    bool
}

func newAssistantOutputRecorder(app *App, sessionID string, jobID string) *assistantOutputRecorder {
	return &assistantOutputRecorder{
		app:            app,
		sessionID:      sessionID,
		jobID:          jobID,
		lastDeltaFlush: time.Now(),
	}
}

func (r *assistantOutputRecorder) emit(event string, value any) error {
	r.parts.record(event, value)
	if event == "delta" {
		if delta, ok := value.(llm.StreamDelta); ok {
			r.appendDelta(delta)
			if err := r.saveCheckpoint(false); err != nil {
				return err
			}
			if r.jobID == "" {
				return nil
			}
			return r.flushDeltaEvent(false)
		}
	}
	if r.jobID == "" {
		return r.saveCheckpoint(false)
	}
	if err := r.flushDeltaEvent(true); err != nil {
		return err
	}
	_, err := r.app.store.AddChatJobEvent(r.jobID, event, value)
	return err
}

func (r *assistantOutputRecorder) appendDelta(delta llm.StreamDelta) {
	if delta.Content != "" {
		r.answer.WriteString(delta.Content)
		if r.jobID != "" {
			r.pendingDelta.Content += delta.Content
			r.pendingDeltaChars += len(delta.Content)
		}
	}
	if delta.ReasoningContent != "" {
		r.reasoning.WriteString(delta.ReasoningContent)
		if r.jobID != "" {
			r.pendingDelta.ReasoningContent += delta.ReasoningContent
			r.pendingDeltaChars += len(delta.ReasoningContent)
		}
	}
}

func (r *assistantOutputRecorder) flushDeltaEvent(force bool) error {
	if r.pendingDelta.Content == "" && r.pendingDelta.ReasoningContent == "" {
		return nil
	}
	// 首个正文和首个思考分片分别立即落库，避免模型已经开始输出，
	// 前端却还要等待批量阈值或任务结束才能看到第一段内容。
	firstVisibleKind := (r.pendingDelta.Content != "" && !r.flushedContent) ||
		(r.pendingDelta.ReasoningContent != "" && !r.flushedReasoning)
	if !force && !firstVisibleKind && r.pendingDeltaChars < 512 && time.Since(r.lastDeltaFlush) < 250*time.Millisecond {
		return nil
	}
	delta := r.pendingDelta
	r.pendingDelta = llm.StreamDelta{}
	r.pendingDeltaChars = 0
	r.lastDeltaFlush = time.Now()
	if delta.Content != "" {
		r.flushedContent = true
	}
	if delta.ReasoningContent != "" {
		r.flushedReasoning = true
	}
	_, err := r.app.store.AddChatJobEvent(r.jobID, "delta", delta)
	return err
}

func (r *assistantOutputRecorder) saveCheckpoint(force bool) error {
	currentAnswer := r.answer.String()
	currentReasoning := r.reasoning.String()
	if !force && len(currentAnswer)-r.lastCheckpointChars < 512 && time.Since(r.lastCheckpoint) < time.Second {
		return nil
	}
	if strings.TrimSpace(currentAnswer) == "" && strings.TrimSpace(currentReasoning) == "" && len(r.parts.parts) == 0 && len(r.parts.events) == 0 && r.messageError == nil {
		return nil
	}
	_, messageID, err := r.app.store.UpsertAssistantMessageCheckpoint(r.sessionID, r.checkpointMessageID, currentAnswer, currentReasoning, r.parts.parts, r.parts.events, r.messageError)
	if err != nil {
		return err
	}
	r.checkpointMessageID = messageID
	r.assistantSaved = true
	r.lastCheckpoint = time.Now()
	r.lastCheckpointChars = len(currentAnswer)
	return nil
}

func (r *assistantOutputRecorder) setError(messageError model.MessageError) {
	errorCopy := messageError
	r.messageError = &errorCopy
}

func (r *assistantOutputRecorder) useFinalAnswer(finalAnswer string) {
	if strings.TrimSpace(finalAnswer) == "" || strings.TrimSpace(finalAnswer) == strings.TrimSpace(r.answer.String()) {
		return
	}
	r.answer.Reset()
	r.answer.WriteString(finalAnswer)
}

func (r *assistantOutputRecorder) ensureFailureAnswer(runErr error) {
	if runErr == nil || strings.TrimSpace(r.answer.String()) != "" {
		return
	}
	if strings.TrimSpace(r.reasoning.String()) == "" && len(r.parts.parts) == 0 && len(r.parts.events) == 0 {
		return
	}
	r.answer.WriteString("运行失败：" + strings.TrimSpace(runErr.Error()))
}

func (r *assistantOutputRecorder) answerText() string {
	return r.answer.String()
}

func (r *assistantOutputRecorder) reasoningText() string {
	return r.reasoning.String()
}

func (r *assistantOutputRecorder) assistantWasSaved() bool {
	return r.assistantSaved
}
