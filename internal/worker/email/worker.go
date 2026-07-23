package emailworker

import (
	"context"
	"encoding/json"
	"time"

	"github.com/perfect-panel/server/internal/model/entity/task"
	emailpkg "github.com/perfect-panel/server/pkg/email"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/tool"
)

type ErrorInfo struct {
	Error string `json:"error"`
	Email string `json:"email"`
	Time  int64  `json:"time"`
}

type Worker struct {
	id     int64
	tasks  TaskStore
	ctx    context.Context
	sender emailpkg.Sender
	status uint8
}

func NewWorker(ctx context.Context, id int64, tasks TaskStore, sender emailpkg.Sender) *Worker {
	return &Worker{id: id, tasks: tasks, ctx: ctx, sender: sender}
}

func (w *Worker) GetID() int64 { return w.id }

func (w *Worker) IsRunning() uint8 { return w.status }

// Start processes a batch-email task until completion or cancellation.
func (w *Worker) Start() {
	limit.Lock()
	defer limit.Unlock()
	taskInfo, err := w.tasks.FindOne(w.ctx, w.id)
	if err != nil {
		logger.Error("Batch Send Email", logger.Field("message", "Failed to find task"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return
	}
	if taskInfo.Status != 0 {
		logger.Error("Batch Send Email", logger.Field("message", "Task already completed or in progress"), logger.Field("task_id", w.id))
		return
	}

	var scope task.EmailScope
	if err := json.Unmarshal([]byte(taskInfo.Scope), &scope); err != nil {
		logger.Error("Batch Send Email", logger.Field("message", "Failed to parse task scope"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return
	}
	if len(scope.Recipients) == 0 && len(scope.Additional) == 0 {
		logger.Error("Batch Send Email", logger.Field("message", "No recipients or additional emails provided"), logger.Field("task_id", w.id))
		return
	}

	var content task.EmailContent
	if err := json.Unmarshal([]byte(taskInfo.Content), &content); err != nil {
		logger.Error("Batch Send Email", logger.Field("message", "Failed to parse task content"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return
	}

	w.status = 1
	recipients := tool.RemoveDuplicateElements(append(scope.Recipients, scope.Additional...)...)
	if len(recipients) == 0 {
		logger.Error("Batch Send Email", logger.Field("message", "No valid recipients found"), logger.Field("task_id", w.id))
		w.status = 2
		return
	}

	interval := time.Second
	if scope.Interval != 0 {
		interval = time.Duration(scope.Interval) * time.Second
	}

	var errors []ErrorInfo
	var count uint64
	for _, recipient := range recipients {
		select {
		case <-w.ctx.Done():
			logger.Info("Batch Send Email", logger.Field("message", "Worker stopped by context cancellation"), logger.Field("task_id", w.id))
			return
		default:
		}
		if taskInfo.Status == 0 {
			taskInfo.Status = 1
		}

		if err := w.sender.Send([]string{recipient}, content.Subject, content.Content); err != nil {
			logger.Error("Batch Send Email", logger.Field("message", "Failed to send email"), logger.Field("error", err.Error()), logger.Field("recipient", recipient), logger.Field("task_id", w.id))
			errors = append(errors, ErrorInfo{Error: err.Error(), Email: recipient, Time: time.Now().Unix()})
			text, _ := json.Marshal(errors)
			taskInfo.Errors = string(text)
		}
		count++
		taskInfo.Current = count
		if err := w.tasks.Update(w.ctx, taskInfo); err != nil {
			logger.Error("Batch Send Email", logger.Field("message", "Failed to update task progress"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
			errors = append(errors, ErrorInfo{Error: err.Error(), Email: recipient, Time: time.Now().Unix()})
			w.status = 2
		}
		time.Sleep(interval)
	}
	taskInfo.Status = 2
	w.status = 2

	if err := w.tasks.Update(w.ctx, taskInfo); err != nil {
		logger.Error("Batch Send Email", logger.Field("message", "Failed to finalize task"), logger.Field("error", err.Error()), logger.Field("task_id", w.id))
		return
	}
	logger.Info("Batch Send Email", logger.Field("message", "Task completed successfully"), logger.Field("task_id", w.id), logger.Field("total_sent", count))
}
