package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/gechr/clog"
	"github.com/gechr/clog/fx"
	"golang.org/x/sync/errgroup"
)

// KeyResult carries one per-key read result while preserving the original key.
type KeyResult[T any] struct {
	Key   string
	Value T
	Err   error
}

// FanOutKeysProgress runs FanOutKeys while showing a determinate progress bar
// as keys complete. The bar mirrors the auth-login spinner's gating:
// NonTTYSilent suppresses it whenever stderr is not a terminal — piped,
// redirected, or captured by an agent — and clog writes status to stderr, so a
// stdout JSON envelope stays clean even under --output=json. A single key, or a
// fanout the bar cannot meaningfully track, simply falls through to FanOutKeys.
func FanOutKeysProgress[T any](
	ctx context.Context,
	label string,
	keys []string,
	parallelism int,
	fn func(context.Context, string) (T, error),
) ([]KeyResult[T], error) {
	if ctx == nil || fn == nil || len(keys) <= 1 {
		return FanOutKeys(ctx, keys, parallelism, fn)
	}

	var (
		results []KeyResult[T]
		fanErr  error
		done    atomic.Int64
	)
	// SetProgress stores to an atomic, so reporting from the parallel workers is
	// race-free; the bar reads the latest count on each animation frame.
	waitErr := clog.Bar(label, len(keys)).
		NonTTYSilent(true).
		Progress(ctx, func(ctx context.Context, u *fx.Update) error {
			tracked := func(ctx context.Context, key string) (T, error) {
				value, err := fn(ctx, key)
				u.SetProgress(int(done.Add(1)))
				return value, err
			}
			results, fanErr = FanOutKeys(ctx, keys, parallelism, tracked)
			return fanErr
		}).
		Silent()
	// Progress returns the task's error verbatim; it is the same fanErr, so
	// either is correct to return.
	if waitErr != nil {
		return results, waitErr
	}
	return results, fanErr
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
