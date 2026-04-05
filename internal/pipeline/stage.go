package pipeline

import "context"

// Stage is a composable processing function that reads from an input channel,
// transforms each element, and writes to an output channel. Stages respect
// context cancellation and close their output channel when done.
type Stage[In, Out any] func(ctx context.Context, in <-chan In) <-chan Out
