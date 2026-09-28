package profile

import (
	"context"
	"errors"
	"fmt"

	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/pkg/xerr"
	"gorm.io/gorm"
)

// BindOAuthCallback completes binding the req.Method identity the provider
// vouches for to the calling account. An account holds at most one identity
// per method: binding the identity it already holds succeeds again, which a
// client retrying a timed-out callback relies on, and another account's
// identity or a second one of the method is refused.
func (s *Service) BindOAuthCallback(ctx context.Context, req *dto.BindOAuthCallbackRequest) error {
	if err := s.deps.Policy.EnsureMethodEnabled(ctx, req.Method); err != nil {
		return err
	}
	current, ok := user.FromContext(ctx)
	if !ok {
		return fmt.Errorf("no signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	fields, ok := req.Callback.(map[string]any)
	if !ok {
		return fmt.Errorf("OAuth callback must be an object: %w", xerr.NewErrCode(xerr.InvalidParams))
	}
	identity, err := s.deps.OAuth.Identify(ctx, req.Method, fields)
	if err != nil {
		return err
	}

	holder, err := s.deps.UserAuth.FindUserAuthMethodByOpenID(ctx, req.Method, identity.Subject)
	switch {
	case err == nil && holder.UserId == current.Id:
		return nil
	case err == nil:
		return fmt.Errorf("the %s identity belongs to another account: %w", req.Method, xerr.NewErrCode(xerr.UserExist))
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find %s identity", req.Method)
	}
	if _, err := s.deps.UserAuth.FindUserAuthMethodByPlatform(ctx, current.Id, req.Method); err == nil {
		return fmt.Errorf("the account already has a %s identity: %w", req.Method, xerr.NewErrCode(xerr.UserExist))
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return xerr.Wrapf(err, xerr.DatabaseQueryError, "find the account's %s identity", req.Method)
	}

	if err := s.deps.UserAuth.InsertUserAuthMethods(ctx, &user.AuthMethods{
		UserId:         current.Id,
		AuthType:       req.Method,
		AuthIdentifier: identity.Subject,
		Verified:       true,
	}); err != nil {
		return xerr.Wrapf(err, xerr.DatabaseInsertError, "bind %s identity", req.Method)
	}
	if err := s.deps.UserCache.ClearUserCache(ctx, current); err != nil {
		return xerr.Wrapf(err, xerr.ERROR, "clear user cache")
	}
	return nil
}
