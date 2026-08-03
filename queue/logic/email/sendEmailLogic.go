package emailLogic

import (
	"bytes"
	"context"
	"encoding/json"
	"text/template"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/svc"
	"github.com/perfect-panel/server/pkg/email"
	"github.com/perfect-panel/server/queue/types"
)

type SendEmailLogic struct {
	svcCtx *svc.ServiceContext
}

func emailLogContent(emailType string, content map[string]interface{}) map[string]interface{} {
	if emailType == types.EmailTypeVerify {
		return map[string]interface{}{"redacted": true}
	}
	return content
}

func renderEmailTemplate(name, text string, data map[string]interface{}) (string, error) {
	tpl, err := template.New(name).Parse(text)
	if err != nil {
		return "", err
	}
	var result bytes.Buffer
	if err := tpl.Execute(&result, data); err != nil {
		return "", err
	}
	return result.String(), nil
}

// resolveSubject prefers the operator-configured subject over the fallback
// literal the producer queued. The configured subject renders with the same
// data as the body; if it fails to render it is still sent as raw text,
// because a localized subject with a template typo beats silently reverting
// to English.
func resolveSubject(ctx context.Context, configured, fallback string, data map[string]interface{}) string {
	if configured == "" {
		return fallback
	}
	rendered, err := renderEmailTemplate("subject", configured, data)
	if err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Execute subject template failed",
			logger.Field("error", err.Error()),
			logger.Field("subject", configured),
		)
		return configured
	}
	return rendered
}

func NewSendEmailLogic(svcCtx *svc.ServiceContext) *SendEmailLogic {
	return &SendEmailLogic{
		svcCtx: svcCtx,
	}
}
func (l *SendEmailLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	var payload types.SendEmailPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Unmarshal payload failed",
			logger.Field("error", err.Error()),
		)
		return nil
	}
	sender, err := email.NewSender(l.svcCtx.Config.Email.Platform, l.svcCtx.Config.Email.PlatformConfig, l.svcCtx.Config.Site.SiteName)
	if err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] NewSender failed", logger.Field("error", err.Error()))
		return nil
	}
	// The operator-configured subject of a typed notification wins over the
	// literal queued by the producer; it renders with the same data as the
	// body so subjects can interpolate {{.SiteName}} and friends.
	var content, bodyTemplate, subjectTemplate string
	switch payload.Type {
	case types.EmailTypeVerify:
		payload.Content["Type"] = uint8(payload.Content["Type"].(float64))
		bodyTemplate = l.svcCtx.Config.Email.VerifyEmailTemplate
		subjectTemplate = l.svcCtx.Config.Email.VerifyEmailSubject
	case types.EmailTypeMaintenance:
		bodyTemplate = l.svcCtx.Config.Email.MaintenanceEmailTemplate
		subjectTemplate = l.svcCtx.Config.Email.MaintenanceEmailSubject
	case types.EmailTypeExpiration:
		bodyTemplate = l.svcCtx.Config.Email.ExpirationEmailTemplate
		subjectTemplate = l.svcCtx.Config.Email.ExpirationEmailSubject
	case types.EmailTypeTrafficExceed:
		bodyTemplate = l.svcCtx.Config.Email.TrafficExceedEmailTemplate
		subjectTemplate = l.svcCtx.Config.Email.TrafficExceedEmailSubject
	case types.EmailTypeCustom:
		if payload.Content == nil {
			logger.WithContext(ctx).Error("[SendEmailLogic] Custom email content is empty")
			return nil
		}
		if tpl, ok := payload.Content["content"].(string); !ok {
			logger.WithContext(ctx).Error("[SendEmailLogic] Custom email content is not a string")
			return nil
		} else {
			content = tpl
		}
	default:
		logger.WithContext(ctx).Error("[SendEmailLogic] Unsupported email type",
			logger.Field("type", payload.Type),
		)
		return nil
	}
	if bodyTemplate != "" {
		content, err = renderEmailTemplate(payload.Type, bodyTemplate, payload.Content)
		if err != nil {
			logger.WithContext(ctx).Error("[SendEmailLogic] Execute template failed",
				logger.Field("error", err.Error()),
				logger.Field("template", bodyTemplate),
			)
			return nil
		}
	}
	subject := resolveSubject(ctx, subjectTemplate, payload.Subject, payload.Content)
	messageLog := log.Message{
		Platform: l.svcCtx.Config.Email.Platform,
		To:       payload.Email,
		Subject:  subject,
		Content:  emailLogContent(payload.Type, payload.Content),
	}

	err = sender.Send([]string{payload.Email}, subject, content)
	if err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Send email failed", logger.Field("error", err.Error()))
		return nil
	}
	messageLog.Status = 1
	emailLog, err := messageLog.Marshal()
	if err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Marshal message log failed",
			logger.Field("error", err.Error()),
		)
		return nil
	}

	if err = l.svcCtx.Store.Log().Insert(ctx, &log.SystemLog{
		Type:     log.TypeEmailMessage.Uint8(),
		Date:     timeutil.Now().Format("2006-01-02"),
		ObjectID: 0,
		Content:  string(emailLog),
	}); err != nil {
		logger.WithContext(ctx).Error("[SendEmailLogic] Insert email log failed",
			logger.Field("error", err.Error()),
		)
		return nil
	}
	return nil
}
