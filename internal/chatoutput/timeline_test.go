package chatoutput

import "testing"

func TestMessagePartsRecorderPersistsModelFallbackInTimeline(t *testing.T) {
	recorder := &timelineRecorder{}
	recorder.Record("model_fallback", map[string]any{
		"from_provider_id": "primary",
		"from_model":       "primary-model",
		"to_provider_id":   "backup",
		"to_model":         "backup-model",
		"reason":           "上游不可用",
	})

	if len(recorder.Events()) != 1 || len(recorder.Parts()) != 1 {
		t.Fatalf("fallback event was not added to the assistant timeline: events=%#v parts=%#v", recorder.Events(), recorder.Parts())
	}
	event := recorder.Events()[0]
	if event.Text != "切换备用模型" || event.Meta != "backup · backup-model" || event.Details["event"] != "model_fallback" {
		t.Fatalf("unexpected persisted fallback event: %#v", event)
	}
	if recorder.Parts()[0].Event == nil || recorder.Parts()[0].Event.CallKey != event.CallKey {
		t.Fatalf("fallback timeline part does not reference the persisted event: %#v", recorder.Parts()[0])
	}
}
