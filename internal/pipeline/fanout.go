package pipeline

import "context"

// FanOut duplicates every value from the input channel to n output channels.
// Each output channel has a buffer of 1 to reduce blocking between consumers.
// All output channels close after the input channel closes or the context is
// cancelled.
//
// Context cancellation produces best-effort delivery: values being broadcast
// when cancellation occurs may reach some consumers but not others.
//
// Special cases:
//   - n <= 0: returns nil and drains the input channel in a background goroutine.
func FanOut[T any](ctx context.Context, in <-chan T, n int) []<-chan T {
	if n <= 0 {
		go func() {
			for {
				select {
				case _, ok := <-in:
					if !ok {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
		return nil
	}

	outs := make([]chan T, n)
	result := make([]<-chan T, n)
	for i := range n {
		ch := make(chan T, 1)
		outs[i] = ch
		result[i] = ch
	}

	go func() {
		defer func() {
			for _, ch := range outs {
				close(ch)
			}
		}()
		for {
			select {
			case v, ok := <-in:
				if !ok {
					return
				}
				for _, ch := range outs {
					select {
					case ch <- v:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return result
}
