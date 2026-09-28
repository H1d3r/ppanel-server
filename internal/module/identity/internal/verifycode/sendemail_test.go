package verifycode

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/perfect-panel/server/internal/infra/taskqueue"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/identitytest"
	"github.com/perfect-panel/server/pkg/xerr"
)

func TestEmailCodeIsSentAndChecked(t *testing.T) {
	f := newCodeFixture(t)
	if _, err := f.svc.SendEmailCode(identitytest.Context(), &dto.SendCodeRequest{Email: "New@Example.com", Type: uint8(auth.Register)}); err != nil {
		t.Fatalf("SendEmailCode() error = %v", err)
	}
	if len(f.queue.tasks) != 1 || f.queue.tasks[0].Type() != taskqueue.ForthwithSendEmail {
		t.Fatalf("tasks = %+v", f.queue.tasks)
	}
	var payload taskqueue.SendEmailPayload
	if err := json.Unmarshal(f.queue.tasks[0].Payload(), &payload); err != nil {
		t.Fatal(err)
	}
	code, _ := payload.Content["Code"].(string)
	if payload.Email != "new@example.com" || code == "" || payload.UserAgent != identitytest.UserAgent {
		t.Fatalf("payload = %+v", payload)
	}
	resp, err := f.svc.CheckVerificationCode(context.Background(), &dto.CheckVerificationCodeRequest{
		Method: "email", Account: "NEW@example.com", Code: code, Type: uint8(auth.Register),
	})
	if err != nil || !resp.Status {
		t.Fatalf("CheckVerificationCode = %+v, %v; want the code found", resp, err)
	}
}

func TestEmailCodePurposeFollowsTheBinding(t *testing.T) {
	f := newCodeFixture(t)
	f.bind(t, "email", "owner@example.com")

	_, err := f.svc.SendEmailCode(identitytest.Context(), &dto.SendCodeRequest{Email: "owner@example.com", Type: uint8(auth.Register)})
	assertCode(t, err, xerr.UserExist)
	_, err = f.svc.SendEmailCode(identitytest.Context(), &dto.SendCodeRequest{Email: "nobody@example.com", Type: uint8(auth.Security)})
	assertCode(t, err, xerr.UserNotExist)
	f.policy.StopRegister = true
	_, err = f.svc.SendEmailCode(identitytest.Context(), &dto.SendCodeRequest{Email: "new@example.com", Type: uint8(auth.Register)})
	assertCode(t, err, xerr.StopRegister)
	_, err = f.svc.SendEmailCode(identitytest.Context(), &dto.SendCodeRequest{Email: "not-an-email", Type: uint8(auth.Security)})
	assertCode(t, err, xerr.InvalidParams)
}
