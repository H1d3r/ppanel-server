// Package httpserver builds the Hertz server of the HTTP API: the middleware
// every request passes (tracing, request logging, CORS), the routes of
// package routes, and the Telegram webhook and payment notify handlers that
// their modules register.
package httpserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync/atomic"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/config"
	billingHTTP "github.com/perfect-panel/server/internal/module/billing/transport/http"
	"github.com/perfect-panel/server/internal/module/notification"
	notificationHTTP "github.com/perfect-panel/server/internal/module/notification/transport/http"
	"github.com/perfect-panel/server/internal/transport/http/middleware"
	"github.com/perfect-panel/server/internal/transport/http/routes"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
)

type Server struct {
	h *server.Hertz
	// shuttingDown records that Shutdown was called, so Start tells the
	// listener closing on purpose from one that failed.
	shuttingDown atomic.Bool
}

type Dependencies struct {
	Routes           routes.Dependencies
	Notification     notification.Service
	TelegramBotToken func() string
	RequestMetadata  requestmeta.Enricher
}

func New(deps Dependencies, addr string, tlsConfig *tls.Config) *Server {
	opts := []config.Option{
		server.WithHostPorts(addr),
		server.WithDisablePrintRoute(true),
	}
	if tlsConfig != nil {
		opts = append(opts, server.WithTLS(tlsConfig))
	}

	return newServer(deps, opts)
}

func newServer(deps Dependencies, opts []config.Option) *Server {
	engine := server.Default(opts...)
	engine.Use(middleware.TraceMiddleware(), middleware.LoggerMiddleware(deps.RequestMetadata), middleware.CorsMiddleware)

	routes.RegisterHandlers(engine, deps.Routes)
	notificationHTTP.RegisterTelegramHandlers(engine, deps.Notification, deps.TelegramBotToken)
	billingHTTP.RegisterNotifyHandlers(engine, deps.Routes.Billing)

	return &Server{h: engine}
}

// Start serves until Shutdown and returns the error that stopped the server
// before then, a listener that could not bind above all. It used to log that
// error only, leaving a process that consumed tasks with no API to serve
// them; the caller decides now, and fails fast.
func (s *Server) Start() (err error) {
	// Hertz's netpoll transport panics instead of returning the error when
	// the listener cannot bind; the standard transport returns it. Both
	// reach the caller as the error.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("http server: %v", r)
			logger.Errorf("server start error: %s", err.Error())
		}
	}()
	err = s.h.Run()
	if err == nil || s.shuttingDown.Load() {
		// The listener closed because Shutdown asked it to.
		return nil
	}
	logger.Errorf("server start error: %s", err.Error())
	return err
}

// Shutdown stops accepting connections and waits for the open requests
// until ctx ends; Start returns nil afterwards.
func (s *Server) Shutdown(ctx context.Context) error {
	s.shuttingDown.Store(true)
	return s.h.Shutdown(ctx)
}

func (s *Server) Engine() *server.Hertz {
	return s.h
}
