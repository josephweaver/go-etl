package main

import (
	"context"
	"fmt"
	"testing"
)

type testPreparer struct {
	calls int
	err   error
}

func (p *testPreparer) Prepare(ctx context.Context) error {
	p.calls++
	return p.err
}

func TestPrepareIfSupportedCallsPreparer(t *testing.T) {
	preparer := &testPreparer{}

	if err := prepareIfSupported(context.Background(), preparer); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if preparer.calls != 1 {
		t.Fatalf("prepare calls = %d, want 1", preparer.calls)
	}
}

func TestPrepareIfSupportedSkipsNonPreparer(t *testing.T) {
	if err := prepareIfSupported(context.Background(), struct{}{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareIfSupportedReturnsPrepareError(t *testing.T) {
	want := fmt.Errorf("prepare failed")
	preparer := &testPreparer{err: want}

	if err := prepareIfSupported(context.Background(), preparer); err != want {
		t.Fatalf("error = %v, want %v", err, want)
	}
}
