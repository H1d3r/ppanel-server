package app

import (
	"context"

	"github.com/perfect-panel/server/internal/app/bootstrap"
	"github.com/perfect-panel/server/internal/app/lifecycle"
	"github.com/perfect-panel/server/internal/app/scheduler"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/notification"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/transport/http/routes"
	httpserver "github.com/perfect-panel/server/internal/transport/http/server"
	"github.com/perfect-panel/server/internal/transport/task"
	"github.com/perfect-panel/server/internal/transport/task/email"
	"github.com/perfect-panel/server/internal/transport/task/order"
	"github.com/perfect-panel/server/internal/transport/task/sms"
	"github.com/perfect-panel/server/internal/transport/task/traffic"
)

// NewServices assembles the application from the configuration and returns
// the services the process runs: the HTTP server with the runtime bootstrap,
// the task worker and the scheduler.
func NewServices(c config.Config) *lifecycle.Group {
	return NewApplication(c).services(c)
}

func (srv *Application) services(c config.Config) *lifecycle.Group {
	// The group starts the services together. The task handlers read the
	// runtime settings the HTTP service's bootstrap loads, so the worker
	// consumes only once the bootstrap signals them; the scheduler only
	// enqueues, and its tasks wait in the queue until then.
	bootstrapped := lifecycle.NewReadiness()
	services := lifecycle.NewServiceGroup()
	services.Add(NewService(srv.serviceDependencies(bootstrapped)))
	services.Add(task.NewService(QueueRedisOpt(c), srv.taskDependencies(bootstrapped)))
	services.Add(scheduler.NewService(QueueRedisOpt(c), c.AppLocation))
	return services
}

// serviceDependencies are the HTTP service's: the runtime bootstrap it runs
// before listening and reports on bootstrapped, the routes it serves and the
// runtime hooks it installs.
func (srv *Application) serviceDependencies(bootstrapped *lifecycle.Readiness) Dependencies {
	return Dependencies{
		Config:       srv.Runtime.Config,
		Identity:     srv.Identity,
		Bootstrap:    srv.bootstrapDependencies(),
		Bootstrapped: bootstrapped,
		HTTP: func() httpserver.Dependencies {
			return httpserver.Dependencies{
				Routes:           srv.routeDependencies(),
				Notification:     srv.Notification,
				TelegramBotToken: func() string { return srv.Runtime.Config().Telegram.BotToken },
				RequestMetadata:  srv.GeoIP.Enrich,
			}
		},
		SetRestart:             srv.Runtime.SetRestart,
		SetReinitializeHandler: srv.Runtime.SetReinitialize,
	}
}

func (srv *Application) bootstrapDependencies() *bootstrap.Dependencies {
	return &bootstrap.Dependencies{
		Config:                   srv.Runtime.Config,
		UpdateRuntime:            srv.Runtime.UpdateRuntime,
		Settings:                 srv.Store.System(),
		SettingsTx:               bootstrapSettingsTx{store: srv.Store},
		ExchangeRate:             srv.ExchangeRate,
		Notification:             srv.Notification,
		SetTelegramBot:           srv.Runtime.SetTelegramBot,
		SetNodeMultiplierManager: srv.Runtime.SetNodeMultiplierManager,
		LoginMethods:             srv.Identity,
		Administrators:           srv.Identity,
	}
}

func (srv *Application) routeDependencies() routes.Dependencies {
	return routes.Dependencies{
		ConfigProvider: srv.Runtime.Config,
		Redis:          srv.Redis,
		Support:        srv.Support,
		Billing:        srv.Billing,
		Platform:       srv.Platform,
		Subscription:   srv.Subscription,
		Identity:       srv.Identity,
		Network:        srv.Network,
	}
}

// taskDependencies are the task worker's, with the signal that the runtime
// settings it reads are loaded. The traffic tasks only flush the
// aggregator's buckets: reports enter through the node API, which checks the
// served subscriptions.
func (srv *Application) taskDependencies(bootstrapped *lifecycle.Readiness) task.Dependencies {
	runtimeConfig := srv.Runtime.Config
	return task.Dependencies{
		Bootstrapped: bootstrapped,
		Email: email.Dependencies{
			Tasks:    srv.Store.Task(),
			Logs:     srv.Store.Log(),
			Queue:    srv.Queue,
			Email:    func() config.EmailConfig { return runtimeConfig().Email },
			SiteName: func() string { return runtimeConfig().Site.SiteName },
		},
		SMS: sms.Dependencies{
			Logs:   srv.Store.Log(),
			Mobile: func() config.MobileConfig { return runtimeConfig().Mobile },
		},
		Order: order.Dependencies{
			Queue:        srv.Queue,
			Inspector:    srv.Inspector,
			Billing:      srv.Billing,
			Notification: srv.Notification,
			Telegram:     func() config.Telegram { return runtimeConfig().Telegram },
			Outbox:       srv.Store.Outbox(),
			Inbox:        srv.Store.Inbox(),
		},
		EventBus: srv.EventBus,
		Traffic: traffic.Dependencies{
			Redis:      srv.Redis,
			Statistics: srv.Network,
			Logs:       srv.Platform,
			Aggregator: network.TrafficAggregatorDeps{
				Usage:         srv.TrafficUsage,
				Store:         network.NewTrafficAggregatorStore(srv.Store),
				Subscriptions: srv.Subscription,
				Redis:         srv.Redis,
			},
		},
		Subscription: srv.Subscription,
		Tasks:        srv.Store.Task(),
		System:       srv.Store.System(),
		ExchangeRate: srv.ExchangeRate,
	}
}

// bootstrapSettingsTx runs the bootstrap's node-secret provisioning in a
// platform transaction of the shared store, handing it the transaction's
// system settings.
type bootstrapSettingsTx struct{ store repository.PlatformTransactor }

var _ bootstrap.SettingsTransactor = bootstrapSettingsTx{}

func (t bootstrapSettingsTx) InSettingsTx(ctx context.Context, fn func(bootstrap.NodeSettings) error) error {
	return t.store.InPlatformTx(ctx, func(tx repository.PlatformStore) error {
		return fn(tx.System())
	})
}

// The notification facade is the bootstrap's Telegram bot port.
var _ bootstrap.TelegramNotifications = notification.Service(nil)
