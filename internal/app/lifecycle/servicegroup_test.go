package lifecycle

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/perfect-panel/server/pkg/logger"
	"github.com/stretchr/testify/assert"
)

var (
	number = 1
	mutex  sync.Mutex
	done   = make(chan struct{})
)

func TestServiceGroup(t *testing.T) {
	multipliers := []int{2, 3, 5, 7}
	want := 1

	group := NewServiceGroup()
	for _, multiplier := range multipliers {
		want *= multiplier
		service := newMockedService(multiplier)
		group.Add(service)
	}

	go group.Start()

	for i := 0; i < len(multipliers); i++ {
		<-done
	}

	group.Stop()

	mutex.Lock()
	defer mutex.Unlock()
	assert.Equal(t, want, number)
}

func TestServiceGroup_WithStart(t *testing.T) {
	multipliers := []int{2, 3, 5, 7}
	want := 1

	var wait sync.WaitGroup
	var lock sync.Mutex
	wait.Add(len(multipliers))
	group := NewServiceGroup()
	for _, multiplier := range multipliers {
		mul := multiplier
		group.Add(WithStart(func() {
			lock.Lock()
			want *= mul
			lock.Unlock()
			wait.Done()
		}))
	}

	go group.Start()
	wait.Wait()
	group.Stop()

	lock.Lock()
	defer lock.Unlock()
	assert.Equal(t, 210, want)
}

func TestServiceGroup_WithStarter(t *testing.T) {
	multipliers := []int{2, 3, 5, 7}
	want := 1

	var wait sync.WaitGroup
	var lock sync.Mutex
	wait.Add(len(multipliers))
	group := NewServiceGroup()
	for _, multiplier := range multipliers {
		mul := multiplier
		group.Add(WithStarter(mockedStarter{
			fn: func() {
				lock.Lock()
				want *= mul
				lock.Unlock()
				wait.Done()
			},
		}))
	}

	go group.Start()
	wait.Wait()
	group.Stop()

	lock.Lock()
	defer lock.Unlock()
	assert.Equal(t, 210, want)
}

type mockedStarter struct {
	fn func()
}

type panicStopService struct{ calls int }

func (*panicStopService) Start()  {}
func (s *panicStopService) Stop() { s.calls++; panic("test stop failure") }

func TestStopRetainsOnceDoPanicSemantics(t *testing.T) {
	s := &panicStopService{}
	group := NewServiceGroup()
	group.Add(s)
	func() { defer func() { _ = recover() }(); group.Stop() }()
	// Unlike sync.OnceFunc, the previous implementation did not repeat a panic.
	group.Stop()
	if s.calls != 1 {
		t.Fatalf("Stop called %d times", s.calls)
	}
}

func (s mockedStarter) Start() {
	s.fn()
}

// failingStopService logs the error its shutdown ran into, the way the HTTP
// server logs a failed graceful shutdown.
type failingStopService struct{ panics bool }

func (failingStopService) Start() {}

func (s failingStopService) Stop() {
	logger.Errorf("server shutdown error: %s", "context deadline exceeded")
	if s.panics {
		panic("stop failed")
	}
}

// bufferedWriter holds entries until it is closed, like the file output,
// whose entries wait in a channel for its writer goroutine.
type bufferedWriter struct {
	mu               sync.Mutex
	pending, flushed []string
	closed           bool
}

var _ logger.Writer = (*bufferedWriter)(nil)

func (w *bufferedWriter) add(v any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, fmt.Sprint(v))
}

func (w *bufferedWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushed = append(w.flushed, w.pending...)
	w.pending = nil
	w.closed = true
	return nil
}

func (w *bufferedWriter) Debug(v any, _ ...logger.LogField) { w.add(v) }
func (w *bufferedWriter) Error(v any, _ ...logger.LogField) { w.add(v) }
func (w *bufferedWriter) Info(v any, _ ...logger.LogField)  { w.add(v) }
func (w *bufferedWriter) Slow(v any, _ ...logger.LogField)  { w.add(v) }
func (w *bufferedWriter) Stack(v any)                       { w.add(v) }

// Stop closes the log output last, so the errors the services log while
// stopping are flushed before the process exits — also when a service panics
// while stopping.
func TestStopClosesLogOutputLast(t *testing.T) {
	for _, panics := range []bool{false, true} {
		previous := logger.Reset()
		writer := &bufferedWriter{}
		logger.SetWriter(writer)

		group := NewServiceGroup()
		group.Add(failingStopService{panics: panics})
		func() {
			defer func() { _ = recover() }()
			group.Stop()
		}()

		if w := logger.Reset(); w != nil {
			t.Fatalf("panics=%v: the log output is still open after Stop", panics)
		}
		if previous != nil {
			logger.SetWriter(previous)
		}
		if !writer.closed || len(writer.pending) != 0 || len(writer.flushed) != 1 || !strings.Contains(writer.flushed[0], "context deadline exceeded") {
			t.Fatalf("panics=%v: writer = %+v, want the shutdown error flushed by the close", panics, writer)
		}
	}
}

// End to end with the file output: the shutdown error is in error.log once
// Stop returns.
func TestStopFlushesTheFileOutput(t *testing.T) {
	dir := t.TempDir()
	previous := logger.Reset()
	t.Cleanup(func() {
		logger.Reset()
		if previous != nil {
			logger.SetWriter(previous)
		}
	})
	if err := logger.SetUp(logger.LogConf{Mode: "file", Path: dir}); err != nil {
		t.Fatalf("SetUp: %v", err)
	}

	group := NewServiceGroup()
	group.Add(failingStopService{})
	group.Stop()

	data, err := os.ReadFile(filepath.Join(dir, "error.log"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "context deadline exceeded") {
		t.Fatalf("error.log = %q, want the shutdown error", data)
	}
}

type mockedService struct {
	quit       chan struct{}
	multiplier int
}

func newMockedService(multiplier int) *mockedService {
	return &mockedService{
		quit:       make(chan struct{}),
		multiplier: multiplier,
	}
}

func (s *mockedService) Start() {
	mutex.Lock()
	number *= s.multiplier
	mutex.Unlock()
	done <- struct{}{}
	<-s.quit
}

func (s *mockedService) Stop() {
	close(s.quit)
}
