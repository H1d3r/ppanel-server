package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"uuid"

	"github.com/perfect-panel/server/internal/app"
	"github.com/perfect-panel/server/internal/app/bootstrap"
	"github.com/perfect-panel/server/internal/app/buildinfo"
	"github.com/perfect-panel/server/internal/app/lifecycle"
	"github.com/perfect-panel/server/internal/app/scheduler"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/module/network"
	"github.com/perfect-panel/server/internal/module/subscription"
	"github.com/perfect-panel/server/internal/transport/http/routes"
	httpserver "github.com/perfect-panel/server/internal/transport/http/server"
	"github.com/perfect-panel/server/internal/transport/http/setup"
	"github.com/perfect-panel/server/internal/transport/task"
	"github.com/perfect-panel/server/internal/transport/task/email"
	"github.com/perfect-panel/server/internal/transport/task/order"
	"github.com/perfect-panel/server/internal/transport/task/sms"
	"github.com/perfect-panel/server/internal/transport/task/traffic"
	"github.com/perfect-panel/server/pkg/conf"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/orm"
	"github.com/perfect-panel/server/pkg/timeutil"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func init() {
	startCmd.Flags().StringVar(&startConfigPath, "config", "etc/ppanel.yaml", "ppanel.yaml directory to read from")
}

var (
	startConfigPath string
)

var startCmd = &cobra.Command{
	Use:   "run",
	Short: "start PPanel",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("[PPanel version] " + buildinfo.Display())
		run()
	},
}

func run() {
	services := getServers()
	defer services.Stop()
	go services.Start()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit
}
func getServers() *lifecycle.Group {
	var c config.Config

	// check config file is exist
	if _, err := os.Stat(startConfigPath); os.IsNotExist(err) {
		// check directory is existed
		if _, err := os.Stat("etc"); os.IsNotExist(err) {
			logger.Errorf("Directory %s does not exist. Creating it...\n", "etc")
			if err = os.MkdirAll("etc", os.ModePerm); err != nil {
				log.Fatalf("Please create the directory %s and place the configuration file %s in it.\n", "etc", startConfigPath)
			}
		}
		// create new config file
		if _, err := os.Create(startConfigPath); err != nil {
			logger.Errorf("Please create the configuration file %s in the directory %s.\n", startConfigPath, "etc")
			panic(fmt.Sprintf("Please create the configuration file %s in the directory %s.\n", startConfigPath, "etc"))
		}
	}
	// check config file is empty, if empty, start init web server
	if initConfig(&c) {
		status, engine := setup.Start(startConfigPath)
		<-status
		if err := engine.Shutdown(context.TODO()); err != nil {
			log.Printf("Init Server Shutdown: %s\n", err.Error())
		}
	}
	conf.MustLoad(startConfigPath, &c)
	// Sessions, order event tickets and guest-checkout HMACs are all keyed by
	// this secret; an empty one lets anyone forge them.
	if c.JwtAuth.AccessSecret == "" {
		log.Fatalf("JwtAuth.AccessSecret is empty in %s; set a long random secret", startConfigPath)
	}
	if len(c.JwtAuth.AccessSecret) < 16 {
		log.Printf("warning: JwtAuth.AccessSecret in %s is shorter than 16 characters and can be guessed; replace it with a long random secret", startConfigPath)
	}
	// Initialize application timezone
	if err := timeutil.LoadLocation(c.AppLocation); err != nil {
		logger.Errorf("load app timezone %q failed: %v, falling back to Local", c.AppLocation, err)
	}
	// init logger
	if err := logger.SetUp(c.Logger); err != nil {
		logger.Errorf("Logger setup failed: %v", err.Error())
	}

	// init service context
	ctx := app.NewApplication(c)
	runtimeConfig := ctx.Runtime.Config
	bootstrapDeps := &bootstrap.Dependencies{
		Config:                   runtimeConfig,
		UpdateConfig:             ctx.Runtime.UpdateConfig,
		Store:                    ctx.Store,
		ExchangeRate:             ctx.ExchangeRate,
		Notification:             ctx.Notification,
		SetTelegramBot:           ctx.Runtime.SetTelegramBot,
		SetNodeMultiplierManager: ctx.Runtime.SetNodeMultiplierManager,
	}
	routeDeps := func() routes.Dependencies {
		return routes.Dependencies{
			ConfigProvider: runtimeConfig,
			Redis:          ctx.Redis,
			Store:          ctx.Store,
			Support:        ctx.Support,
			Billing:        ctx.Billing,
			Platform:       ctx.Platform,
			Subscription:   ctx.Subscription,
			Identity:       ctx.Identity,
			Network:        ctx.Network,
		}
	}
	trafficDeps := traffic.Dependencies{
		Store: ctx.Store,
		Redis: ctx.Redis,
		Log:   func() config.Log { return runtimeConfig().Log },
		// The tasks only flush buckets; reports enter through the node API,
		// which checks the served subscriptions.
		Aggregator: network.TrafficAggregatorDeps{
			Usage: subscription.NewTrafficUsage(ctx.Store),
			Store: ctx.Store,
			Redis: ctx.Redis,
		},
	}
	queueDeps := task.Dependencies{
		Email: email.Dependencies{
			Store:    ctx.Store,
			Queue:    ctx.Queue,
			Email:    func() config.EmailConfig { return runtimeConfig().Email },
			SiteName: func() string { return runtimeConfig().Site.SiteName },
		},
		SMS: sms.Dependencies{
			Store:  ctx.Store,
			Mobile: func() config.MobileConfig { return runtimeConfig().Mobile },
			Model:  func() string { return runtimeConfig().Model },
		},
		Order: order.Dependencies{
			Store:        ctx.Store,
			Redis:        ctx.Redis,
			Queue:        ctx.Queue,
			Inspector:    ctx.Inspector,
			Billing:      ctx.Billing,
			Subscription: ctx.Subscription,
			Notification: ctx.Notification,
			Telegram:     func() config.Telegram { return runtimeConfig().Telegram },
		},
		EventBus:     ctx.EventBus,
		Traffic:      trafficDeps,
		Subscription: ctx.Subscription,
		Store:        ctx.Store,
		ExchangeRate: ctx.ExchangeRate,
	}

	services := lifecycle.NewServiceGroup()
	services.Add(app.NewService(app.Dependencies{
		Config:    runtimeConfig,
		Store:     ctx.Store,
		Bootstrap: bootstrapDeps,
		HTTP: func() httpserver.Dependencies {
			return httpserver.Dependencies{
				Routes:           routeDeps(),
				Notification:     ctx.Notification,
				TelegramBotToken: func() string { return runtimeConfig().Telegram.BotToken },
				RequestMetadata:  ctx.GeoIP.Enrich,
			}
		},
		SetRestart:             ctx.Runtime.SetRestart,
		SetReinitializeHandler: ctx.Runtime.SetReinitialize,
	}))
	services.Add(task.NewService(app.QueueRedisOpt(c), queueDeps))
	services.Add(scheduler.NewService(app.QueueRedisOpt(c), c.AppLocation))
	return services
}

