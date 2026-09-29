package profile

import (
	"context"
	"errors"

	"github.com/perfect-panel/server/internal/auth/identifier"
	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/auth/usersession"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/verification"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// UpdateUserPassword sets the calling account's password and ends its
// sessions. A session alone must not be enough to take the account over for
// good: changing an existing password proves the current one, and setting
// the first password of an account that has an email or phone number bound
// proves that address with a security code sent to it, so a stolen session
// cannot bootstrap a password and then use it to move the account. An
// account with neither a password nor a bound address (OAuth or device only)
// has nothing else to prove and sets its password with the session.
func (s *Service) UpdateUserPassword(ctx context.Context, req *dto.UpdateUserPasswordRequest) error {
	userInfo, ok := user.FromContext(ctx)
	if !ok {
		return xerr.Errorf(xerr.InvalidAccess, "Invalid Access")
	}
	var proofKey string
	if userInfo.Password != "" {
		// The current password, under the same guess limit as sign-in.
		if err := s.checkPassword(ctx, userInfo, req.OldPassword); err != nil {
			return err
		}
	} else {
		var err error
		if proofKey, err = s.proveBoundAddress(ctx, userInfo.Id, req.CurrentCode); err != nil {
			return err
		}
	}
	if proofKey != "" {
		if err := verification.ValidateVerificationCode(ctx, s.deps.Redis, proofKey, req.CurrentCode, true); err != nil {
			return xerr.Wrapf(err, xerr.VerifyCodeError, "check the bound address code")
		}
	}
	// The new hash always uses the current algorithm; a migrated user would
	// otherwise keep verifying it with the old legacy algorithm.
	if err := s.deps.Users.UpdateColumns(ctx, userInfo.Id, password.UserColumns(req.Password)); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseUpdateError, "update password of user %d", userInfo.Id)
	}
	// Every session from before the change ends, including a stolen one.
	if err := usersession.Revoke(ctx, s.deps.Redis, userInfo.Id); err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "revoke sessions of user %d", userInfo.Id)
	}
	return nil
}

// proveBoundAddress checks code against the security codes sent to the
// account's bound email and phone number and returns the key of the code it
// matches, for the caller to spend once the change goes through. An account
// with neither bound has nothing to prove and gets no key. Wrong guesses
// count against each code's own guess limit, which deletes the code.
func (s *Service) proveBoundAddress(ctx context.Context, userID int64, code string) (string, error) {
	keys, err := s.boundAddressKeys(ctx, userID)
	if err != nil || len(keys) == 0 {
		return "", err
	}
	if code == "" {
		return "", xerr.Errorf(xerr.InvalidParams, "the security code sent to the bound email or phone number is required to set the first password")
	}
	var lastErr error
	for _, key := range keys {
		if lastErr = verification.ValidateVerificationCode(ctx, s.deps.Redis, key, code, false); lastErr == nil {
			return key, nil
		}
	}
	return "", xerr.Wrapf(lastErr, xerr.VerifyCodeError, "check the bound address code")
}

// boundAddressKeys returns the keys of the security codes sent to the
// account's bound email and phone number, in that order; none for an
// account with neither.
func (s *Service) boundAddressKeys(ctx context.Context, userID int64) ([]string, error) {
	var keys []string
	for _, authType := range []string{identifier.Email, identifier.Mobile} {
		method, err := s.deps.UserAuth.FindUserAuthMethodByUserId(ctx, authType, userID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find %s identity", authType)
		}
		keys = append(keys, securityCodeKey(authType, method.AuthIdentifier))
	}
	return keys, nil
}
