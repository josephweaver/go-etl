//go:build linux

package main

import (
	"os"
	"os/signal"
	"sync"
	"syscall"
)

// newPlatformWorkerDrainSource translates Linux SIGUSR1 delivery into the
// adapter-neutral worker drain boundary. The returned stop function is
// idempotent and waits until the signal goroutine has exited.
func newPlatformWorkerDrainSource(clock WorkerLifecycleClock) (WorkerDrainSource, func()) {
	if clock == nil {
		clock = realWorkerLifecycleClock{}
	}

	drains := NewWorkerDrainRequests()
	signals := make(chan os.Signal, 1)
	stopping := make(chan struct{})
	stopped := make(chan struct{})
	signal.Notify(signals, syscall.SIGUSR1)

	go func() {
		defer close(stopped)
		for {
			select {
			case <-stopping:
				return
			case <-signals:
				select {
				case <-stopping:
					return
				default:
				}
				_, _ = drains.Request(WorkerDrainRequest{
					Reason:      WorkerDrainReasonSlurmSignal,
					RequestedAt: clock.Now(),
				})
			}
		}
	}()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			signal.Stop(signals)
			close(stopping)
			<-stopped
		})
	}
	return drains, stop
}
