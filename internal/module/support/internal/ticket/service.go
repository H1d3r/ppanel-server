// Package ticket implements the ticket subdomain of the support module. Only
// the module facade (internal/module/support) may reach it.
package ticket

import (
	"context"
	"net/url"
	"strings"

	"github.com/perfect-panel/server/internal/infra/mapping"
	"github.com/perfect-panel/server/internal/infra/requestctx"
	"github.com/perfect-panel/server/internal/module/identity/entity/user"
	dto "github.com/perfect-panel/server/internal/module/support/contract"
	entity "github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/xerr"
	"github.com/pkg/errors"
)

// Notifier mirrors ticket lifecycle into other channels — today the forum
// topic in the Telegram admin group. Implementations are best-effort by
// contract: they never fail the ticket operation, so the methods return
// nothing and do their own logging.
type Notifier interface {
	TicketCreated(ctx context.Context, t *entity.Ticket)
	TicketReplied(ctx context.Context, ticketID int64, from, content string)
	TicketStatusChanged(ctx context.Context, ticketID int64, status uint8)
}

type Service struct {
	repo    repository.TicketRepo
	notify  Notifier
	limiter CreationLimiter
}

// NewService builds the ticket service; notify may be nil when no mirror
// channel is wired, limiter nil when ticket creation is not rate limited.
func NewService(repo repository.TicketRepo, notify Notifier, limiter CreationLimiter) *Service {
	return &Service{repo: repo, notify: notify, limiter: limiter}
}

func currentUser(ctx context.Context) (*user.User, error) {
	u, ok := ctx.Value(requestctx.CtxKeyUser).(*user.User)
	if !ok {
		logger.Error("current user is not found in context")
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "Invalid Access")
	}
	return u, nil
}

// CreateFollow appends an admin reply and flips the ticket back to Waiting.
func (s *Service) CreateFollow(ctx context.Context, req *dto.CreateTicketFollowRequest) error {
	if _, err := s.repo.FindOne(ctx, req.TicketId); err != nil {
		logger.WithContext(ctx).Errorw("[CreateTicketFollow] FindOne error", logger.Field("error", err.Error()), logger.Field("ticketId", req.TicketId))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "find ticket failed: %v", err.Error())
	}
	if err := s.repo.InsertTicketFollow(ctx, &entity.Follow{
		TicketId: req.TicketId,
		From:     req.From,
		Type:     req.Type,
		Content:  req.Content,
	}); err != nil {
		logger.WithContext(ctx).Errorw("[CreateTicketFollow] Database insert error", logger.Field("error", err.Error()), logger.Field("request", req))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create ticket follow failed: %v", err.Error())
	}
	if err := s.repo.UpdateTicketStatus(ctx, req.TicketId, 0, entity.Waiting); err != nil {
		logger.WithContext(ctx).Errorw("[CreateTicketFollow] Database update error", logger.Field("error", err.Error()), logger.Field("status", entity.Waiting))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket status failed: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketReplied(ctx, req.TicketId, req.From, req.Content)
	}
	return nil
}

func (s *Service) List(ctx context.Context, req *dto.GetTicketListRequest) (*dto.GetTicketListResponse, error) {
	total, list, err := s.repo.QueryTicketList(ctx, int(req.Page), int(req.Size), req.UserId, req.Status, req.Search)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetTicketList] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryTicketList error: %v", err)
	}
	resp := &dto.GetTicketListResponse{
		Total: total,
		List:  make([]dto.Ticket, 0),
	}
	mapping.DeepCopy(&resp.List, list)
	return resp, nil
}

func (s *Service) GetDetail(ctx context.Context, req *dto.GetTicketRequest) (*dto.Ticket, error) {
	data, err := s.repo.QueryTicketDetail(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetTicket] Query Database Error: ", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get ticket detail failed: %v", err.Error())
	}
	resp := &dto.Ticket{}
	mapping.DeepCopy(resp, data)
	return resp, nil
}

func (s *Service) UpdateStatus(ctx context.Context, req *dto.UpdateTicketStatusRequest) error {
	if err := s.repo.UpdateTicketStatus(ctx, req.Id, 0, *req.Status); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateTicketStatus] Update Database Error: ", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket error: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketStatusChanged(ctx, req.Id, *req.Status)
	}
	return nil
}

func (s *Service) CreateUserTicket(ctx context.Context, req *dto.CreateUserTicketRequest) error {
	u, err := currentUser(ctx)
	if err != nil {
		return err
	}
	if s.limiter != nil {
		allowed, err := s.limiter.Allow(ctx, u.Id)
		switch {
		case err != nil:
			// Fail open: the limit protects staff from floods, not the
			// ticket desk from a Redis outage.
			logger.WithContext(ctx).Errorw("[CreateUserTicket] rate limit check failed", logger.Field("error", err.Error()), logger.Field("user_id", u.Id))
		case !allowed:
			return errors.Wrapf(xerr.NewErrCode(xerr.TooManyRequests), "ticket creation limit exceeded for user %d", u.Id)
		}
	}
	// Insert backfills the id, which the mirror channel needs for its topic.
	t := &entity.Ticket{
		Title:       req.Title,
		Description: req.Description,
		UserId:      u.Id,
		Status:      entity.Pending,
	}
	if err := s.repo.Insert(ctx, t); err != nil {
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "insert ticket error: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketCreated(ctx, t)
	}
	return nil
}

