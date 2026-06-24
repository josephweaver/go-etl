package main

import "context"

type Transport interface {
	Copy(ctx context.Context, localPath string, remotePath string) error
	Exec(ctx context.Context, args ...string) ([]byte, error)
}
