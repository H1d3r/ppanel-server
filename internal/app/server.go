package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/app/bootstrap"
	"github.com/perfect-panel/server/internal/config"
	httpserver "github.com/perfect-panel/server/internal/transport/http/server"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/trace"
	"github.com/perfect-panel/server/pkg/xerr"
)

// Service is the HTTP service: it runs the bootstrap, then serves the routes,
// and restarts the server when an administrator changes the subscribe path.
type Service struct {
	server transportServer
	deps   Dependencies
}

// Dependencies is what the HTTP service needs to start and restart: the
// runtime configuration and the bootstrap that loads it, the identity
// module's startup work, the routes and the runtime hooks it installs.
type Dependencies struct {
	Config                 func() config.Config
	Bootstrap              *bootstrap.Dependencies
	HTTP                   func() httpserver.Dependencies
	SetRestart             func(func() error)
	SetReinitializeHandler func(func(string) error)
	// Identity runs the identity module's startup check and data fix-up
	// once the bootstrap migrated the schema; the identity facade provides
	// it.
	Identity IdentityStartup
}

// IdentityStartup is the identity module's part of the server start: the
// check that refuses stored email bindings email sign-in cannot tell apart,
// and the fix-up that stores phone numbers in E.164.
type IdentityStartup interface {
	ValidateEmailIdentities(ctx context.Context) error
	NormalizePhoneNumbers(ctx context.Context) error
}

// NewService builds the HTTP service; Start runs the bootstrap.
func NewService(deps Dependencies) *Service {
	return &Service{deps: deps}
}

type transportServer interface {
	Start()
	Shutdown(ctx context.Context) error
}

func newTransportServer(deps httpserver.Dependencies, runtimeConfig config.Config, addr string) transportServer {
	var tlsConfig *tls.Config
	if runtimeConfig.TLS.Enable {
		cert, err := tls.LoadX509KeyPair(runtimeConfig.TLS.CertFile, runtimeConfig.TLS.KeyFile)
		if err != nil {
			// Fail fast: a process that keeps running without listening
			// hides the outage from the orchestrator.
			logger.Errorf("load tls certificate error: %s", err.Error())
			panic(fmt.Sprintf("load tls certificate: %v", err))
		}
		tlsConfig = &tls.Config{
			MinVersion:   tls.VersionTLS12,
			Certificates: []tls.Certificate{cert},
		}
	}
	return httpserver.New(deps, addr, tlsConfig)
}

// Start loads the runtime configuration, installs the runtime hooks and
// serves until Stop or Restart.
func (m *Service) Start() {
	if m.deps.Config == nil || m.deps.Bootstrap == nil || m.deps.HTTP == nil {
		panic("the HTTP service is missing its configuration, bootstrap or routes")
	}

	// The start-up work belongs to no request.
	ctx := context.Background()
	runtimeConfig := m.deps.Config()
	serverAddr := fmt.Sprintf("%v:%d", runtimeConfig.Host, runtimeConfig.Port)
	if err := bootstrap.Start(ctx, m.deps.Bootstrap); err != nil {
		// Fail fast: serving with a partially loaded configuration would
		// silently run with defaults such as open registration. Detail keeps
		// the database failure behind a coded error, such as the first
		// administrator's, in the line.
		logger.Errorf("bootstrap error: %s", xerr.Detail(err))
		panic(err)
	}
	if err := m.deps.Identity.ValidateEmailIdentities(ctx); err != nil {
		logger.Errorf("stored email identities: %s", xerr.Detail(err))
		panic(err)
	}
	normalizeIdentityData(ctx, m.deps.Identity)
	m.server = newTransportServer(m.deps.HTTP(), m.deps.Config(), serverAddr)
	traceConfig := runtimeConfig.Trace
	if traceConfig.Name == "" {
		traceConfig.Name = trace.TraceName
	}
	trace.StartAgent(traceConfig)
	if m.deps.SetRestart != nil {
		m.deps.SetRestart(m.Restart)
	}
	// A failed reload keeps the previous configuration and is reported to
	// the administrator who changed the settings. The hook carries no
	// context (the platform and identity modules call it after saving the
	// settings), so the reload runs on a root context.
	reinitialize := func(subsystem string) error {
		return bootstrap.Reload(context.Background(), m.deps.Bootstrap, bootstrap.Subsystem(subsystem))
	}
	if m.deps.SetReinitializeHandler != nil {
		m.deps.SetReinitializeHandler(reinitialize)
	}
	logger.Infof("server start at %v", serverAddr)
	m.server.Start()
}

// Stop shuts the server down, giving open requests five seconds, then
// flushes the spans the trace exporter still holds.
func (m *Service) Stop() {
	if m.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.server.Shutdown(ctx); err != nil {
		logger.Errorf("server shutdown error: %s", err.Error())
	}
	logger.Info("server shutdown")
	trace.StopAgent()
}

// Restart shuts the server down and starts it again with the current
// configuration. An administrator's request triggers it, and that request is
// served by the server being shut down, so the shutdown runs on its own
// context.
func (m *Service) Restart() error {
	if m.server == nil {
		return errors.New("server is nil")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.server.Shutdown(ctx); err != nil {
		logger.Errorf("server shutdown error: %v", err.Error())
		return err
	}
	logger.Info("server shutdown")
	go m.Start()
	return nil
}
