package auditlog

import (
	"context"
	"strconv"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
)

// FilterSubscribeLog pages a user's subscription fetches, optionally those of
// one subscription.
func (s *Service) FilterSubscribeLog(ctx context.Context, req *dto.FilterSubscribeLogRequest) (*dto.FilterSubscribeLogResponse, error) {
	params := filterParams(log.TypeSubscribe, req.UserId, req.FilterLogParams)
	// The subscription log is searched by subscription only.
	params.Search = ""
	if req.UserSubscribeId != 0 {
		params.Search = `"user_subscribe_id":` + strconv.FormatInt(req.UserSubscribeId, 10)
	}
	total, list, err := logPage(ctx, s.deps.Logs, "subscription", params, func(row *log.SystemLog, content *log.Subscribe) dto.SubscribeLog {
		return withRequestMetadata(&dto.SubscribeLog{
			UserId:          row.ObjectID,
			Token:           content.Token,
			UserSubscribeId: content.UserSubscribeId,
			Timestamp:       row.CreatedAt.UnixMilli(),
		}, content.Request())
	})
	if err != nil {
		return nil, err
	}
	return &dto.FilterSubscribeLogResponse{Total: total, List: list}, nil
}
