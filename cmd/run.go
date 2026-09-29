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
	"github.com/perfect-panel/server/internal/app/buildinfo"
	"github.com/perfect-panel/server/internal/app/lifecycle"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/transport/http/setup"
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
		run(cmd.Context())
	},
}

// run starts the servers and stops them when the process receives SIGINT,
// SIGTERM or SIGQUIT. ctx is the command's root context: start-up serves no
// request, so nothing narrower exists.
func run(ctx context.Context) {
	services := getServers(ctx)
	defer services.Stop()
	go services.Start()
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	<-quit
}

// getServers loads the configuration, running the setup wizard first when it
// is incomplete, prepares the process (clock zone, logger) and builds the
// servers from it.
func getServers(ctx context.Context) *lifecycle.Group {
	var c config.Config
	createConfigFileIfMissing()
	// The wizard writes the file and reports on status once it is complete.
	if initConfig(&c) {
		status, engine := setup.Start(startConfigPath)
		<-status
		if err := engine.Shutdown(ctx); err != nil {
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
	// The application timezone is the process's single clock: business times,
	// GORM's timestamps and the database session all use it.
	if err := timeutil.LoadProcessLocation(c.AppLocation); err != nil {
		logger.Errorf("load app timezone %q failed: %v, falling back to Local", c.AppLocation, err)
	}
	if err := logger.SetUp(c.Logger); err != nil {
		logger.Errorf("Logger setup failed: %v", err.Error())
	}

	return app.NewServices(c)
}

// createConfigFileIfMissing creates an empty configuration file, and the etc
// directory, when the file does not exist, so that initConfig starts the setup
// wizard on it. It ends the process when either cannot be created.
func createConfigFileIfMissing() {
	if _, err := os.Stat(startConfigPath); !os.IsNotExist(err) {
		return
	}
	if _, err := os.Stat("etc"); os.IsNotExist(err) {
		logger.Errorf("Directory %s does not exist. Creating it...\n", "etc")
		if err = os.MkdirAll("etc", os.ModePerm); err != nil {
			log.Fatalf("Please create the directory %s and place the configuration file %s in it.\n", "etc", startConfigPath)
		}
	}
	file, err := os.Create(startConfigPath)
	if err != nil {
		logger.Errorf("Please create the configuration file %s in the directory %s.\n", startConfigPath, "etc")
		panic(fmt.Sprintf("Please create the configuration file %s in the directory %s.\n", startConfigPath, "etc"))
	}
	// Nothing was written, so a failed close loses nothing; the wizard writes
	// the file through its own handle.
	_ = file.Close()
}

// initConfig loads the configuration file into c and reports whether the
// setup wizard has to run first: a custom file must name a database, and the
// default file without a JWT secret is a new installation unless the
// PPANEL_DB and PPANEL_REDIS environment variables complete it. When they do,
// the configuration gets a generated secret and those connections and is
// written back to the file.
func initConfig(c *config.Config) bool {
	conf.MustLoad(startConfigPath, c)
	if startConfigPath != "etc/ppanel.yaml" && c.DatabaseConfig().Addr == "" {
		return true
	}
	if c.JwtAuth.AccessSecret != "" || startConfigPath != "etc/ppanel.yaml" {
		return false
	}
	c.JwtAuth.AccessSecret = uuid.NewV4().String()
	dsn := os.Getenv("PPANEL_DB")
	if dsn == "" {
		return true
	}
	// The session zone of the new database follows the file's AppLocation:
	// stored times and daily statistics are read in the session zone.
	cfg := orm.ParseDSNIn(dsn, c.AppLocation)
	if cfg == nil {
		return true
	}
	c.SetDatabaseConfig(*cfg)

	uri := os.Getenv("PPANEL_REDIS")
	if uri == "" {
		return true
	}
	addr, pass, db, err := config.ParseRedisURI(uri)
	if err != nil {
		return true
	}
	c.Redis.Host = addr
	c.Redis.Pass = pass
	c.Redis.DB = db

	// Every boot setting the file already had is written back with the
	// generated secret and the connections.
	newConfig := config.File{
		Host:          c.Host,
		AppLocation:   c.AppLocation,
		Port:          c.Port,
		Transport:     c.Transport,
		TLS:           c.TLS,
		Debug:         c.Debug,
		JwtAuth:       c.JwtAuth,
		Logger:        c.Logger,
		Trace:         c.Trace,
		Database:      c.DatabaseConfig(),
		Redis:         c.Redis,
		EdgeSubscribe: c.EdgeSubscribe,
	}
	fileData, err := yaml.Marshal(newConfig)
	if err != nil {
		log.Fatalf("encode the configuration: %v", err)
	}
	// The file holds the JWT secret and database credentials, and WriteFile
	// keeps the mode of an existing file.
	if err := os.WriteFile(startConfigPath, fileData, 0600); err != nil {
		log.Fatalf("write %s: %v", startConfigPath, err)
	}
	if err := os.Chmod(startConfigPath, 0600); err != nil {
		log.Fatalf("restrict %s to its owner: %v", startConfigPath, err)
	}
	return false
}
