package chatdock

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRejectPrivateHostHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := rejectPrivateHost(ctx, "example.invalid")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}

func TestPublicImageTransportRejectsPrivateDialTarget(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
	}))
	defer server.Close()

	_, err := probeImageURL(context.Background(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "private or localhost") {
		t.Fatalf("expected private target rejection, got %v", err)
	}
	if requestCount != 0 {
		t.Fatalf("private server received %d requests", requestCount)
	}
}

func TestPublicImageTransportHonorsCanceledDialContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	transport := publicImageTransport()
	defer transport.CloseIdleConnections()
	_, err := transport.DialContext(ctx, "tcp", "example.invalid:80")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled dial, got %v", err)
	}
}

func TestPrivateImageIPClassification(t *testing.T) {
	tests := map[string]bool{
		"127.0.0.1":            true,
		"::ffff:127.0.0.1":     true,
		"10.0.0.1":             true,
		"100.64.0.1":           true,
		"169.254.1.1":          true,
		"192.0.2.1":            true,
		"198.18.0.1":           true,
		"203.0.113.1":          true,
		"::1":                  true,
		"fc00::1":              true,
		"fe80::1":              true,
		"2001:db8::1":          true,
		"8.8.8.8":              false,
		"2606:4700:4700::1111": false,
	}
	for raw, wantBlocked := range tests {
		t.Run(raw, func(t *testing.T) {
			if got := isPrivateOrLocalIP(net.ParseIP(raw)); got != wantBlocked {
				t.Fatalf("isPrivateOrLocalIP(%s) = %v, want %v", raw, got, wantBlocked)
			}
		})
	}
}
