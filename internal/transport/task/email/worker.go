package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/perfect-panel/server/internal/infra/mail"
	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/platform/entity/task"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
	"github.com/perfect-panel/server/pkg/slicesx"
	"github.com/perfect-panel/server/pkg/timeutil"
)

// ErrorInfo is one failed delivery of a campaign, as the task records it.
type ErrorInfo struct {
	Error string `json:"error"`
	Email string `json:"email"`
	Time  int64  `json:"time"`
}

// DailyLimitReached asks the queue shell to persist a continuation for the
// next civil day instead of keeping one queue worker asleep for hours.
type DailyLimitReached struct {
	NextAt time.Time
}

// ErrTaskNotActive stops a worker whose task an administrator cancelled or
// another run finished; the queue shell treats it as done, not failed.
var ErrTaskNotActive = errors.New("batch email task is no longer active")

func (e *DailyLimitReached) Error() string {
	return fmt.Sprintf("batch email daily limit reached; resume at %s", e.NextAt.Format(time.RFC3339))
}

// Worker sends one campaign to its recipients, one at a time, recording the
// progress and every failure on the task so a later run resumes after the
// last recipient sent.
type Worker struct {
	id       int64
	tasks    TaskStore
	ctx      context.Context
	sender   mail.Sender
	logs     MessageLogStore
	platform string
}

// WorkerOption configures a Worker.
type WorkerOption func(*Worker)

// MessageLogStore is the message log a worker audits each delivery in.
type MessageLogStore interface {
	Insert(ctx context.Context, data *logEntity.SystemLog) error
	Update(ctx context.Context, data *logEntity.SystemLog) error
}

// WithMessageLogs audits each delivery in logs under the provider platform.
func WithMessageLogs(logs MessageLogStore, platform string) WorkerOption {
	return func(worker *Worker) {
		worker.logs = logs
		worker.platform = platform
	}
}

// NewWorker builds the worker of task id; ctx bounds the run.
func NewWorker(ctx context.Context, id int64, tasks TaskStore, sender mail.Sender, options ...WorkerOption) *Worker {
	worker := &Worker{id: id, tasks: tasks, ctx: ctx, sender: sender}
	for _, option := range options {
		if option != nil {
			option(worker)
		}
	}
	return worker
}

// GetID returns the id of the task the worker runs.
func (w *Worker) GetID() int64 { return w.id }

// batchRun is one run of a batch-email task: the task row, what it sends to
// whom, and the failures recorded so far.
type batchRun struct {
	task       *task.Task
	scope      task.EmailScope
	content    task.EmailContent
	recipients []string
	interval   time.Duration
	failures   []ErrorInfo
}

// Start processes a batch-email task until completion or cancellation.
func (w *Worker) Start() error {
	run, err := w.prepare()
	if err != nil || run == nil {
		return err
	}
	for index := int(run.task.Current); index < len(run.recipients); index++ {
		if err := w.sendNext(run, index); err != nil {
			return err
		}
	}
	return w.complete(run)
}

// prepare loads the task and marks it in progress. It returns no run when
// the task is already terminal, and records a task that cannot run (a corrupt
// scope or content, no recipients) as failed.
func (w *Worker) prepare() (*batchRun, error) {
	taskInfo, err := w.tasks.FindOneByType(w.ctx, w.id, task.TypeEmail)
	if err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to find task"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return nil, err
	}
	w.restoreRequestMetadata(taskInfo.Scope)
	switch taskInfo.Status {
	case task.StatusCompleted, task.StatusFailed, task.StatusCancelled, task.StatusEnqueueFailed:
		logger.WithContext(w.ctx).Info("Batch Send Email", logger.Field("message", "Task is already terminal"), logger.Field("task_id", w.id), logger.Field("status", taskInfo.Status))
		return nil, nil
	}

	run := &batchRun{task: taskInfo, interval: time.Second}
	if err := json.Unmarshal([]byte(taskInfo.Scope), &run.scope); err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to parse task scope"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return nil, w.failTask(taskInfo, fmt.Errorf("parse task scope: %w", err))
	}
	if len(run.scope.Recipients) == 0 && len(run.scope.Additional) == 0 {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "No recipients or additional emails provided"), logger.Field("task_id", w.id))
		return nil, w.failTask(taskInfo, fmt.Errorf("no recipients provided"))
	}
	// Migrate legacy in-scope counters on first resume; current tasks keep these
	// bounded fields in dedicated columns so progress updates never rewrite the
	// potentially very large recipient list.
	if taskInfo.DailyDate == "" && taskInfo.DailySent == 0 {
		taskInfo.DailyDate = run.scope.DailyDate
		taskInfo.DailySent = run.scope.DailySent
	} else {
		run.scope.DailyDate = taskInfo.DailyDate
		run.scope.DailySent = taskInfo.DailySent
	}
	if err := json.Unmarshal([]byte(taskInfo.Content), &run.content); err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to parse task content"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return nil, w.failTask(taskInfo, fmt.Errorf("parse task content: %w", err))
	}

	run.recipients = slicesx.RemoveDuplicateElements(append(run.scope.Recipients, run.scope.Additional...)...)
	if len(run.recipients) == 0 {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "No valid recipients found"), logger.Field("task_id", w.id))
		return nil, w.failTask(taskInfo, fmt.Errorf("no valid recipients found"))
	}
	if taskInfo.Current > uint64(len(run.recipients)) {
		return nil, w.failTask(taskInfo, fmt.Errorf("task progress exceeds recipient count"))
	}
	if run.scope.Interval != 0 {
		run.interval = time.Duration(run.scope.Interval) * time.Second
	}
	if run.failures, err = w.storedFailures(taskInfo); err != nil {
		return nil, err
	}
	// Tasks created before task_error was introduced keep their failure list
	// in the legacy column; it is read only as a migration fallback.
	if len(run.failures) == 0 && taskInfo.Errors != "" {
		if err := json.Unmarshal([]byte(taskInfo.Errors), &run.failures); err != nil {
			return nil, w.failTask(taskInfo, fmt.Errorf("parse task errors: %w", err))
		}
	}

	taskInfo.Status = task.StatusInProgress
	if err := w.persist(taskInfo, &run.scope); err != nil {
		return nil, err
	}
	return run, nil
}

