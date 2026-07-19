package llm

import (
	"strings"
	"testing"

	"chatdock/internal/chatdock/model"
)

func imageOnlyHistoryMessage() model.Message {
	return model.Message{
		Role: "user",
		ModelAttachments: []model.AttachmentRecord{{
			Attachment: model.Attachment{ID: "image-1", Name: "photo.png", MIMEType: "image/png"},
			ModelURL:   "https://chatdock.example/api/model-images/image-1?sig=test",
		}},
	}
}

func TestBuildChatMessagesAnyKeepsImageOnlyUserMessage(t *testing.T) {
	messages := BuildChatMessagesAny(model.ModelConfig{}, []model.Message{imageOnlyHistoryMessage()})
	if len(messages) != 1 || messages[0]["role"] != "user" {
		t.Fatalf("image-only messages = %#v", messages)
	}
	blocks, ok := messages[0]["content"].([]map[string]any)
	if !ok || len(blocks) != 2 {
		t.Fatalf("image-only content blocks = %#v", messages[0]["content"])
	}
	if blocks[0]["type"] != "text" || blocks[1]["type"] != "image_url" {
		t.Fatalf("image-only block order = %#v", blocks)
	}
}

func TestTextOnlyBuilderSkipsImageOnlyBlankMessage(t *testing.T) {
	messages := BuildChatMessages(model.ModelConfig{}, []model.Message{imageOnlyHistoryMessage()})
	if len(messages) != 0 {
		t.Fatalf("text-only messages should not contain a blank image placeholder: %#v", messages)
	}
}

func TestEarlierContextSummaryDescribesImageOnlyMessage(t *testing.T) {
	summary := summarizeEarlierContext([]model.Message{imageOnlyHistoryMessage()})
	if !strings.Contains(summary, "用户：[图片附件]") {
		t.Fatalf("image-only summary = %q", summary)
	}
}

func TestContextPlanUsesBoundedCustomCapacity(t *testing.T) {
	count, summarize := ContextPlan(model.ModelConfig{ContextMode: model.ContextModeCustom, MaxContextMessages: model.MaxContextMessagesLimit + 1})
	if count != model.MaxContextMessagesLimit || summarize {
		t.Fatalf("custom context plan = (%d, %v)", count, summarize)
	}
}

func TestValidChatHistoryStillDropsEmptyNonImageMessages(t *testing.T) {
	history := []model.Message{
		{Role: "user"},
		{Role: "assistant", Content: "   "},
		imageOnlyHistoryMessage(),
	}
	valid := validChatHistory(history)
	if len(valid) != 1 || !hasModelImageAttachment(valid[0].ModelAttachments) {
		t.Fatalf("valid history = %#v", valid)
	}
}
