package main

import "context"

type Runtime interface {
	Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error
}

type SharedFilesystemWorkerRuntime struct {
	Root string
}

func (r SharedFilesystemWorkerRuntime) Prepare(ctx context.Context, transport Transport, dialect ShellDialect) error {
	return nil
}
