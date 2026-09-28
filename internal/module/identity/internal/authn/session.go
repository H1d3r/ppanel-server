package authn

import (
	"context"
	"errors"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/module/identity/internal/account"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// signIn ends a successful sign-in: it binds the requested device, if any,
// and issues the session.
func (s *Service) signIn(ctx context.Context, userID int64, deviceIdentifier, loginType string) (*dto.LoginResponse, error) {
	device, err := s.bindLoginDevice(ctx, deviceIdentifier, userID)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.InvalidAccess, "bind device")
	}
	token, err := account.IssueSession(ctx, s.deps.Redis, s.deps.Config().sessions(), userID, loginType, device)
	if err != nil {
		return nil, err
	}
	return &dto.LoginResponse{Token: token}, nil
}

// bindLoginDevice binds the device a sign-in names to the account; a sign-in
// naming none binds nothing.
func (s *Service) bindLoginDevice(ctx context.Context, identifier string, userID int64) (*user.Device, error) {
	if identifier == "" {
		return nil, nil
	}
	device, err := s.BindDeviceToUser(ctx, identifier, userID)
	if err != nil {
		return nil, err
	}
	if device == nil || device.Id <= 0 || device.UserId != userID || !device.Enabled {
		return nil, fmt.Errorf("invalid device binding: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	return device, nil
}

// findAccount returns the account an identity of authType signs in to.
func (s *Service) findAccount(ctx context.Context, authType, identifier string) (*user.User, error) {
	method, err := s.deps.Store.UserAuth().FindUserAuthMethodByOpenID(ctx, authType, identifier)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no account has this %s identity: %w", authType, xerr.NewErrCode(xerr.UserNotExist))
		}
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find %s identity", authType)
	}
	userInfo, err := s.deps.Store.User().FindOne(ctx, method.UserId)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("user %d of the %s identity does not exist: %w", method.UserId, authType, xerr.NewErrCode(xerr.UserNotExist))
		}
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find user %d", method.UserId)
	}
	return userInfo, nil
}
