package pipeline

import (
	"context"
	"sync"
)

// FanIn merges multiple input channels into a single output channel.
// The output channel closes after all input channels have closed or the
// context is cancelled. Nil channels in the input are skipped.
//
// Special cases:
//   - Zero channels: returns a closed channel immediately (no goroutines).
//   - Single non-nil channel: returns it directly (no goroutine overhead).
func FanIn[T any](ctx context.Context, channels ...<-chan T) <-chan T {
	// Filter nil channels.
	var valid []<-chan T
	for _, ch := range channels {
		if ch != nil {
			valid = append(valid, ch)
		}
	}

	// Zero channels: return closed channel.
	if len(valid) == 0 {
		out := make(chan T)
		close(out)
		return out
	}

	// Single channel: wrap for consistent ctx cancellation semantics.
	if len(valid) == 1 {
		out := make(chan T)
		go func() {
			defer close(out)
			for v := range valid[0] {
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}

	out := make(chan T)

	var wg sync.WaitGroup
	wg.Add(len(valid))

	for _, ch := range valid {
		go func(c <-chan T) {
			defer wg.Done()
			for v := range c {
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}(ch)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}
