package httpapi

import "testing"

func TestBearerTokenFromAuthorizationParsesSchemeStrictly(t *testing.T) {
	cases := map[string]string{
		"Bearer token-value":       "token-value",
		"bearer token-value":       "token-value",
		"  BEARER   token-value  ": "token-value",
		"":                         "",
		"token-value":              "",
		"Basic token-value":        "",
		"Bearer":                   "",
		"Bearer one two":           "",
	}
	for header, want := range cases {
		if got := bearerTokenFromAuthorization(header); got != want {
			t.Fatalf("header %q token = %q, want %q", header, got, want)
		}
	}
}

func TestConstantTimeStringEqualUsesFixedLengthDigests(t *testing.T) {
	if !constantTimeStringEqual("token-value", "token-value") {
		t.Fatal("equal strings did not match")
	}
	for _, received := range []string{"", "short", "token-value-extra"} {
		if constantTimeStringEqual(received, "token-value") {
			t.Fatalf("unexpected string match for %q", received)
		}
	}
}
