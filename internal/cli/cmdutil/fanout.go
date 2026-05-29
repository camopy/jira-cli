package cmdutil

import (
	"context"
	"errors"
	"fmt"

	"golang.org/x/sync/errgroup"
)

// KeyResult carries one per-key read result while preserving the original key.
type KeyResult[T any] struct {
	Key   string
	Value T
	Err   error
}

// FanOutKeys runs fn for each key with bounded concurrency and returns results
// in the same order as keys. Per-key errors are stored in the result slot so
// one failed key does not cancel unrelated work.
func FanOutKeys[T any](
	ctx context.Context,
	keys []string,
	parallelism int,
	fn func(context.Context, string) (T, error),
) ([]KeyResult[T], error) {
	if ctx == nil {
		return nil, errors.New("context must not be nil")
	}
	if fn == nil {
		return nil, errors.New("fanout function must not be nil")
	}
	if parallelism < defaultParallelism || parallelism > maxParallelism {
		return nil, fmt.Errorf("parallelism must be between %d and %d", defaultParallelism, maxParallelism)
	}

	results := make([]KeyResult[T], len(keys))
	for i, key := range keys {
		results[i].Key = key
	}
	if len(keys) == 0 {
		return results, nil
	}
	if parallelism == defaultParallelism {
		for i, key := range keys {
			if err := ctx.Err(); err != nil {
				return results, err
			}
			value, err := fn(ctx, key)
			results[i] = KeyResult[T]{Key: key, Value: value, Err: err}
		}
		if err := ctx.Err(); err != nil {
			return results, err
		}
		return results, nil
	}

	workerCount := parallelism
	if workerCount > len(keys) {
		workerCount = len(keys)
	}

	jobs := make(chan int)
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(workerCount)
	for range workerCount {
		group.Go(func() error {
			for {
				select {
				case <-groupCtx.Done():
					return nil
				case i, ok := <-jobs:
					if !ok {
						return nil
					}
					if err := groupCtx.Err(); err != nil {
						return nil
					}
					value, err := fn(groupCtx, keys[i])
					results[i] = KeyResult[T]{Key: keys[i], Value: value, Err: err}
				}
			}
		})
	}

	var feedErr error
feed:
	for i := range keys {
		if err := groupCtx.Err(); err != nil {
			feedErr = err
			break
		}
		select {
		case <-groupCtx.Done():
			feedErr = groupCtx.Err()
			break feed
		case jobs <- i:
		}
	}
	close(jobs)

	if err := group.Wait(); err != nil {
		return results, err
	}
	if feedErr != nil {
		return results, feedErr
	}
	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}
