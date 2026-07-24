package chatoutput

import (
	"strings"
	"time"

	"chatdock/internal/chatdock/llm"
	"chatdock/internal/chatdock/model"
	"chatdock/internal/chatdock/store"
)

type Recorder struct {
	store     *store.Store
	sessionID string
	jobID     string

	answer    strings.Builder
	reasoning strings.Builder
	timeline  timelineRecorder

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

func NewRecorder(store *store.Store, sessionID string, jobID string) *Recorder {
	return &Recorder{
		store:          store,
		sessionID:      sessionID,
		jobID:          jobID,
		lastDeltaFlush: time.Now(),
	}
}

func (r *Recorder) Emit(event string, value any) error {
	r.timeline.Record(event, value)
	if event == "delta" {
		if delta, ok := value.(llm.StreamDelta); ok {
			r.appendDelta(delta)
			if err := r.SaveCheckpoint(false); err != nil {
				return err
			}
			if r.jobID == "" {
				return nil
			}
			return r.FlushDeltaEvent(false)
		}
	}
	if r.jobID == "" {
		return r.SaveCheckpoint(false)
	}
	if err := r.FlushDeltaEvent(true); err != nil {
		return err
	}
	_, err := r.store.AddChatJobEvent(r.jobID, event, value)
	return err
}

func (r *Recorder) appendDelta(delta llm.StreamDelta) {
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

func (r *Recorder) FlushDeltaEvent(force bool) error {
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
	_, err := r.store.AddChatJobEvent(r.jobID, "delta", delta)
	return err
}

func (r *Recorder) SaveCheckpoint(force bool) error {
	currentAnswer := r.answer.String()
	currentReasoning := r.reasoning.String()
	if !force && len(currentAnswer)-r.lastCheckpointChars < 512 && time.Since(r.lastCheckpoint) < time.Second {
		return nil
	}
	if strings.TrimSpace(currentAnswer) == "" && strings.TrimSpace(currentReasoning) == "" && r.timeline.Empty() && r.messageError == nil {
		return nil
	}
	_, messageID, err := r.store.UpsertAssistantMessageCheckpoint(r.sessionID, r.checkpointMessageID, currentAnswer, currentReasoning, r.timeline.Parts(), r.timeline.Events(), r.messageError)
	if err != nil {
		return err
	}
	r.checkpointMessageID = messageID
	r.assistantSaved = true
	r.lastCheckpoint = time.Now()
	r.lastCheckpointChars = len(currentAnswer)
	return nil
}

func (r *Recorder) SetError(messageError model.MessageError) {
	errorCopy := messageError
	r.messageError = &errorCopy
}

func (r *Recorder) UseFinalAnswer(finalAnswer string) {
	if strings.TrimSpace(finalAnswer) == "" || strings.TrimSpace(finalAnswer) == strings.TrimSpace(r.answer.String()) {
		return
	}
	r.answer.Reset()
	r.answer.WriteString(finalAnswer)
}

func (r *Recorder) EnsureFailureAnswer(runErr error) {
	if runErr == nil || strings.TrimSpace(r.answer.String()) != "" {
		return
	}
	if strings.TrimSpace(r.reasoning.String()) == "" && r.timeline.Empty() {
		return
	}
	r.answer.WriteString("运行失败：" + strings.TrimSpace(runErr.Error()))
}

func (r *Recorder) AnswerText() string {
	return r.answer.String()
}

func (r *Recorder) ReasoningText() string {
	return r.reasoning.String()
}

func (r *Recorder) AssistantWasSaved() bool {
	return r.assistantSaved
}
