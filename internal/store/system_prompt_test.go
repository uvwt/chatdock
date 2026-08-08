package store

import "testing"

func TestBuildFinalSystemPromptOrder(t *testing.T) {
	got := BuildFinalSystemPrompt("全局提示", "项目提示")
	want := "全局提示\n\n项目提示"
	if got != want {
		t.Fatalf("prompt = %q, want %q", got, want)
	}
}

func TestBuildFinalSystemPromptOmitsEmptyParts(t *testing.T) {
	tests := []struct {
		name    string
		global  string
		project string
		want    string
	}{
		{name: "global only", global: " 全局 ", want: "全局"},
		{name: "project only", project: " 项目 ", want: "项目"},
		{name: "empty", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := BuildFinalSystemPrompt(tt.global, tt.project); got != tt.want {
				t.Fatalf("prompt = %q, want %q", got, tt.want)
			}
		})
	}
}