// CreateUserFollow appends a user reply after verifying ticket ownership and
// flips the ticket to Pending. The author is always the ticket owner: a
// client-supplied "from" would let a user post what renders as a staff reply.
func (s *Service) CreateUserFollow(ctx context.Context, req *dto.CreateUserTicketFollowRequest) error {
	u, err := currentUser(ctx)
	if err != nil {
		return err
	}
	followType, err := userFollowType(req.Type, req.Content)
	if err != nil {
		return err
	}
	t, err := s.repo.FindOne(ctx, req.TicketId)
	if err != nil {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Database query error", logger.Field("error", err.Error()), logger.Field("request", req))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query ticket failed: %v", err.Error())
	}
	if u.Id != t.UserId {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Invalid access", logger.Field("user_id", u.Id), logger.Field("ticket_user_id", t.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "invalid access")
	}
	if err := s.repo.InsertTicketFollow(ctx, &entity.Follow{
		TicketId: req.TicketId,
		From:     entity.FromUser,
		Type:     followType,
		Content:  req.Content,
	}); err != nil {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Database insert error", logger.Field("error", err.Error()), logger.Field("request", req))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseInsertError), "create ticket follow failed: %v", err.Error())
	}
	if err := s.repo.UpdateTicketStatus(ctx, req.TicketId, u.Id, entity.Pending); err != nil {
		logger.WithContext(ctx).Errorw("[CreateUserTicketFollow] Database update error", logger.Field("error", err.Error()), logger.Field("status", entity.Pending))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket status failed: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketReplied(ctx, req.TicketId, entity.FromUser, req.Content)
	}
	return nil
}

// rasterDataURLPrefixes are the inline image encodings a user may attach: the
// web client re-encodes uploads to WebP and sends them as base64 data URLs.
// SVG is excluded because it is a document that can carry script.
var rasterDataURLPrefixes = []string{
	"data:image/webp;base64,",
	"data:image/png;base64,",
	"data:image/jpeg;base64,",
	"data:image/gif;base64,",
}

// userFollowType validates what a ticket owner may post: text, or an image
// the clients render as <img src> — an inline raster data URL or an absolute
// http(s) URL. An omitted type is text, as the column default stores it.
func userFollowType(followType uint8, content string) (uint8, error) {
	switch followType {
	case 0, entity.FollowText:
		return entity.FollowText, nil
	case entity.FollowImage:
		if !isImageReference(content) {
			return 0, errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "image follow must be an inline raster image or an http(s) URL")
		}
		return entity.FollowImage, nil
	}
	return 0, errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "unsupported follow type %d", followType)
}

func isImageReference(content string) bool {
	for _, prefix := range rasterDataURLPrefixes {
		if len(content) > len(prefix) && strings.EqualFold(content[:len(prefix)], prefix) {
			return true
		}
	}
	u, err := url.Parse(content)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (s *Service) GetUserDetail(ctx context.Context, req *dto.GetUserTicketDetailRequest) (*dto.Ticket, error) {
	data, err := s.repo.QueryTicketDetail(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetUserTicketDetailsLogic] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "get ticket detail failed: %v", err.Error())
	}
	u, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	if data.UserId != u.Id {
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "invalid access")
	}
	resp := &dto.Ticket{}
	mapping.DeepCopy(resp, data)
	return resp, nil
}

func (s *Service) GetUserList(ctx context.Context, req *dto.GetUserTicketListRequest) (*dto.GetUserTicketListResponse, error) {
	u, err := currentUser(ctx)
	if err != nil {
		return nil, err
	}
	logger.WithContext(ctx).Debugf("Current user: %v", u.Id)
	total, list, err := s.repo.QueryTicketList(ctx, req.Page, req.Size, u.Id, req.Status, req.Search)
	if err != nil {
		logger.WithContext(ctx).Errorw("[GetUserTicketListLogic] Database Error", logger.Field("error", err.Error()))
		return nil, errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "QueryTicketList error: %v", err)
	}
	resp := &dto.GetUserTicketListResponse{
		Total: total,
		List:  make([]dto.Ticket, 0),
	}
	mapping.DeepCopy(&resp.List, list)
	return resp, nil
}

// UpdateUserStatus lets a user close their own ticket. Closing is the only
// status change the product offers users: Waiting and Processed are staff
// decisions, and a reply is what moves a ticket back to Pending. Ownership is
// checked before anything changes, because the status mirror reaches the
// Telegram admin group.
func (s *Service) UpdateUserStatus(ctx context.Context, req *dto.UpdateUserTicketStatusRequest) error {
	u, err := currentUser(ctx)
	if err != nil {
		return err
	}
	if req.Status == nil || *req.Status != entity.Closed {
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidParams), "users may only close tickets")
	}
	t, err := s.repo.FindOne(ctx, req.Id)
	if err != nil {
		logger.WithContext(ctx).Errorw("[UpdateUserTicketStatusLogic] Database query error", logger.Field("error", err.Error()), logger.Field("ticket_id", req.Id))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseQueryError), "query ticket failed: %v", err.Error())
	}
	if t.UserId != u.Id {
		logger.WithContext(ctx).Errorw("[UpdateUserTicketStatusLogic] Invalid access", logger.Field("user_id", u.Id), logger.Field("ticket_user_id", t.UserId))
		return errors.Wrapf(xerr.NewErrCode(xerr.InvalidAccess), "invalid access")
	}
	if err := s.repo.UpdateTicketStatus(ctx, req.Id, u.Id, entity.Closed); err != nil {
		logger.WithContext(ctx).Errorw("[UpdateUserTicketStatusLogic] Database Error", logger.Field("error", err.Error()))
		return errors.Wrapf(xerr.NewErrCode(xerr.DatabaseUpdateError), "update ticket error: %v", err.Error())
	}
	if s.notify != nil {
		s.notify.TicketStatusChanged(ctx, req.Id, entity.Closed)
	}
	return nil
}
