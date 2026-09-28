package verifycode

import (
	"context"
	"errors"
	"testing"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/pkg/xerr"
)

type fakeSmsCodePolicy struct {
	method string
	err    error
}

func (p *fakeSmsCodePolicy) EnsureRegistrationOpen(_ context.Context, method string) error {
	p.method = method
	return p.err
}

func (p *fakeSmsCodePolicy) EnsureMethodEnabled(context.Context, string) error { return nil }

func TestSendSmsCodeUsesInjectedRegistrationPolicy(t *testing.T) {
	blocked := errors.New("registration disabled")
	policy := &fakeSmsCodePolicy{err: blocked}
	logic := NewSendSmsCodeLogic(context.Background(), SendSmsCodeDependencies{Policy: policy})

	_, err := logic.SendSmsCode(&dto.SendSmsCodeRequest{Type: uint8(auth.Register)})
	if !errors.Is(err, blocked) {
		t.Fatalf("SendSmsCode error = %v, want registration policy error", err)
	}
	if policy.method != identifier.Mobile {
		t.Fatalf("registration method = %q, want %q", policy.method, identifier.Mobile)
	}
}

// With the whitelist on, a code is never sent (and never costs the operator)
// outside the configured area codes.
func TestSendSmsCodeEnforcesAreaCodeWhitelist(t *testing.T) {
	logic := NewSendSmsCodeLogic(context.Background(), SendSmsCodeDependencies{
		Policy: &fakeSmsCodePolicy{},
		Config: SmsCodeConfig{WhitelistEnabled: true, Whitelist: []string{"86", "+852"}},
	})

	_, err := logic.SendSmsCode(&dto.SendSmsCodeRequest{Type: 1, TelephoneAreaCode: "882", Telephone: "1234567"})
	var codeErr *xerr.CodeError
	if !errors.As(err, &codeErr) || codeErr.GetErrCode() != xerr.TelephoneError {
		t.Fatalf("SendSmsCode() error = %v, want TelephoneError for a non-whitelisted area code", err)
	}
}

func TestAreaCodeAllowed(t *testing.T) {
	whitelist := []string{"86", "+852", " 1 "}
	for areaCode, want := range map[string]bool{"86": true, "+86": true, "852": true, "1": true, "+1": true, "882": false, "": false, "8": false} {
		if got := areaCodeAllowed(areaCode, whitelist); got != want {
			t.Errorf("areaCodeAllowed(%q) = %v, want %v", areaCode, got, want)
		}
	}
}
