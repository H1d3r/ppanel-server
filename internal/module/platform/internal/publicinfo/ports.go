package publicinfo

import (
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// GlobalConfigStore is the persistence surface used by public global
// configuration reads. It excludes unrelated application repositories.
type GlobalConfigStore interface {
	System() repository.SystemRepo
	Auth() repository.AuthRepo
}

// GlobalConfigSnapshot is the public runtime configuration consumed when
// assembling the global configuration response.
type GlobalConfigSnapshot struct {
	Site      config.SiteConfig
	Subscribe config.SubscribeConfig
	Email     config.EmailConfig
	Mobile    config.MobileConfig
	Register  config.RegisterConfig
	Verify    config.Verify
	Invite    config.InviteConfig
}

// GetGlobalConfigDependencies explicitly declares the collaborators of the
// global configuration query instead of passing ServiceContext to business
// logic.
type GetGlobalConfigDependencies struct {
	Store  GlobalConfigStore
	Config GlobalConfigSnapshot
}

// Deps declares the subdomain's dependencies; the module facade forwards
// them from the composition root.
type Deps struct {
	// Store carries the public read surface (system settings, auth methods,
	// user/node counters, client downloads).
	Store repository.Store
	Redis *redis.Client
	// Config snapshots the runtime-mutable public configuration per request.
	Config func() GlobalConfigSnapshot
}
