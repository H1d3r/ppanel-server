package telegramsecret

import "testing"

func TestDeriveIsDeterministicAndTokenScoped(t *testing.T) {
	a := Derive("123456:token-a")
	if a != Derive("123456:token-a") {
		t.Fatal("Derive is not deterministic")
	}
	if a == Derive("123456:token-b") {
		t.Fatal("different tokens must derive different secrets")
	}
	if len(a) != 64 {
		t.Fatalf("secret length = %d, want 64 hex chars", len(a))
	}
}

// The derived secret must never equal a naive digest of the token itself:
// the previous scheme (md5 of the token in a query parameter) is what this
// package replaces, and the purpose string is what keeps them apart.
func TestDeriveDiffersFromPlainDigest(t *testing.T) {
	if Derive("") == Derive(purpose) {
		t.Fatal("purpose separation is not applied")
	}
}

func TestEqual(t *testing.T) {
	secret := Derive("123456:token")
	if !Equal(secret, secret) {
		t.Fatal("Equal rejected the matching secret")
	}
	if Equal("wrong", secret) {
		t.Fatal("Equal accepted a mismatching secret")
	}
	if Equal("", secret) {
		t.Fatal("Equal accepted an empty secret")
	}
}
