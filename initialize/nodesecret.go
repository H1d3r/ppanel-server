package initialize

import (
	"context"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/random"
	"github.com/perfect-panel/server/pkg/tool"
)

// LegacyDefaultNodeSecret is the node secret the original seed data shipped. The
// value is public knowledge, so an installation still carrying it serves the
// node API — which hands out every user's subscription uuid — to anyone.
const LegacyDefaultNodeSecret = "12345678"

// nodeSecretLength yields about 190 bits over the 62-character alphabet.
const nodeSecretLength = 32

// NodeSecret provisions a random node secret when the database does not carry
// one yet, so a fresh installation never serves the node API with a guessable
// credential. It has to run after Migrate, which seeds the empty row, and
// before Node, which copies the value into the runtime config.
//
// An installation still holding LegacyDefaultNodeSecret is only reported, never
// rotated: every node was configured with that secret, so rotating it here would
// silently cut them off. The operator rotates it from the admin panel and
// reconfigures the nodes in the same window.
func NodeSecret(svcCtx *svc.ServiceContext) {
	logger.Debug("Node secret initialization")
	// The read and the write share a transaction so the read goes to the
	// database instead of Redis; GetNodeConfig is a cached query, and the write
	// below does not invalidate that cache.
	err := svcCtx.Store.InPlatformTx(context.Background(), func(store repository.PlatformStore) error {
		configs, err := store.System().GetNodeConfig(context.Background())
		if err != nil {
			return err
		}
		var nodeConfig config.NodeDBConfig
		tool.SystemConfigSliceReflectToStruct(configs, &nodeConfig)

		switch nodeConfig.NodeSecret {
		case "":
			secret := random.KeyNew(nodeSecretLength, 1)
			if err := store.System().UpdateValueByCategoryKey(context.Background(), "server", "NodeSecret", secret); err != nil {
				return err
			}
			logger.Info("[NodeSecret] generated a random node secret, read it from the admin panel to configure nodes")
		case LegacyDefaultNodeSecret:
			logger.Error("[NodeSecret] the node secret is still the well-known default, rotate it from the admin panel and reconfigure every node")
		}
		return nil
	})
	if err != nil {
		logger.Errorf("[NodeSecret] provision error: %v", err.Error())
		panic(err)
	}
}
