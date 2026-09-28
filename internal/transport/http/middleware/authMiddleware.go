package middleware

import (
	"context"
	"errors"
	"fmt"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/perfect-panel/server/internal/auth/usersession"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/httpx"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

// SessionAccounts resolves the account and device of a session.
type SessionAccounts interface {
	FindOne(ctx context.Context, id int64) (*user.User, error)
	// FindDeviceForAuth reads the current device state, bypassing caches.
	FindDeviceForAuth(ctx context.Context, id int64) (*user.Device, error)
}

// AccountStore is the part of the application store AccountsFromStore
// adapts; repository.Store satisfies it.
type AccountStore interface {
	User() repository.UserRepo
	UserDevice() repository.UserDeviceRepo
}

// AccountsFromStore resolves session accounts through the store's user and
// device repositories.
func AccountsFromStore(store AccountStore) SessionAccounts {
	return storeAccounts{store: store}
}

type storeAccounts struct{ store AccountStore }

func (a storeAccounts) FindOne(ctx context.Context, id int64) (*user.User, error) {
	return a.store.User().FindOne(ctx, id)
}

func (a storeAccounts) FindDeviceForAuth(ctx context.Context, id int64) (*user.Device, error) {
	return a.store.UserDevice().FindDeviceForAuth(ctx, id)
}

type AuthDeps struct {
	JWT   config.JwtAuth
	Redis *redis.Client
	// Accounts resolves the session's account and device.
	Accounts SessionAccounts
	// Deprecated: set Accounts (AccountsFromStore adapts a store). Store is
	// read only when Accounts is nil.
	Store AccountStore
}

func (deps AuthDeps) accounts() SessionAccounts {
	if deps.Accounts != nil {
		return deps.Accounts
	}
	if deps.Store != nil {
		return AccountsFromStore(deps.Store)
	}
	return nil
}

func AuthMiddleware(deps AuthDeps) app.HandlerFunc {
	return func(ctx context.Context, requestCtx *app.RequestContext) {
		ctx, err := AuthenticateRequest(ctx, deps, string(requestCtx.GetHeader("Authorization")))
		if err != nil {
			httpx.HttpResult(requestCtx, nil, err)
			requestCtx.Abort()
			return
		}
		if metadata, ok := requestmeta.From(ctx); ok && metadata.ActorID > 0 {
			requestCtx.Set(requestActorIDKey, metadata.ActorID)
		}
		requestCtx.Next(ctx)
	}
}

// OptionalAuthMiddleware authenticates a request when it supplies an
// Authorization header, while preserving anonymous access for routes that
// intentionally support guest checkout.  Handlers on those routes must still
// explicitly require an authenticated user before operating on a user-owned
// resource.
func OptionalAuthMiddleware(deps AuthDeps) app.HandlerFunc {
	return func(ctx context.Context, requestCtx *app.RequestContext) {
		token := string(requestCtx.GetHeader("Authorization"))
		if token == "" {
			requestCtx.Next(ctx)
			return
		}

		authenticatedCtx, err := AuthenticateRequest(ctx, deps, token)
		if err != nil {
			httpx.HttpResult(requestCtx, nil, err)
			requestCtx.Abort()
			return
		}
		if metadata, ok := requestmeta.From(authenticatedCtx); ok && metadata.ActorID > 0 {
			requestCtx.Set(requestActorIDKey, metadata.ActorID)
		}
		requestCtx.Next(authenticatedCtx)
	}
}

// AdminGuard admits only administrators. It runs after AuthMiddleware on the
// admin route groups, which the routes package opens only through a helper
// that installs both.
func AdminGuard() app.HandlerFunc {
	return func(ctx context.Context, requestCtx *app.RequestContext) {
		if err := RequireAdmin(ctx); err != nil {
			httpx.HttpResult(requestCtx, nil, err)
			requestCtx.Abort()
			return
		}
		requestCtx.Next(ctx)
	}
}

// RequireAdmin reports whether the request's authenticated user is an
// administrator.
func RequireAdmin(ctx context.Context) error {
	u, ok := user.FromContext(ctx)
	if !ok {
		return fmt.Errorf("admin access needs a signed-in user: %w", xerr.NewErrCode(xerr.InvalidAccess))
	}
	if u.IsAdmin == nil || !*u.IsAdmin {
		return fmt.Errorf("user %d is not an administrator: %w", u.Id, xerr.NewErrCode(xerr.InvalidAccess))
	}
	return nil
}

func AuthenticateRequest(ctx context.Context, deps AuthDeps, token string) (context.Context, error) {
	if token == "" {
		logger.WithContext(ctx).Debug("[AuthMiddleware] Token Empty")
		return ctx, fmt.Errorf("token empty: %w", xerr.NewErrCode(xerr.ErrorTokenEmpty))
	}

	claims, err := usersession.Validate(ctx, deps.Redis, deps.JWT.AccessSecret, token)
	if err != nil {
		logger.WithContext(ctx).Debug("[AuthMiddleware] session refused", logger.Field("error", err.Error()))
		if errors.Is(err, usersession.ErrInvalidToken) {
			return ctx, fmt.Errorf("token invalid: %w", xerr.NewErrCode(xerr.ErrorTokenExpire))
		}
		return ctx, fmt.Errorf("%v: %w", err, xerr.NewErrCode(xerr.InvalidAccess))
	}
	accounts := deps.accounts()
	if accounts == nil {
		return ctx, fmt.Errorf("session accounts unavailable: %w", xerr.NewErrCode(xerr.ERROR))
	}
	if claims.DeviceID != 0 {
		device, err := accounts.FindDeviceForAuth(ctx, claims.DeviceID)
		if err != nil || device == nil || !device.Enabled || device.UserId != claims.UserID {
			return ctx, fmt.Errorf("device is unavailable: %w", xerr.NewErrCode(xerr.InvalidAccess))
		}
	}

	userInfo, err := accounts.FindOne(ctx, claims.UserID)
	if err != nil {
		return ctx, xerr.Wrapf(err, xerr.DatabaseQueryError, "find user %d", claims.UserID)
	}
	if userInfo.DeletedAt.Valid {
		return ctx, fmt.Errorf("user deleted: %w", xerr.NewErrCode(xerr.UserNotExist))
	}
	if userInfo.Enable == nil || !*userInfo.Enable {
		return ctx, fmt.Errorf("user disabled: %w", xerr.NewErrCode(xerr.UserDisabled))
	}

	ctx = context.WithValue(ctx, requestctx.LoginType, claims.LoginType)
	ctx = user.NewContext(ctx, userInfo)
	ctx = context.WithValue(ctx, requestctx.CtxKeySessionID, claims.SessionID)
	ctx = requestmeta.WithActor(ctx, claims.UserID)
	ctx = logger.ContextWithFields(ctx, logger.Field("actor_id", claims.UserID))
	return ctx, nil
}
