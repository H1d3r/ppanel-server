// Package publicinfo implements the public-info subdomain of the platform
// module: the unauthenticated site-level reads (global configuration, terms
// of service and privacy policy, aggregate statistics, client downloads,
// liveness). Only the module facade may reach it.
package publicinfo

import (
	"time"

	"golang.org/x/sync/singleflight"
)

// Service is the public-info subdomain entry point used by the platform
// facade.
type Service struct {
	deps Deps
	// statRefresh collapses concurrent refreshes of the site statistics.
	statRefresh singleflight.Group
	// refreshTimeout and resolveTimeout are the budgets of one statistics
	// refresh and of the hostname resolution inside it (statRefreshTimeout
	// and statResolveTimeout); tests shorten them.
	refreshTimeout, resolveTimeout time.Duration
}

// NewService builds the public-info service.
func NewService(deps Deps) *Service {
	return &Service{deps: deps, refreshTimeout: statRefreshTimeout, resolveTimeout: statResolveTimeout}
}
