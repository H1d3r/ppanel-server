// Package verifycode implements the verification-code subdomain of the
// identity module: issuing and pre-checking the email and SMS codes that gate
// registration and account mutations. Only the module facade may reach it.
package verifycode

import (
	"context"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/identity/internal/authn/registerpolicy"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
)

// Snapshot is the per-request view of the runtime-mutable settings the
// verification-code flows consume.
type Snapshot struct {
	DomainSuffixList   string
	EnableDomainSuffix bool
	VerifyCodeInterval int64
	VerifyCodeLimit    int64
	VerifyCodeExpire   int64
	// MobileWhitelist lists the area codes SMS codes may be sent to when
	// MobileWhitelistEnabled is set.
	MobileWhitelistEnabled bool
	MobileWhitelist        []string
	SiteLogo               string
	SiteName               string
}

// VerificationIdentityStore is the persistence surface the code flows use
// to tell registration from security codes.
type VerificationIdentityStore interface {
	UserAuth() repository.UserAuthRepo
}

// VerificationTaskQueue publishes verification-code delivery tasks.
type VerificationTaskQueue interface {
	EnqueueContext(ctx context.Context, task *asynq.Task, opts ...asynq.Option) (*asynq.TaskInfo, error)
}

// Deps declares the subdomain's dependencies; the identity facade forwards
// them from the composition root and supplies the account policy from the
// authentication subdomain.
type Deps struct {
	Store  VerificationIdentityStore
	Redis  *redis.Client
	Queue  VerificationTaskQueue
	Policy registerpolicy.Policy
	// Config snapshots the runtime-mutable settings per request.
	Config func() Snapshot
}

// Service is the verification-code subdomain entry point used by the
// identity facade.
type Service struct {
	deps Deps
}

// NewService builds the subdomain over the dependencies the facade forwards.
func NewService(deps Deps) *Service {
	return &Service{deps: deps}
}
