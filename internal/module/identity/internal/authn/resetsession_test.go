package authn

import (
	"context"
	"testing"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/auth/usersession"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/identitytest"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/xerr"
)

// A reset usually follows a compromise, so sessions issued before it end
// while the one it issues works.
func TestResetPasswordRevokesEarlierSessions(t *testing.T) {
	f := newFixture(t)
	owner := f.account(t, "email", "owner@example.com", "old-password")
	earlier, err := usersession.Issue(context.Background(), f.Redis, testSecret, 3600, usersession.Grant{UserID: owner.Id, LoginType: "email"})
	if err != nil {
		t.Fatal(err)
	}
	f.saveCode(t, verification.EmailCodeKey(auth.Security, "owner@example.com"), "123456")

	resp, err := f.svc.ResetPassword(identitytest.Context(), &dto.ResetPasswordRequest{Email: "Owner@example.com", Code: "123456", Password: "new-password-1"})
	if err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}

	var stored user.User
	if err := f.DB.First(&stored, owner.Id).Error; err != nil {
		t.Fatal(err)
	}
	if !password.MultiPasswordVerify(stored.Algo, stored.Salt, "new-password-1", stored.Password) {
		t.Fatal("the new password was not written")
	}
	if _, err := usersession.Validate(context.Background(), f.Redis, testSecret, earlier); err == nil {
		t.Fatal("a session from before the reset still works")
	}
	if f.sessionUser(t, resp.Token) != owner.Id {
		t.Fatal("the session issued by the reset does not work")
	}
	if audits := f.loginAudits(t, owner.Id); len(audits) != 1 || !audits[0].Success || audits[0].Method != "email" {
		t.Fatalf("login audits = %+v, want the reset's sign-in", audits)
	}
}

// A code sent to an identifier a deleted or disabled account still holds
// must not bring the account back. The refused attempt is audited like a
// refused sign-in.
func TestResetPasswordRejectsDeletedAndDisabledAccounts(t *testing.T) {
	for name, disable := range map[string]string{
		"deleted":  "UPDATE user SET deleted_at = CURRENT_TIMESTAMP",
		"disabled": "UPDATE user SET enable = false",
	} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			owner := f.account(t, "mobile", "+8613800138000", "old-password")
			if err := f.DB.Exec(disable).Error; err != nil {
				t.Fatal(err)
			}
			f.saveCode(t, verification.MobileCodeKey(auth.Security, "+8613800138000"), "123456")

			_, err := f.svc.TelephoneResetPassword(identitytest.Context(), &dto.TelephoneResetPasswordRequest{
				TelephoneAreaCode: "86", Telephone: "13800138000", Code: "123456", Password: "new-password-1",
			})
			if code := xerr.CodeOf(err); err == nil || (code != xerr.UserNotExist && code != xerr.UserDisabled) {
				t.Fatalf("error = %v, want the account refused", err)
			}
			var stored user.User
			if err := f.DB.Unscoped().First(&stored, owner.Id).Error; err != nil {
				t.Fatal(err)
			}
			if !password.MultiPasswordVerify(stored.Algo, stored.Salt, "old-password", stored.Password) {
				t.Fatal("the account's password was written")
			}
			if audits := f.loginAudits(t, owner.Id); len(audits) != 1 || audits[0].Success {
				t.Fatalf("login audits = %+v, want one failure", audits)
			}
		})
	}
}

func TestResetPasswordNeedsTheCodeBeforeLookingTheAccountUp(t *testing.T) {
	f := newFixture(t)
	owner := f.account(t, "email", "owner@example.com", "old-password")
	_, err := f.svc.ResetPassword(identitytest.Context(), &dto.ResetPasswordRequest{Email: "owner@example.com", Code: "000000", Password: "new-password-1"})
	assertCode(t, err, xerr.VerifyCodeError)
	_, err = f.svc.ResetPassword(identitytest.Context(), &dto.ResetPasswordRequest{Email: "nobody@example.com", Code: "000000", Password: "new-password-1"})
	assertCode(t, err, xerr.VerifyCodeError)
	if audits := f.loginAudits(t, owner.Id); len(audits) != 0 {
		t.Fatalf("login audits = %+v, want none before the code is proven", audits)
	}
}
