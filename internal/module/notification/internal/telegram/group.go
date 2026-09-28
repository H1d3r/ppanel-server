package telegram

import (
	"strconv"

	"github.com/go-telegram/bot/models"
	"github.com/perfect-panel/server/internal/module/notification/entity/telegramtopic"
	"github.com/perfect-panel/server/internal/module/support/entity/ticket"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/pkg/errors"
	"gorm.io/gorm"
)

// handleGroup processes one message inside the admin group: topic lifecycle
// service messages, administrator commands, and replies inside support or
// ticket topics.
func (l *TelegramLogic) handleGroup(msg *models.Message) {
	if msg.ForumTopicClosed != nil || msg.ForumTopicReopened != nil {
		l.syncTopicLifecycle(msg)
		return
	}
	if msg.From == nil || msg.From.IsBot {
		return
	}
	if cmd := messageCommand(msg); cmd != "" {
		if isAdminCommand(cmd) {
			l.deps.Admin.Handle(msg)
		}
		return
	}
	if msg.MessageThreadID == 0 || l.deps.Topics == nil {
		return
	}
	topic, err := l.deps.Topics.FindByThread(l.ctx, msg.Chat.ID, int64(msg.MessageThreadID))
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			l.Errorw("group relay: topic lookup failed", logger.Field("error", err.Error()))
		}
		return
	}
	// Only support and ticket topics carry a user conversation; chatter in
	// the notification feed (or any future kind) is none of the bot's
	// business.
	if topic.Kind != telegramtopic.KindSupport && topic.Kind != telegramtopic.KindTicket {
		return
	}
	// Whatever staff write in a mapped topic reaches a customer, so it
	// carries the same authority as an administrator command.
	if reject := l.rejectNonAdminSender(msg, "回复未送达用户"); reject != "" {
		l.rejectAloud(msg, reject)
		return
	}
	switch topic.Kind {
	case telegramtopic.KindSupport:
		l.relayAdminReply(msg, topic)
	case telegramtopic.KindTicket:
		l.appendTicketFollow(msg, topic)
	}
}

// rejectNonAdminSender enforces panel-administrator identity for actions
// taken inside mapped topics: the sender must have their own Telegram bound
// to an enabled, non-deleted administrator account. The sender would
// otherwise reasonably believe the action took effect, so a rejection is
// always spoken, never silent; consequence names what did not happen.
func (l *TelegramLogic) rejectNonAdminSender(msg *models.Message, consequence string) string {
	if l.deps.Users == nil || l.deps.UserAuth == nil {
		return "系统未配置，" + consequence + "。"
	}
	if msg.From == nil {
		return "⚠️ 无法识别发送者，" + consequence + "。"
	}
	auth, err := l.deps.UserAuth.FindUserAuthMethodByOpenID(l.ctx, "telegram", strconv.FormatInt(msg.From.ID, 10))
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "⚠️ 您的 Telegram 未绑定管理员账号，" + consequence + "。"
		}
		l.Errorw("group relay: sender lookup failed", logger.Field("error", err.Error()))
		return "系统错误，" + consequence + "。"
	}
	u, err := l.deps.Users.FindOne(l.ctx, auth.UserId)
	if err != nil {
		l.Errorw("group relay: sender user lookup failed", logger.Field("error", err.Error()))
		return "系统错误，" + consequence + "。"
	}
	if refusal := panelAdminRefusal(u); refusal != "" {
		l.Infow("group relay: sender may not administer", logger.Field("user_id", u.Id), logger.Field("reason", refusal))
		return "⚠️ 您不是管理员，" + consequence + "。"
	}
	return ""
}

// rejectAloud posts a rejection into the topic the sender acted in. It is
// rate-limited per sender: an unbound member flooding a topic must not make
// the bot flood it too.
func (l *TelegramLogic) rejectAloud(msg *models.Message, reject string) {
	if l.deps.Limiter != nil && msg.From != nil {
		if allowed, _ := l.deps.Limiter.Allow(l.ctx, msg.From.ID); !allowed {
			return
		}
	}
	_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), reject)
}

