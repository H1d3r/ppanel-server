package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/mail"
	taskEntity "github.com/perfect-panel/server/internal/module/platform/entity/task"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

// BatchEmailHandler runs a marketing email campaign, the batch task an
// administrator created. The task row in the platform kernel's bookkeeping
// carries the campaign and its progress, so a campaign that reaches the
// daily sending limit continues from a follow-up task the next day, and a
// retried delivery resumes where the last run stopped.
type BatchEmailHandler struct {
	deps Dependencies
	// senders keeps the provider client between campaigns; it is rebuilt
	// when the email configuration changes.
	senders mail.Senders
}

// NewBatchEmailHandler builds the handler over the task bookkeeping, the
// message log, the queue and the runtime email settings.
func NewBatchEmailHandler(deps Dependencies) *BatchEmailHandler {
	return &BatchEmailHandler{
		deps: deps,
	}
}

func (h *BatchEmailHandler) ProcessTask(ctx context.Context, task *asynq.Task) error {
	payload := task.Payload()
	if len(payload) == 0 {
		logger.WithContext(ctx).Error("[BatchEmail] ProcessTask failed: empty payload")
		return asynq.SkipRetry
	}
	taskID, err := strconv.ParseInt(string(payload), 10, 64)
	if err != nil {
		logger.WithContext(ctx).Error("[BatchEmail] ProcessTask failed: invalid task ID",
			logger.Field("error", err.Error()),
			logger.Field("payload", string(payload)),
		)
		return asynq.SkipRetry
	}
	if h.deps.Tasks == nil {
		return errors.New("batch email task store is nil")
	}
	taskInfo, err := h.deps.Tasks.FindOneByType(ctx, taskID, taskEntity.TypeEmail)
	if err != nil {
		return h.handleFailure(ctx, taskID, err)
	}
	if taskInfo.Status == taskEntity.StatusCompleted || taskInfo.Status == taskEntity.StatusCancelled || taskInfo.Status == taskEntity.StatusEnqueueFailed ||
		(taskInfo.Status == taskEntity.StatusFailed && taskInfo.Current >= taskInfo.Total) {
		return nil
	}
	if taskInfo.Status == taskEntity.StatusFailed {
		updated, err := h.deps.Tasks.UpdateStatusFrom(ctx, taskID, taskEntity.TypeEmail, []int8{taskEntity.StatusFailed}, taskEntity.StatusPending)
		if err != nil {
			return err
		}
		if !updated {
			return nil
		}
	}
	if h.deps.Email == nil || h.deps.SiteName == nil {
		return h.handleFailure(ctx, taskID, errors.New("batch email runtime configuration is unavailable"))
	}
	sender, err := h.senders.Get(h.deps.Email().Platform, h.deps.Email().PlatformConfig, h.deps.SiteName())
	if err != nil {
		logger.WithContext(ctx).Error("[BatchEmail] NewSender failed", logger.Field("error", err.Error()))
		return h.handleFailure(ctx, taskID, err)
	}
	manager := NewWorkerManager()
	if manager == nil {
		logger.WithContext(ctx).Error("[BatchEmail] ProcessTask failed: worker manager is nil")
		return asynq.SkipRetry
	}

	err = manager.RunWorker(ctx, taskID, h.deps.Tasks, sender,
		WithMessageLogs(h.deps.Logs, h.deps.Email().Platform))
	if errors.Is(err, ErrTaskNotActive) {
		return nil
	}
	var dailyLimit *DailyLimitReached
	if !errors.As(err, &dailyLimit) {
		return h.handleFailure(ctx, taskID, err)
	}
	if h.deps.Queue == nil {
		return errors.New("batch email continuation queue is nil")
	}
	continuation := asynq.NewTask(task.Type(), task.Payload())
	continuationID := fmt.Sprintf("marketing-email-%d-%s", taskID, dailyLimit.NextAt.Format("20060102"))
	_, enqueueErr := h.deps.Queue.EnqueueContext(ctx, continuation, asynq.ProcessAt(dailyLimit.NextAt), asynq.TaskID(continuationID))
	if errors.Is(enqueueErr, asynq.ErrTaskIDConflict) {
		return nil
	}
	return h.handleFailure(ctx, taskID, enqueueErr)
}

// handleFailure returns cause for asynq to retry the campaign. On the last
// attempt it also marks the task failed and appends cause to the task's
// recorded errors, so the administrator sees why the campaign stopped.
func (h *BatchEmailHandler) handleFailure(ctx context.Context, taskID int64, cause error) error {
	if cause == nil {
		return nil
	}
	retried, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxOK := asynq.GetMaxRetry(ctx)
	if !retryOK || !maxOK || retried < maxRetry {
		return cause
	}
	if h.deps.Tasks == nil {
		return cause
	}
	data, err := h.deps.Tasks.FindOneByType(ctx, taskID, taskEntity.TypeEmail)
	if err != nil {
		return errors.Join(cause, err)
	}
	data.Status = taskEntity.StatusFailed
	var taskErrors []ErrorInfo
	if data.Errors != "" {
		if unmarshalErr := json.Unmarshal([]byte(data.Errors), &taskErrors); unmarshalErr != nil {
			taskErrors = append(taskErrors, ErrorInfo{Error: data.Errors, Time: timeutil.Now().Unix()})
		}
	}
	taskErrors = append(taskErrors, ErrorInfo{Error: cause.Error(), Time: timeutil.Now().Unix()})
	encoded, marshalErr := json.Marshal(taskErrors)
	if marshalErr != nil {
		return errors.Join(cause, marshalErr)
	}
	data.Errors = string(encoded)
	if _, err := h.deps.Tasks.UpdateActive(ctx, data); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