// storedFailures reads the delivery failures the task recorded in the
// task_error table.
func (w *Worker) storedFailures(taskInfo *task.Task) ([]ErrorInfo, error) {
	stored, err := w.tasks.FindErrors(w.ctx, []int64{taskInfo.Id})
	if err != nil {
		return nil, fmt.Errorf("load task errors: %w", err)
	}
	failures := make([]ErrorInfo, 0, len(stored))
	for _, item := range stored {
		failures = append(failures, ErrorInfo{Error: item.Error, Email: item.Target, Time: item.OccurredAt})
	}
	return failures, nil
}

// sendNext sends the email of recipient index and records the progress,
// then waits the task's interval unless it was the last recipient.
func (w *Worker) sendNext(run *batchRun, index int) error {
	if err := w.ensureDailyCapacity(&run.scope, run.task); err != nil {
		return err
	}
	select {
	case <-w.ctx.Done():
		logger.WithContext(w.ctx).Info("Batch Send Email", logger.Field("message", "Worker stopped by context cancellation"), logger.Field("task_id", w.id))
		return w.ctx.Err()
	default:
	}

	recipient := run.recipients[index]
	audit, err := w.beginMessage()
	if err != nil {
		// Nothing has been delivered yet, so returning the error is safe and
		// lets the queue retry without duplicating an email.
		return err
	}
	sendErr := w.send(recipient, run.content)
	if errors.Is(sendErr, context.Canceled) || errors.Is(sendErr, context.DeadlineExceeded) {
		return sendErr
	}
	var failure *task.TaskError
	if sendErr != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to send email"), logger.Field("error", sendErr.Error()), logger.Field("task_id", w.id))
		occurredAt := timeutil.Now().Unix()
		failure = &task.TaskError{
			TaskId: run.task.Id, Position: uint64(index), Target: recipient,
			Error: sendErr.Error(), OccurredAt: occurredAt,
		}
		run.failures = append(run.failures, ErrorInfo{Error: sendErr.Error(), Email: recipient, Time: occurredAt})
	}
	w.finishMessage(audit, sendErr == nil)
	run.task.Current = uint64(index + 1)
	run.scope.DailySent++
	var persistErr error
	if failure != nil {
		persistErr = w.persistWithError(run.task, &run.scope, failure)
	} else {
		persistErr = w.persist(run.task, &run.scope)
	}
	if persistErr != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to update task progress"), logger.Field("error", persistErr.Error()), logger.Field("task_id", w.id))
		return persistErr
	}
	if index+1 < len(run.recipients) {
		return waitContext(w.ctx, run.interval)
	}
	return nil
}

// complete finishes the task: failed when every recipient failed, completed
// otherwise, with the failures kept in the legacy column for older readers.
func (w *Worker) complete(run *batchRun) error {
	taskInfo := run.task
	taskInfo.Status = task.StatusCompleted
	failedRecipients := make(map[string]struct{}, len(run.failures))
	for _, item := range run.failures {
		if item.Email != "" {
			failedRecipients[item.Email] = struct{}{}
		}
	}
	if len(failedRecipients) >= len(run.recipients) {
		taskInfo.Status = task.StatusFailed
	}
	if len(run.failures) > 0 {
		text, marshalErr := json.Marshal(run.failures)
		if marshalErr != nil {
			return w.failTask(taskInfo, fmt.Errorf("marshal task errors: %w", marshalErr))
		}
		taskInfo.Errors = string(text)
	}

	if err := w.persist(taskInfo, &run.scope); err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to finalize task"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return err
	}
	logger.WithContext(w.ctx).Info("Batch Send Email", logger.Field("message", "Task completed"), logger.Field("task_id", w.id), logger.Field("total_attempted", taskInfo.Current))
	return nil
}

