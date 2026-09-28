// Package oauthflow runs the provider round trip OAuth sign-in and account
// binding share: the authorization URL with its state, and the callback that
// redeems the state and turns into the identity the provider vouches for.
// What happens with that identity (signing in, registering, binding) is up
// to the caller.
package oauthflow

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/perfect-panel/server/internal/module/identity/entity/auth"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthprovider"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthstate"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/redis/go-redis/v9"
)

// providerTimeout bounds the provider requests of one callback.
const providerTimeout = 10 * time.Second

// replayGrace lets a client re-submit a single-use callback after a
// timed-out exchange; beyond it, a repeat is a replay.
const replayGrace = 60 * time.Second

// AuthConfigs loads a method's stored configuration.
type AuthConfigs interface {
	FindOneByMethod(ctx context.Context, method string) (*auth.Auth, error)
}

// Deps declares the flow's collaborators.
type Deps struct {
	Auths AuthConfigs
	Redis *redis.Client
	// Providers lists the methods; oauthprovider.Default when nil.
	Providers oauthprovider.Registry
	// SiteHost snapshots the configured site host redirects are pinned to;
	// optional.
	SiteHost func() string
}

// Flow runs OAuth round trips.
type Flow struct {
	deps Deps
}

func New(deps Deps) *Flow {
	if deps.Providers == nil {
		deps.Providers = oauthprovider.Default()
	}
	return &Flow{deps: deps}
}

// AuthURL returns the URL starting a sign-in through method that comes back
// to redirect.
func (f *Flow) AuthURL(ctx context.Context, method, redirect string) (string, error) {
	spec, provider, err := f.provider(ctx, method)
	if err != nil {
		return "", err
	}
	if spec.PinRedirect {
		if err := oauthstate.ValidateRedirect(redirect, f.siteHost()); err != nil {
			return "", xerr.Wrapf(err, xerr.InvalidParams, "invalid %s redirect", method)
		}
	}
	state := ""
	if spec.State {
		if state, err = oauthstate.Issue(ctx, f.deps.Redis, method, redirect); err != nil {
			return "", xerr.Wrapf(err, xerr.ERROR, "store %s state", method)
		}
	}
	uri, err := provider.AuthURL(redirect, state)
	if err != nil {
		return "", codeFor(ctx, method, err)
	}
	return uri, nil
}

// Identify completes the round trip of a callback through method and returns
// the identity the provider vouches for. A state-based callback redeems its
// state; a single-use one (Telegram) is redeemed here too.
func (f *Flow) Identify(ctx context.Context, method string, fields map[string]any) (*oauthprovider.Identity, error) {
	spec, ok := f.deps.Providers.Lookup(method)
	if !ok {
		return nil, notSupported(method)
	}
	callback := oauthprovider.Callback{Fields: fields}
	if spec.State {
		code, _ := fields["code"].(string)
		state, _ := fields["state"].(string)
		if strings.TrimSpace(state) == "" || strings.TrimSpace(code) == "" {
			return nil, xerr.Errorf(xerr.InvalidParams, "%s callback needs a code and a state", method)
		}
		redirect, err := oauthstate.Consume(ctx, f.deps.Redis, method, state)
		if err != nil {
			if errors.Is(err, oauthstate.ErrUnknown) {
				return nil, xerr.Wrapf(err, xerr.OAuthStateInvalid, "redeem %s state", method)
			}
			return nil, xerr.Wrapf(err, xerr.ERROR, "redeem %s state", method)
		}
		callback.Code, callback.Redirect = code, redirect
	}
	_, provider, err := f.provider(ctx, method)
	if err != nil {
		return nil, err
	}
	providerCtx, cancel := context.WithTimeout(ctx, providerTimeout)
	defer cancel()
	identity, err := provider.Identify(providerCtx, callback)
	if err != nil {
		return nil, codeFor(ctx, method, err)
	}
	if identity.Subject == "" {
		return nil, xerr.Errorf(xerr.OAuthProviderError, "%s returned no user id", method)
	}
	if identity.ReplayKey != "" {
		if err := f.redeem(ctx, method, identity.ReplayKey); err != nil {
			return nil, err
		}
	}
	return identity, nil
}

// redeem enforces single use of a callback without a state. A Redis outage
// must not lock users out, so it degrades to the callback's own signature
// and freshness checks.
func (f *Flow) redeem(ctx context.Context, method, key string) error {
	allowed, err := oauthstate.ClaimSingleUse(ctx, f.deps.Redis, key, timeutil.Now(), replayGrace, oauthprovider.CallbackLifetime)
	if err != nil {
		logger.WithContext(ctx).Errorw("oauth callback replay check unavailable",
			logger.Field("method", method), logger.Field("error", err.Error()))
		return nil
	}
	if !allowed {
		return xerr.Errorf(xerr.OAuthCallbackReplayed, "%s callback has already been used", method)
	}
	return nil
}

func (f *Flow) provider(ctx context.Context, method string) (oauthprovider.Method, oauthprovider.Provider, error) {
	spec, ok := f.deps.Providers.Lookup(method)
	if !ok {
		return spec, nil, notSupported(method)
	}
	stored, err := f.deps.Auths.FindOneByMethod(ctx, method)
	if err != nil {
		return spec, nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "load %s configuration", method)
	}
	provider, err := spec.New(stored.Config)
	if err != nil {
		return spec, nil, codeFor(ctx, method, err)
	}
	return spec, provider, nil
}

func (f *Flow) siteHost() string {
	if f.deps.SiteHost == nil {
		return ""
	}
	return f.deps.SiteHost()
}

func notSupported(method string) error {
	return xerr.Errorf(xerr.AuthenticatorNotSupportedError, "oauth method %q is not supported", method)
}

// codeFor attaches the client code of a provider failure. Failures that are
// not the client's are logged here: their codes are specific, so the access
// log does not record their cause.
func codeFor(ctx context.Context, method string, err error) error {
	code := xerr.OAuthProviderError
	switch {
	case errors.Is(err, oauthprovider.ErrIncompleteCallback):
		code = xerr.InvalidParams
	case errors.Is(err, oauthprovider.ErrInvalidCallback):
		code = xerr.OAuthCallbackInvalid
	case errors.Is(err, oauthprovider.ErrExpiredCallback):
		code = xerr.OAuthCallbackExpired
	case errors.Is(err, oauthprovider.ErrMisconfigured):
		code = xerr.OAuthProviderMisconfigured
	}
	if code == xerr.OAuthProviderError || code == xerr.OAuthProviderMisconfigured {
		logger.WithContext(ctx).Errorw("oauth provider failed", logger.Field("method", method), logger.Field("error", err.Error()))
	}
	return xerr.Wrapf(err, code, "%s sign-in", method)
}