// relayAdminReply copies a staff message from a support topic to the bound
// user's private chat, hiding which administrator wrote it.
func (l *TelegramLogic) relayAdminReply(msg *models.Message, topic *telegramtopic.Topic) {
	method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "telegram", topic.RefId)
	if err != nil {
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "⚠️ 该用户已解绑 Telegram，消息无法送达。")
		return
	}
	customerChat, err := strconv.ParseInt(method.AuthIdentifier, 10, 64)
	if err != nil {
		l.Errorw("support relay: malformed chat id", logger.Field("value", method.AuthIdentifier))
		return
	}
	if err := l.deps.TopicClient.CopyTo(l.ctx, customerChat, msg.Chat.ID, msg.ID); err != nil {
		l.Errorw("support relay: copy to user failed", logger.Field("error", err.Error()), logger.Field("user_id", topic.RefId))
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "⚠️ 回复送达失败，用户可能已停用 Bot。")
	}
}

// appendTicketFollow records a staff message in a ticket topic as an
// administrator reply, exactly like the /rp command, so the website thread
// and the topic stay one conversation.
func (l *TelegramLogic) appendTicketFollow(msg *models.Message, topic *telegramtopic.Topic) {
	if l.deps.Tickets == nil {
		return
	}
	if msg.Text == "" {
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "工单回复目前仅支持文本消息。")
		return
	}
	if err := l.deps.Tickets.InsertTicketFollow(l.ctx, &ticket.Follow{
		TicketId: topic.RefId,
		From:     "admin",
		Type:     1,
		Content:  msg.Text,
	}); err != nil {
		l.Errorw("ticket relay: insert follow failed", logger.Field("error", err.Error()), logger.Field("ticket_id", topic.RefId))
		_ = l.deps.Messenger.Send(msg.Chat.ID, int64(msg.MessageThreadID), "⚠️ 回复保存失败，请稍后再试。")
		return
	}
	if err := l.deps.Tickets.UpdateTicketStatus(l.ctx, topic.RefId, 0, ticket.Waiting); err != nil {
		l.Errorw("ticket relay: status update failed", logger.Field("error", err.Error()), logger.Field("ticket_id", topic.RefId))
	}
}

// syncTopicLifecycle mirrors a HUMAN closing or reopening a topic inside
// Telegram back onto the mapping and, for ticket topics, the ticket itself.
// The bot's own Close/Reopen calls emit the same service messages (with the
// bot as From) and must be ignored: syncing them back would overwrite the
// status the website side just wrote — e.g. an automatic reopen while
// posting a website reply would flip the ticket from Waiting to Pending.
//
// Any group member with the manage-topics right can close a topic, so the
// effects beyond the mapping — the ticket status and the customer notice —
// require the same panel-administrator identity as a topic reply.
func (l *TelegramLogic) syncTopicLifecycle(msg *models.Message) {
	if msg.From != nil && msg.From.IsBot {
		return
	}
	if msg.MessageThreadID == 0 || l.deps.Topics == nil {
		return
	}
	topic, err := l.deps.Topics.FindByThread(l.ctx, msg.Chat.ID, int64(msg.MessageThreadID))
	if err != nil {
		return
	}
	closed := msg.ForumTopicClosed != nil
	status := uint8(telegramtopic.StatusActive)
	if closed {
		status = telegramtopic.StatusClosed
	}
	// The mapping records the topic's actual state in Telegram, whoever
	// changed it, so the relays keep reopening it when needed.
	if err := l.deps.Topics.UpdateStatus(l.ctx, topic.Id, status); err != nil {
		l.Errorw("topic lifecycle: mapping update failed", logger.Field("error", err.Error()))
	}
	switch {
	case topic.Kind == telegramtopic.KindTicket && l.deps.Tickets != nil:
		if reject := l.rejectNonAdminSender(msg, "工单状态未同步"); reject != "" {
			l.rejectAloud(msg, reject)
			return
		}
		ticketStatus := uint8(ticket.Pending)
		if closed {
			ticketStatus = ticket.Closed
		}
		if err := l.deps.Tickets.UpdateTicketStatus(l.ctx, topic.RefId, 0, ticketStatus); err != nil {
			l.Errorw("topic lifecycle: ticket status sync failed", logger.Field("error", err.Error()), logger.Field("ticket_id", topic.RefId))
		}
	case topic.Kind == telegramtopic.KindSupport && closed:
		if reject := l.rejectNonAdminSender(msg, "用户未收到会话结束通知"); reject != "" {
			l.rejectAloud(msg, reject)
			return
		}
		// Best effort: tell the customer the conversation ended.
		if method, err := l.deps.UserAuth.FindUserAuthMethodByUserId(l.ctx, "telegram", topic.RefId); err == nil {
			if chatID, perr := strconv.ParseInt(method.AuthIdentifier, 10, 64); perr == nil {
				_ = l.sendMessage("本次客服会话已结束。如需帮助，直接发送消息即可重新开启。", chatID)
			}
		}
	}
}