func (w *Worker) restoreRequestMetadata(scopeJSON string) {
	var metadata requestmeta.Metadata
	if json.Unmarshal([]byte(scopeJSON), &metadata) != nil {
		return
	}
	w.ctx = requestmeta.With(w.ctx, metadata)
	w.ctx = logger.ContextWithRequestMetadata(w.ctx, metadata)
}

// beginMessage records the attempt to send one campaign email. The audit
// entry names no recipient: addresses stay out of the message log.
func (w *Worker) beginMessage() (*logEntity.SystemLog, error) {
	if w.logs == nil {
		return nil, nil
	}
	metadata, _ := requestmeta.From(w.ctx)
	message := logEntity.Message{
		Metadata: metadata,
		To:       logger.RedactedValue, Subject: "custom", Platform: w.platform,
		Content: map[string]any{"redacted": true, "email_type": "custom", "batch_task_id": w.id},
		Status:  0,
	}
	content, err := message.Marshal()
	if err != nil {
		return nil, err
	}
	audit := &logEntity.SystemLog{
		Type: logEntity.TypeEmailMessage.Uint8(), Date: timeutil.Now().Format("2006-01-02"), Content: string(content),
	}
	if err := w.logs.Insert(w.ctx, audit); err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to insert email log"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return nil, err
	}
	return audit, nil
}

func (w *Worker) finishMessage(audit *logEntity.SystemLog, sent bool) {
	if audit == nil || w.logs == nil {
		return
	}
	var message logEntity.Message
	if err := message.Unmarshal([]byte(audit.Content)); err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to parse pending email log"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return
	}
	message.Status = 2
	if sent {
		message.Status = 1
	}
	content, err := message.Marshal()
	if err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to finalize email log"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return
	}
	audit.Content = string(content)
	if err := w.logs.Update(w.ctx, audit); err != nil {
		logger.WithContext(w.ctx).Error("Batch Send Email", logger.Field("message", "Failed to update email log"), logger.Field("error", err.Error()), logger.Field("task_id", w.id), logger.Field("log_id", audit.Id))
	}
}

func (w *Worker) send(recipient string, content task.EmailContent) error {
	select {
	case sendOne <- struct{}{}:
		defer func() { <-sendOne }()
	case <-w.ctx.Done():
		return w.ctx.Err()
	}
	// Stopping the worker cancels w.ctx, which ends a delivery in flight.
	return w.sender.SendContext(w.ctx, []string{recipient}, content.Subject, content.Content)
}

func (w *Worker) persist(taskInfo *task.Task, scope *task.EmailScope) error {
	taskInfo.DailyDate = scope.DailyDate
	taskInfo.DailySent = scope.DailySent
	updated, err := w.tasks.UpdateActiveProgress(w.ctx, taskInfo)
	if err != nil {
		return err
	}
	if !updated {
		return ErrTaskNotActive
	}
	return nil
}

func (w *Worker) persistWithError(taskInfo *task.Task, scope *task.EmailScope, failure *task.TaskError) error {
	taskInfo.DailyDate = scope.DailyDate
	taskInfo.DailySent = scope.DailySent
	updated, err := w.tasks.UpdateActiveProgressWithError(w.ctx, taskInfo, failure)
	if err != nil {
		return err
	}
	if !updated {
		return ErrTaskNotActive
	}
	return nil
}

func (w *Worker) failTask(taskInfo *task.Task, cause error) error {
	taskInfo.Status = task.StatusFailed
	taskInfo.Errors = cause.Error()
	updated, err := w.tasks.UpdateActiveProgress(w.ctx, taskInfo)
	if err != nil {
		return fmt.Errorf("record failed email task: %w", err)
	}
	if !updated {
		return ErrTaskNotActive
	}
	return nil
}

func (w *Worker) ensureDailyCapacity(scope *task.EmailScope, taskInfo *task.Task) error {
	if scope.Limit == 0 {
		return nil
	}
	now := timeutil.Now()
	today := now.Format(time.DateOnly)
	if scope.DailyDate != today {
		scope.DailyDate = today
		scope.DailySent = 0
		return w.persist(taskInfo, scope)
	}
	if scope.DailySent < scope.Limit {
		return nil
	}
	tomorrow := now.AddDate(0, 0, 1)
	nextDay := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, tomorrow.Location())
	return &DailyLimitReached{NextAt: nextDay}
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
