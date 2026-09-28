package profile

import (
	"context"
	"errors"

	"github.com/perfect-panel/server/internal/auth/identifier"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// UpdateBindEmail binds a new email address, proven by a code sent to it, to
// the calling account, replacing its current one.
func (s *Service) UpdateBindEmail(ctx context.Context, req *dto.UpdateBindEmailRequest) error {
	if err := s.deps.Policy.EnsureMethodEnabled(ctx, identifier.Email); err != nil {
		return err
	}
	domainList, restrict := s.deps.EmailDomains()
	email, err := identifier.ValidateEmail(req.Email, domainList, restrict)
	if err != nil {
		return xerr.Wrapf(err, xerr.InvalidParams, "invalid email")
	}
	req.Email = email
	// The new address becomes a login identifier, so its owner must prove
	// control of it first, as binding a mobile number does.
	cacheKey := verification.EmailCodeKey(auth.Register, email)
	if err := verification.ValidateVerificationCode(ctx, s.deps.Redis, cacheKey, req.Code, false); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}
	u, ok := user.FromContext(ctx)
	if !ok {
		return xerr.Errorf(xerr.InvalidAccess, "no signed-in user")
	}
	method, err := s.deps.UserAuth.FindUserAuthMethodByUserId(ctx, "email", u.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	m, err := s.deps.UserAuth.FindUserAuthMethodByOpenID(ctx, "email", req.Email)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	if m.Id > 0 {
		return xerr.Errorf(xerr.UserExist, "the email is bound to an account")
	}
	if err := verification.ValidateVerificationCode(ctx, s.deps.Redis, cacheKey, req.Code, true); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}
	if method.Id == 0 {
		method = &user.AuthMethods{
			UserId:         u.Id,
			AuthType:       "email",
			AuthIdentifier: req.Email,
			Verified:       true,
		}
		if err := s.deps.UserAuth.InsertUserAuthMethods(ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseInsertError, "bind identity")
		}
	} else {
		method.Verified = true
		method.AuthIdentifier = req.Email
		if err := s.deps.UserAuth.UpdateUserAuthMethods(ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update identity")
		}
	}
	return nil
}
