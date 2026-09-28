package adminserver

import (
	"context"

	dto "github.com/perfect-panel/server/internal/module/network/contract"
	"github.com/perfect-panel/server/internal/module/network/internal/protocolmap"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

type GetServerProtocolsLogic struct {
	logger.Logger
	ctx  context.Context
	deps Deps
}

// Get Server Protocols
func newGetServerProtocolsLogic(ctx context.Context, deps Deps) *GetServerProtocolsLogic {
	return &GetServerProtocolsLogic{
		Logger: logger.WithContext(ctx),
		ctx:    ctx,
		deps:   deps,
	}
}

func (l *GetServerProtocolsLogic) GetServerProtocols(req *dto.GetServerProtocolsRequest) (resp *dto.GetServerProtocolsResponse, err error) {
	// find server
	data, err := l.deps.Store.Node().FindOneServer(l.ctx, req.Id)
	if err != nil {
		l.Errorf("[GetServerProtocols] FindOneServer Error: %s", err.Error())
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "[GetServerProtocols] FindOneServer Error: %s", err.Error())
	}

	dst, err := data.UnmarshalProtocols()
	if err != nil {
		l.Errorf("[GetServerProtocols] UnmarshalProtocols Error: %s", err.Error())
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "unmarshal protocols of server %d", req.Id)
	}
	protocols, err := protocolmap.ToDTO(dst)
	if err != nil {
		return nil, xerr.Wrapf(err, xerr.DatabaseQueryError, "map protocols of server %d", req.Id)
	}

	return &dto.GetServerProtocolsResponse{
		Protocols: protocols,
	}, nil
}
