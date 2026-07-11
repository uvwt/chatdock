package chatdock

import (
	"net/http/httptest"
	"strings"
	"testing"
)

type strictJSONRequest struct {
	Name string `json:"name"`
}

func TestReadJSONAcceptsSingleStrictValue(t *testing.T) {
	request := httptest.NewRequest("POST", "/", strings.NewReader("  {\"name\":\"chatdock\"}  \n"))
	var input strictJSONRequest
	if err := readJSON(request, &input); err != nil {
		t.Fatal(err)
	}
	if input.Name != "chatdock" {
		t.Fatalf("unexpected decoded input: %#v", input)
	}
}

func TestReadJSONRejectsUnknownFieldsAndAdditionalValues(t *testing.T) {
	for name, body := range map[string]string{
		"unknown field":    `{"name":"chatdock","legacy":true}`,
		"additional value": `{"name":"chatdock"} {"name":"second"}`,
	} {
		t.Run(name, func(t *testing.T) {
			request := httptest.NewRequest("POST", "/", strings.NewReader(body))
			var input strictJSONRequest
			if err := readJSON(request, &input); err == nil {
				t.Fatalf("expected strict JSON error for %q", body)
			}
		})
	}
}

func TestReadJSONRejectsOversizedBodyBeforeDecoding(t *testing.T) {
	body := `{"name":"` + strings.Repeat("x", maxJSONRequestBytes) + `"}`
	request := httptest.NewRequest("POST", "/", strings.NewReader(body))
	var input strictJSONRequest
	if err := readJSON(request, &input); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected body limit error, got %v", err)
	}
}
