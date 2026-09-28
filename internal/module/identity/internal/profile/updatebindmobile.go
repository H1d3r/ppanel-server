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

// UpdateBindMobile binds a new phone number, proven by a code sent to it, to
// the calling account, replacing its current one.
func (s *Service) UpdateBindMobile(ctx context.Context, req *dto.UpdateBindMobileRequest) error {
	if err := s.deps.Policy.EnsureMethodEnabled(ctx, identifier.Mobile); err != nil {
		return err
	}
	u, ok := user.FromContext(ctx)
	if !ok {
		return xerr.Errorf(xerr.InvalidAccess, "no signed-in user")
	}
	phoneNumber, err := identifier.FormatToE164(req.AreaCode, req.Mobile)
	if err != nil {
		return xerr.Wrapf(err, xerr.TelephoneError, "invalid phone number")
	}
	cacheKey := verification.MobileCodeKey(auth.Register, phoneNumber)
	if err := verification.ValidateVerificationCode(ctx, s.deps.Redis, cacheKey, req.Code, false); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}

	m, err := s.deps.UserAuth.FindUserAuthMethodByOpenID(ctx, "mobile", phoneNumber)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	if m.Id > 0 {
		return xerr.Errorf(xerr.UserExist, "the mobile number is bound to an account")
	}

	method, err := s.deps.UserAuth.FindUserAuthMethodByUserId(ctx, "mobile", u.Id)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find identity")
	}
	if err := verification.ValidateVerificationCode(ctx, s.deps.Redis, cacheKey, req.Code, true); err != nil {
		return xerr.Wrapf(err, xerr.VerifyCodeError, "check verification code")
	}
	// err is still the lookup's: an account without a mobile binding gets
	// a new one.
	if errors.Is(err, gorm.ErrRecordNotFound) {
		method = &user.AuthMethods{
			UserId:         u.Id,
			AuthType:       "mobile",
			AuthIdentifier: phoneNumber,
			Verified:       true,
		}
		if err := s.deps.UserAuth.InsertUserAuthMethods(ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseInsertError, "bind identity")
		}
	} else {
		method.Verified = true
		method.AuthIdentifier = phoneNumber
		if err := s.deps.UserAuth.UpdateUserAuthMethods(ctx, method); err != nil {
			return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update identity")
		}
	}
	return nil
}
