package chatdock

import "testing"

func TestMessagePartsRecorderPersistsModelFallbackInTimeline(t *testing.T) {
	recorder := &messagePartsRecorder{}
	recorder.record("model_fallback", map[string]any{
		"from_provider_id": "primary",
		"from_model":       "primary-model",
		"to_provider_id":   "backup",
		"to_model":         "backup-model",
		"reason":           "上游不可用",
	})

	if len(recorder.events) != 1 || len(recorder.parts) != 1 {
		t.Fatalf("fallback event was not added to the assistant timeline: events=%#v parts=%#v", recorder.events, recorder.parts)
	}
	event := recorder.events[0]
	if event.Text != "切换备用模型" || event.Meta != "backup · backup-model" || event.Details["event"] != "model_fallback" {
		t.Fatalf("unexpected persisted fallback event: %#v", event)
	}
	if recorder.parts[0].Event == nil || recorder.parts[0].Event.CallKey != event.CallKey {
		t.Fatalf("fallback timeline part does not reference the persisted event: %#v", recorder.parts[0])
	}
}
