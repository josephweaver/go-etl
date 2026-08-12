//go:build !linux

package main

// newPlatformWorkerDrainSource preserves the injectable drain boundary on
// platforms where the worker does not subscribe to Linux SIGUSR1.
func newPlatformWorkerDrainSource(WorkerLifecycleClock) (WorkerDrainSource, func()) {
	return NewWorkerDrainRequests(), func() {}
}
