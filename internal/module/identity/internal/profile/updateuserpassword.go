package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/auth/password"
	"github.com/perfect-panel/server/internal/auth/usersession"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/xerr"
)

// UpdateUserPassword sets the calling account's password and ends its
// sessions.
func (s *Service) UpdateUserPassword(ctx context.Context, req *dto.UpdateUserPasswordRequest) error {
	userInfo, ok := user.FromContext(ctx)
	if !ok {
		return xerr.Errorf(xerr.InvalidAccess, "Invalid Access")
	}
	// A session alone must not be enough to take the account over for good:
	// changing an existing password proves the current one.
	if userInfo.Password != "" && !password.MultiPasswordVerify(userInfo.Algo, userInfo.Salt, req.OldPassword, userInfo.Password) {
		return xerr.Errorf(xerr.UserPasswordError, "current password is incorrect")
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
