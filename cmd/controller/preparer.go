package main

import "context"

type Preparer interface {
	Prepare(ctx context.Context) error
}

func prepareIfSupported(ctx context.Context, value any) error {
	preparer, ok := value.(Preparer)
	if !ok {
		return nil
	}
	return preparer.Prepare(ctx)
}
