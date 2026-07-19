package chatdock

import (
	"context"
	"strings"
	"testing"
)

func TestValidatePublicHTTPImageURLRejectsEmbeddedCredentialsBeforeDNS(t *testing.T) {
	_, err := validatePublicHTTPImageURL(context.Background(), "https://user:secret@example.invalid/image.png")
	if err == nil || !strings.Contains(err.Error(), "credentials are not allowed") {
		t.Fatalf("userinfo URL error = %v", err)
	}
}

func TestValidateImageResponseMIMEUsesContentAndRejectsMismatch(t *testing.T) {
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13, 'I', 'H', 'D', 'R'}
	jpeg := []byte{0xff, 0xd8, 0xff, 0xe0, 0, 16, 'J', 'F', 'I', 'F', 0}

	for _, tc := range []struct {
		declared string
		body     []byte
		want     string
	}{
		{declared: "", body: png, want: "image/png"},
		{declared: "application/octet-stream", body: png, want: "image/png"},
		{declared: "image/jpg", body: jpeg, want: "image/jpeg"},
	} {
		got, err := validateImageResponseMIME(tc.declared, tc.body)
		if err != nil || got != tc.want {
			t.Fatalf("declared=%q MIME=%q error=%v, want %q", tc.declared, got, err, tc.want)
		}
	}

	if _, err := validateImageResponseMIME("image/png", []byte("<html>not an image</html>")); err == nil || !strings.Contains(err.Error(), "not a supported image") {
		t.Fatalf("HTML masquerading as PNG error = %v", err)
	}
	if _, err := validateImageResponseMIME("image/jpeg", png); err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("declared/detected mismatch error = %v", err)
	}
}
