package usersub

import (
	"regexp"
	"testing"
)

func TestNewTokenKeepsStoredFormatAndIsRandom(t *testing.T) {
	format := regexp.MustCompile(`^[0-9a-f]{32}$`)
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		token := NewToken()
		if !format.MatchString(token) {
			t.Fatalf("NewToken() = %q, want 32 lowercase hex characters", token)
		}
		if _, dup := seen[token]; dup {
			t.Fatalf("NewToken() repeated %q", token)
		}
		seen[token] = struct{}{}
	}
}
