package svc

import (
	"github.com/perfect-panel/server/internal/module/support"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// NewStore assembles the shared store from the module-owned repo builders
// (falling back to the repository package's legacy builders for
// implementations that have not migrated yet). One connection pool, module-
// owned persistence (ADR-001 step-6 preparation).
func NewStore(db *gorm.DB, rds *redis.Client) *repository.GormStore {
	builders := repository.LegacyBuilders(rds)
	builders.Support = support.NewRepoBuilder()
	return repository.NewGormStoreWithBuilders(db, rds, builders)
}
