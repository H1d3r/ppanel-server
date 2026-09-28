package authn

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/auth/usersession"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/account"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/xerr"
)

// ResetPassword sets a new password for the account of an email address,
// proven by a security code sent to it, and signs the account in.
func (s *Service) ResetPassword(ctx context.Context, req *dto.ResetPasswordRequest) (*dto.LoginResponse, error) {
	if err := s.policy.VerifyHuman(ctx, registerpolicy.Reset, req.CfToken); err != nil {
		return nil, err
	}
	if err := s.policy.EnsureMethodEnabled(ctx, identifier.Email); err != nil {
		return nil, err
	}
	email := identifier.CanonicalEmail(req.Email)
	return s.resetPassword(ctx, passwordReset{
		method:     identifier.Email,
		identifier: email,
		codeKey:    verification.EmailCodeKey(auth.Security, email),
		code:       req.Code,
		password:   req.Password,
		device:     req.Identifier,
		loginType:  req.LoginType,
	})
}

// passwordReset is a password reset through one identity.
type passwordReset struct {
	method, identifier string
	codeKey, code      string
	password           string
	device, loginType  string
}

func (s *Service) resetPassword(ctx context.Context, reset passwordReset) (resp *dto.LoginResponse, err error) {
	// The code is checked before the account is looked up, so the reset
	// does not reveal which identifiers have accounts.
	if err := verification.ValidateVerificationCode(ctx, s.deps.Redis, reset.codeKey, reset.code, false); err != nil {
		return nil, xerr.Wrapf(err, xerr.VerifyCodeError, "check reset code")
	}
	userInfo, err := s.findAccount(ctx, reset.method, reset.identifier)
	if err != nil {
		return nil, err
	}
	attempt := account.NewAttempt(s.deps.Store.Log(), reset.method)
	attempt.Identify(userInfo.Id)
	defer func() {
		if err = attempt.Finish(ctx, err); err != nil {
			resp = nil
		}
	}()
	// A code sent to an identifier a deleted or disabled account still holds
	// must not bring the account back.
	if err := account.EnsureActive(userInfo); err != nil {
		return nil, err
	}
	if err := verification.ValidateVerificationCode(ctx, s.deps.Redis, reset.codeKey, reset.code, true); err != nil {
		return nil, xerr.Wrapf(err, xerr.VerifyCodeError, "check reset code")
	}
	if err := s.deps.Store.User().UpdateColumns(ctx, userInfo.Id, password.UserColumns(reset.password)); err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseUpdateError, "update password of user %d", userInfo.Id)
	}
	// A reset usually follows a compromise: end every earlier session before
	// issuing the new one.
	if err := usersession.Revoke(ctx, s.deps.Redis, userInfo.Id); err != nil {
		return nil, xerr.Wrapf(err, xerr.ERROR, "revoke sessions of user %d", userInfo.Id)
	}
	clearLoginFailures(ctx, s.deps.Redis, userInfo.Id)
	return s.signIn(ctx, userInfo.Id, reset.device, reset.loginType)
}
