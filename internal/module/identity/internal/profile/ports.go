// Package profile implements the self-service identity subdomain of the
// identity module: account info, credentials, third-party bindings, devices
// and notification preferences. Only the module facade may reach it.
package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/module/identity/internal/oauthflow"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Users     repository.UserRepo
	UserAuth  repository.UserAuthRepo
	Auth      repository.AuthRepo
	Devices   repository.UserDeviceRepo
	UserCache repository.UserCacheRepo
	Logs      repository.LogRepo
	// Wallet is the billing-domain read port for the account view.
	Wallet repository.WalletRepo
	Redis  *redis.Client
	// Store carries the identity-scoped transaction for device unbinding.
	Store Store
	// Policy gates method rebinding on the same switches as sign-in.
	Policy registerpolicy.Policy
	// OAuth runs the provider round trip of account binding, shared with
	// sign-in.
	OAuth *oauthflow.Flow

	// EmailDomains snapshots the runtime-mutable email domain-suffix policy.
	EmailDomains func() (domainList string, restrict bool)
	// TelegramBotName snapshots the runtime-mutable Telegram bot name.
	TelegramBotName func() string
	// NotifyUnbind sends the best-effort Telegram unbind notice through the
	// runtime-configured bot.
	NotifyUnbind func(ctx context.Context, userID, chatID int64) error
	KickDevice   func(userID int64, identifier string)
}

// Store is the persistence capability required by this package. It excludes
// unrelated repositories and application-wide transactions.
type Store interface {
	repository.IdentityTransactor
}
