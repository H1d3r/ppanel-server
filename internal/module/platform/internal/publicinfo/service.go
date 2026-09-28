// Package publicinfo implements the public-info subdomain of the platform
// module: the unauthenticated site-level reads (global configuration, terms
// of service and privacy policy, aggregate statistics, client downloads,
// liveness). Only the module facade may reach it.
package publicinfo

import (
	"golang.org/x/sync/singleflight"
)

// Service is the public-info subdomain entry point used by the platform
// facade.
type Service struct {
	deps Deps
	// statRefresh collapses concurrent refreshes of the site statistics.
	statRefresh singleflight.Group
}

// NewService builds the public-info service.
func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}
