package lifecycle

import (
	"log"
	"sync"

	"github.com/perfect-panel/server/pkg/logger"
)

type (
	// Starter is the interface wraps the Start method.
	Starter interface {
		Start()
	}

	// Stopper is the interface wraps the Stop method.
	Stopper interface {
		Stop()
	}

	// Service is the interface that groups Start and Stop methods.
	Service interface {
		Starter
		Stopper
	}

	// Group A ServiceGroup is a group of services.
	// Attention: the starting order of the added services is not guaranteed.
	Group struct {
		services []Service
		stopOnce sync.Once
	}
)

// NewServiceGroup returns a ServiceGroup.
func NewServiceGroup() *Group {
	return new(Group)
}

// Add adds service into sg.
func (sg *Group) Add(service Service) {
	// push front, stop with reverse order.
	sg.services = append([]Service{service}, sg.services...)
}

// Start starts the ServiceGroup.
// There should not be any logic code after calling this method, because this
// method is a blocking one: it returns once every service's Start returned.
func (sg *Group) Start() {
	AddShutdownListener(func() {
		sg.Stop()
	})

	sg.doStart()
}

// Stop stops the services in the reverse order they were added, then closes
// the log output as the final step. The file output buffers entries, so the
// lines written while stopping — typically the shutdown errors — only reach
// the files once the output is closed. The output closes even when a service
// panics while stopping.
func (sg *Group) Stop() {
	sg.stopOnce.Do(func() {
		defer closeLogs()
		sg.doStop()
	})
}

// closeLogs flushes and closes the log output. Anything logged afterwards
// goes to the console.
func closeLogs() {
	if err := logger.Close(); err != nil {
		log.Printf("close log output: %v", err)
	}
}

func (sg *Group) doStart() {
	var group sync.WaitGroup
	for _, service := range sg.services {
		group.Go(service.Start)
	}
	group.Wait()
}

func (sg *Group) doStop() {
	for _, service := range sg.services {
		service.Stop()
	}
}

// WithStart wraps a start func as a Service.
func WithStart(start func()) Service {
	return startOnlyService{
		start: start,
	}
}

// WithStarter wraps a Starter as a Service.
func WithStarter(start Starter) Service {
	return starterOnlyService{
		Starter: start,
	}
}

type (
	stopper struct{}

	startOnlyService struct {
		start func()
		stopper
	}

	starterOnlyService struct {
		Starter
		stopper
	}
)

func (s stopper) Stop() {
}

func (s startOnlyService) Start() {
	s.start()
}
