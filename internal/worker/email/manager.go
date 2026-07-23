package emailworker

import (
	"context"
	"sync"
	"time"

	"github.com/perfect-panel/server/internal/model/entity/task"
	emailpkg "github.com/perfect-panel/server/pkg/email"
	"github.com/perfect-panel/server/pkg/logger"
)

// TaskStore is the minimal task repository interface needed by the batch-email worker.
type TaskStore interface {
	FindOne(ctx context.Context, id int64) (*task.Task, error)
	Update(ctx context.Context, data *task.Task) error
}

var (
	Manager *WorkerManager
	once    sync.Once
	limit   sync.RWMutex
)

// WorkerManager owns asynchronous batch-email workers. It is an application
// worker, so it belongs in internal rather than the reusable email package.
type WorkerManager struct {
	tasks   TaskStore
	sender  emailpkg.Sender
	mutex   sync.RWMutex
	workers map[int64]*Worker
	cancels map[int64]context.CancelFunc
}

func NewWorkerManager(tasks TaskStore, sender emailpkg.Sender) *WorkerManager {
	if Manager != nil {
		return Manager
	}
	once.Do(func() {
		Manager = &WorkerManager{
			tasks:   tasks,
			workers: make(map[int64]*Worker),
			cancels: make(map[int64]context.CancelFunc),
			sender:  sender,
		}
	})
	go func() {
		for {
			select {
			case <-time.After(time.Minute):
				checkWorker()
			}
		}
	}()
	return Manager
}

// AddWorker starts a worker only when one is not already processing the task.
func (m *WorkerManager) AddWorker(id int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.workers[id]; !exists {
		ctx, cancel := context.WithCancel(context.Background())
		worker := NewWorker(ctx, id, m.tasks, m.sender)
		m.workers[id] = worker
		m.cancels[id] = cancel
		go worker.Start()
		logger.Info("Batch Send Email",
			logger.Field("message", "Added new worker"),
			logger.Field("task_id", id),
		)
		return
	}
	logger.Info("Batch Send Email",
		logger.Field("message", "Worker already exists"),
		logger.Field("task_id", id),
	)
}

// GetWorker returns the worker currently assigned to id.
func (m *WorkerManager) GetWorker(id int64) *Worker {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	if worker, exists := m.workers[id]; exists {
		return worker
	}
	logger.Error("Batch Send Email",
		logger.Field("message", "Worker not found"),
		logger.Field("task_id", id),
	)
	return nil
}

// RemoveWorker cancels and removes the worker assigned to id.
func (m *WorkerManager) RemoveWorker(id int64) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	if _, exists := m.workers[id]; exists {
		delete(m.workers, id)
		if cancelFunc, ok := m.cancels[id]; ok {
			cancelFunc()
			delete(m.cancels, id)
		}
		logger.Info("Batch Send Email",
			logger.Field("message", "Removed worker"),
			logger.Field("task_id", id),
		)
		return
	}
	logger.Error("Batch Send Email",
		logger.Field("message", "Worker not found for removal"),
		logger.Field("task_id", id),
	)
}

func checkWorker() {
	if Manager == nil {
		return
	}
	Manager.mutex.Lock()
	defer Manager.mutex.Unlock()
	for id, worker := range Manager.workers {
		if worker.IsRunning() != 2 {
			continue
		}
		delete(Manager.workers, id)
		if cancelFunc, ok := Manager.cancels[id]; ok {
			cancelFunc()
			delete(Manager.cancels, id)
		}
		logger.Info("Batch Send Email",
			logger.Field("message", "Removed completed worker"),
			logger.Field("task_id", id),
		)
	}
}
