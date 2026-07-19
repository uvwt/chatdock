package chatdock

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

func TestBearerTokenMatchesFixedLengthDigests(t *testing.T) {
	if !bearerTokenMatches("token-value", "token-value") {
		t.Fatal("equal bearer tokens did not match")
	}
	for _, received := range []string{"", "short", "token-value-extra"} {
		if bearerTokenMatches(received, "token-value") {
			t.Fatalf("unexpected bearer token match for %q", received)
		}
	}
}
