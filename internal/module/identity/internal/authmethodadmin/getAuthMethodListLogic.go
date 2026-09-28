package authmethodadmin

import (
	"context"
	"encoding/json"

	"github.com/perfect-panel/server/internal/infra/mapping"
	dto "github.com/perfect-panel/server/internal/module/identity/contract"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
)

type GetAuthMethodListLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// NewGetAuthMethodListLogic Get auth method list
func newGetAuthMethodListLogic(ctx context.Context, deps Deps) *GetAuthMethodListLogic {
	return &GetAuthMethodListLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetAuthMethodListLogic) GetAuthMethodList() (resp *dto.GetAuthMethodListResponse, err error) {
	methods, err := l.deps.Auths.FindAll(l.ctx)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "list auth methods")
	}
	var list []dto.AuthMethodConfig
	for _, method := range methods {
		var item dto.AuthMethodConfig
		mapping.DeepCopy(&item, method)
		if method.Config != "" {
			if err := json.Unmarshal([]byte(method.Config), &item.Config); err != nil {
				return nil, xerr.Wrapf(err, xerr.ERROR, "decode stored %s config", method.Method)
			}
		}
		list = append(list, item)
	}
	return &dto.GetAuthMethodListResponse{List: list}, nil
}
