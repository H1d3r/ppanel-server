package authmethodadmin

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type GetAuthMethodConfigLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewGetAuthMethodConfigLogic Get auth method config
func newGetAuthMethodConfigLogic(ctx context.Context, deps Deps) *GetAuthMethodConfigLogic {
	return &GetAuthMethodConfigLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetAuthMethodConfigLogic) GetAuthMethodConfig(req *dto.GetAuthMethodConfigRequest) (resp *dto.AuthMethodConfig, err error) {
	method, err := l.deps.Auths.FindOneByMethod(l.ctx, req.Method)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "find auth method %q", req.Method)
	}

	resp = new(dto.AuthMethodConfig)
	mapping.DeepCopy(resp, method)
	if method.Config != "" {
		if err := json.Unmarshal([]byte(method.Config), &resp.Config); err != nil {
			return nil, xerr.Wrapf(err, xerr.ERROR, "decode stored %s config", req.Method)
		}
	}
	return
}