func initConfig(c *config.Config) bool {
	// load config
	conf.MustLoad(startConfigPath, c)
	//  check custom config
	if startConfigPath != "etc/ppanel.yaml" && c.DatabaseConfig().Addr == "" {
		return true
	}
	// check access secret
	if c.JwtAuth.AccessSecret == "" && startConfigPath == "etc/ppanel.yaml" {
		c.JwtAuth.AccessSecret = uuid.NewV4().String()
		// Get environment variables
		dsn := os.Getenv("PPANEL_DB")
		if dsn == "" {
			return true
		}
		cfg := orm.ParseDSN(dsn)
		if cfg == nil {
			return true
		} else {
			c.SetDatabaseConfig(*cfg)
		}

		// Get environment variables
		uri := os.Getenv("PPANEL_REDIS")
		if uri == "" {
			return true
		}
		addr, pass, db, err := config.ParseRedisURI(uri)
		if err != nil {
			return true
		} else {
			c.Redis.Host = addr
			c.Redis.Pass = pass
			c.Redis.DB = db
		}
		// save yaml file
		newConfig := config.File{
			Host:     c.Host,
			Port:     c.Port,
			Debug:    c.Debug,
			JwtAuth:  c.JwtAuth,
			Logger:   c.Logger,
			Trace:    c.Trace,
			Database: c.DatabaseConfig(),
			Redis:    c.Redis,
		}
		fileData, err := yaml.Marshal(newConfig)
		if err != nil {
			panic(err.Error())
		}
		// write to file; the file holds the JWT secret and database
		// credentials, and WriteFile keeps the mode of an existing file
		if err := os.WriteFile(startConfigPath, fileData, 0600); err != nil {
			panic(err.Error())
		}
		if err := os.Chmod(startConfigPath, 0600); err != nil {
			panic(err.Error())
		}
	}
	return false
}
