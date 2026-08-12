package main

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestWorkerDrainRequestValidate(t *testing.T) {
	now := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		request WorkerDrainRequest
		wantErr bool
	}{
		{
			name: "slurm signal",
			request: WorkerDrainRequest{
				Reason:      WorkerDrainReasonSlurmSignal,
				RequestedAt: now,
			},
		},
		{
			name: "administrative",
			request: WorkerDrainRequest{
				Reason:      WorkerDrainReasonAdministrative,
				RequestedAt: now,
			},
		},
		{
			name: "unsupported reason",
			request: WorkerDrainRequest{
				Reason:      "operator_guess",
				RequestedAt: now,
			},
			wantErr: true,
		},
		{
			name: "missing time",
			request: WorkerDrainRequest{
				Reason: WorkerDrainReasonSlurmSignal,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.request.Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestWorkerDrainRequestsLatchesFirstValidRequest(t *testing.T) {
	drains := NewWorkerDrainRequests()
	requestedAt := time.Date(2026, time.August, 4, 8, 30, 0, 0, time.FixedZone("EDT", -4*60*60))
	first := WorkerDrainRequest{
		Reason:      WorkerDrainReasonSlurmSignal,
		RequestedAt: requestedAt,
	}

	accepted, err := drains.Request(first)
	if err != nil {
		t.Fatalf("Request(first) error = %v", err)
	}
	if !accepted {
		t.Fatal("Request(first) accepted = false, want true")
	}

	want := first
	want.RequestedAt = requestedAt.UTC()
	if got := <-drains.C(); got != want {
		t.Fatalf("drain event = %#v, want %#v", got, want)
	}
	if got, ok := drains.Current(); !ok || got != want {
		t.Fatalf("Current() = (%#v, %t), want (%#v, true)", got, ok, want)
	}

	accepted, err = drains.Request(WorkerDrainRequest{
		Reason:      WorkerDrainReasonAdministrative,
		RequestedAt: requestedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("Request(second) error = %v", err)
	}
	if accepted {
		t.Fatal("Request(second) accepted = true, want false")
	}
	select {
	case got := <-drains.C():
		t.Fatalf("unexpected second drain event: %#v", got)
	default:
	}
}

func TestWorkerDrainRequestsRejectsInvalidRequestWithoutLatching(t *testing.T) {
	drains := NewWorkerDrainRequests()
	if accepted, err := drains.Request(WorkerDrainRequest{}); err == nil || accepted {
		t.Fatalf("Request(invalid) = (%t, %v), want (false, error)", accepted, err)
	}
	if got, ok := drains.Current(); ok {
		t.Fatalf("Current() = (%#v, true), want no request", got)
	}

	valid := WorkerDrainRequest{
		Reason:      WorkerDrainReasonAdministrative,
		RequestedAt: time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC),
	}
	if accepted, err := drains.Request(valid); err != nil || !accepted {
		t.Fatalf("Request(valid) = (%t, %v), want (true, nil)", accepted, err)
	}
}

func TestWorkerDrainRequestsZeroValueIsUsable(t *testing.T) {
	var drains WorkerDrainRequests
	want := WorkerDrainRequest{
		Reason:      WorkerDrainReasonSlurmSignal,
		RequestedAt: time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC),
	}

	if accepted, err := drains.Request(want); err != nil || !accepted {
		t.Fatalf("Request() = (%t, %v), want (true, nil)", accepted, err)
	}
	if got := <-drains.C(); got != want {
		t.Fatalf("drain event = %#v, want %#v", got, want)
	}
}

func TestWorkerDrainRequestsConcurrentRequestsAcceptExactlyOne(t *testing.T) {
	var drains WorkerDrainRequests
	const requestCount = 32
	var accepted atomic.Int32
	var wait sync.WaitGroup
	wait.Add(requestCount)

	for i := 0; i < requestCount; i++ {
		go func(i int) {
			defer wait.Done()
			ok, err := drains.Request(WorkerDrainRequest{
				Reason:      WorkerDrainReasonSlurmSignal,
				RequestedAt: time.Date(2026, time.August, 4, 12, 0, i, 0, time.UTC),
			})
			if err != nil {
				t.Errorf("Request(%d) error = %v", i, err)
				return
			}
			if ok {
				accepted.Add(1)
			}
		}(i)
	}
	wait.Wait()

	if got := accepted.Load(); got != 1 {
		t.Fatalf("accepted requests = %d, want 1", got)
	}
	event := <-drains.C()
	if current, ok := drains.Current(); !ok || current != event {
		t.Fatalf("Current() = (%#v, %t), want (%#v, true)", current, ok, event)
	}
	select {
	case got := <-drains.C():
		t.Fatalf("unexpected second drain event: %#v", got)
	default:
	}
}

func TestPlatformWorkerDrainSourceSupportsInjectionAndIdempotentStop(t *testing.T) {
	source, stop := newPlatformWorkerDrainSource(nil)
	if source == nil || stop == nil {
		t.Fatal("newPlatformWorkerDrainSource() returned a nil source or stop function")
	}
	defer stop()

	drains, ok := source.(*WorkerDrainRequests)
	if !ok {
		t.Fatalf("platform drain source type = %T, want *WorkerDrainRequests", source)
	}
	want := WorkerDrainRequest{
		Reason:      WorkerDrainReasonAdministrative,
		RequestedAt: time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC),
	}
	if accepted, err := drains.Request(want); err != nil || !accepted {
		t.Fatalf("Request() = (%t, %v), want (true, nil)", accepted, err)
	}
	if got := <-source.C(); got != want {
		t.Fatalf("platform drain event = %#v, want %#v", got, want)
	}

	stop()
	stop()
}

func TestPlatformWorkerDrainSourceReceivesLinuxSIGUSR1(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux SIGUSR1 delivery is Linux-only")
	}

	source, stop := newPlatformWorkerDrainSource(nil)
	defer stop()
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("FindProcess() error = %v", err)
	}
	const linuxSIGUSR1 = syscall.Signal(10)
	if err := process.Signal(linuxSIGUSR1); err != nil {
		t.Fatalf("Signal(SIGUSR1) error = %v", err)
	}

	select {
	case request := <-source.C():
		if request.Reason != WorkerDrainReasonSlurmSignal || request.RequestedAt.IsZero() {
			t.Fatalf("SIGUSR1 drain request = %#v", request)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SIGUSR1 drain request")
	}
}
