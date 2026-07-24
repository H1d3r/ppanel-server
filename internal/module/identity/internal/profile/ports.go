// Package profile implements the self-service identity subdomain of the
// identity module: account info, credentials, third-party bindings, devices
// and notification preferences. Only the module facade may reach it.
package profile

import (
	"context"

	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// OAuthMethodPolicy checks whether a requested authentication method is
// enabled; the composition root adapts the legacy register policy until the
// auth flows join the module.
type OAuthMethodPolicy interface {
	EnsureMethodEnabled(ctx context.Context, method string) error
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	Users     repository.UserRepo
	UserAuth  repository.UserAuthRepo
	Auth      repository.AuthRepo
	Devices   repository.UserDeviceRepo
	UserCache repository.UserCacheRepo
	Logs      repository.LogRepo
	Redis     *redis.Client
	// Store carries the identity-scoped transaction for device unbinding.
	Store  repository.Store
	Policy OAuthMethodPolicy

	// EmailDomains snapshots the runtime-mutable email domain-suffix policy.
	EmailDomains func() (domainList string, restrict bool)
	// TelegramBotName snapshots the runtime-mutable Telegram bot name.
	TelegramBotName func() string
	// NotifyUnbind sends the best-effort Telegram unbind notice through the
	// runtime-configured bot.
	NotifyUnbind func(userID, chatID int64) error
}
