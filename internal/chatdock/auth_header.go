package chatdock

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"
)

func bearerTokenFromAuthorization(value string) string {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func constantTimeStringEqual(received string, expected string) bool {
	receivedHash := sha256.Sum256([]byte(received))
	expectedHash := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(receivedHash[:], expectedHash[:]) == 1
}
