package identifier

import "testing"

func TestCanonicalEmail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "mixed case and whitespace", in: "  Alice.Example@Example.COM  ", want: "alice.example@example.com"},
		{name: "already canonical", in: "alice@example.com", want: "alice@example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalEmail(tt.in); got != tt.want {
				t.Fatalf("CanonicalEmail(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if got := CanonicalEmail(CanonicalEmail(tt.in)); got != tt.want {
				t.Fatalf("CanonicalEmail must be idempotent: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanonicalIdentifierOnlyChangesEmail(t *testing.T) {
	tests := []struct {
		name       string
		authType   string
		identifier string
		want       string
	}{
		{name: "email", authType: Email, identifier: " Alice@Example.COM ", want: "alice@example.com"},
		{name: "mobile", authType: Mobile, identifier: " +1 555 0100 ", want: " +1 555 0100 "},
		{name: "device", authType: Device, identifier: " Device-ABC ", want: " Device-ABC "},
		{name: "oauth", authType: "google", identifier: " OAuth-Subject ", want: " OAuth-Subject "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CanonicalIdentifier(tt.authType, tt.identifier); got != tt.want {
				t.Fatalf("CanonicalIdentifier(%q, %q) = %q, want %q", tt.authType, tt.identifier, got, tt.want)
			}
		})
	}
}

func TestEmailMailboxKey(t *testing.T) {
	for email, want := range map[string]string{
		"John.Smith+promo@Gmail.com":   "johnsmith@gmail.com",
		"j.o.h.n.smith@googlemail.com": "johnsmith@gmail.com",
		"john.smith+a+b@example.com":   "john.smith@example.com",
		"john.smith@example.com":       "john.smith@example.com",
		"+tag@example.com":             "+tag@example.com",
		"not-an-email":                 "not-an-email",
	} {
		if got := EmailMailboxKey(email); got != want {
			t.Errorf("EmailMailboxKey(%q) = %q, want %q", email, got, want)
		}
	}
}
