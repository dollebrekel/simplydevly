package pipeline

import (
	"context"
	"runtime"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

// sendAll writes values to a new channel and closes it.
func sendAll[T any](values ...T) <-chan T {
	ch := make(chan T, len(values))
	for _, v := range values {
		ch <- v
	}
	close(ch)
	return ch
}

// collect drains a channel into a slice.
func collect[T any](ch <-chan T) []T {
	var out []T
	for v := range ch {
		out = append(out, v)
	}
	return out
}

// goroutineCount returns the current number of goroutines.
func goroutineCount() int {
	return runtime.NumGoroutine()
}

// --- FanIn tests ---

func TestFanIn_MergesMultipleChannels(t *testing.T) {
	ctx := context.Background()
	ch1 := sendAll(1, 2, 3)
	ch2 := sendAll(4, 5, 6)
	ch3 := sendAll(7, 8, 9)

	out := FanIn(ctx, ch1, ch2, ch3)
	result := collect(out)

	sort.Ints(result)
	assert.Equal(t, []int{1, 2, 3, 4, 5, 6, 7, 8, 9}, result)
}

func TestFanIn_SingleChannel(t *testing.T) {
	ctx := context.Background()
	ch := sendAll("a", "b", "c")
	out := FanIn(ctx, ch)
	result := collect(out)
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestFanIn_EmptyChannels(t *testing.T) {
	ctx := context.Background()
	ch1 := sendAll[int]()
	ch2 := sendAll[int]()

	out := FanIn(ctx, ch1, ch2)
	result := collect(out)
	assert.Empty(t, result)
}

func TestFanIn_NoChannels(t *testing.T) {
	ctx := context.Background()
	out := FanIn[int](ctx)
	result := collect(out)
	assert.Empty(t, result)
}

func TestFanIn_NilChannelsSkipped(t *testing.T) {
	ctx := context.Background()
	ch1 := sendAll(1, 2)
	var nilCh <-chan int
	ch3 := sendAll(3, 4)

	out := FanIn(ctx, ch1, nilCh, ch3)
	result := collect(out)

	sort.Ints(result)
	assert.Equal(t, []int{1, 2, 3, 4}, result)
}

func TestFanIn_AllNilChannels(t *testing.T) {
	ctx := context.Background()
	var ch1, ch2 <-chan int

	out := FanIn(ctx, ch1, ch2)
	result := collect(out)
	assert.Empty(t, result)
}

func TestFanIn_GoroutineLeakFree(t *testing.T) {
	before := goroutineCount()

	ctx := context.Background()
	ch1 := sendAll(1, 2)
	ch2 := sendAll(3, 4)
	out := FanIn(ctx, ch1, ch2)
	collect(out)

	// Allow goroutines to wind down.
	time.Sleep(50 * time.Millisecond)
	after := goroutineCount()
	assert.LessOrEqual(t, after, before+2, "goroutine leak detected")
}

func TestFanIn_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Infinite producer.
	ch := make(chan int)
	go func() {
		defer close(ch)
		i := 0
		for {
			select {
			case ch <- i:
				i++
			case <-ctx.Done():
				return
			}
		}
	}()

	out := FanIn(ctx, ch)

	// Read a few values then cancel.
	<-out
	<-out
	cancel()

	// Drain remaining — channel should close.
	for range out {
	}
}

// --- FanOut tests ---

func TestFanOut_DuplicatesToAllConsumers(t *testing.T) {
	ctx := context.Background()
	in := sendAll(10, 20, 30)

	outs := FanOut(ctx, in, 3)
	require.Len(t, outs, 3)

	// Must collect concurrently — FanOut sends to all consumers per value,
	// so sequential consumption deadlocks when buffers fill.
	results := make([][]int, 3)
	var wg sync.WaitGroup
	wg.Add(3)
	for i, ch := range outs {
		go func(idx int, c <-chan int) {
			defer wg.Done()
			results[idx] = collect(c)
		}(i, ch)
	}
	wg.Wait()

	for i, result := range results {
		assert.Equal(t, []int{10, 20, 30}, result, "consumer %d", i)
	}
}

func TestFanOut_SingleConsumer(t *testing.T) {
	ctx := context.Background()
	in := sendAll("x", "y")

	outs := FanOut(ctx, in, 1)
	require.Len(t, outs, 1)
	assert.Equal(t, []string{"x", "y"}, collect(outs[0]))
}

func TestFanOut_ZeroConsumers(t *testing.T) {
	ctx := context.Background()
	in := sendAll(1, 2, 3)
	outs := FanOut(ctx, in, 0)
	assert.Nil(t, outs)
}

func TestFanOut_NegativeConsumers(t *testing.T) {
	ctx := context.Background()
	in := sendAll(1, 2, 3)
	outs := FanOut(ctx, in, -1)
	assert.Nil(t, outs)
}

func TestFanOut_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Unbuffered, slow input — gives us time to cancel.
	in := make(chan int)
	go func() {
		defer close(in)
		for i := range 100 {
			select {
			case in <- i:
			case <-ctx.Done():
				return
			}
		}
	}()

	outs := FanOut(ctx, in, 2)

	// Drain all consumers concurrently.
	var wg sync.WaitGroup
	wg.Add(len(outs))
	for _, ch := range outs {
		go func(c <-chan int) {
			defer wg.Done()
			// Read a couple values then cancel.
			count := 0
			for range c {
				count++
				if count >= 2 {
					cancel()
				}
			}
		}(ch)
	}
	wg.Wait()
}

func TestFanOut_GoroutineLeakFree(t *testing.T) {
	before := goroutineCount()

	ctx := context.Background()
	in := sendAll(1, 2, 3)
	outs := FanOut(ctx, in, 3)

	var wg sync.WaitGroup
	wg.Add(len(outs))
	for _, ch := range outs {
		go func(c <-chan int) {
			defer wg.Done()
			collect(c)
		}(ch)
	}
	wg.Wait()

	time.Sleep(50 * time.Millisecond)
	after := goroutineCount()
	assert.LessOrEqual(t, after, before+2, "goroutine leak detected")
}

// --- Chain tests ---

func TestChain_ComposesTwoStages(t *testing.T) {
	double := func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for v := range in {
				select {
				case out <- v * 2:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}

	addTen := func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for v := range in {
				select {
				case out <- v + 10:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}

	chained := Chain(double, addTen)

	ctx := context.Background()
	in := sendAll(1, 2, 3)
	result := collect(chained(ctx, in))

	// (1*2)+10=12, (2*2)+10=14, (3*2)+10=16
	assert.Equal(t, []int{12, 14, 16}, result)
}

func TestChain_DifferentTypes(t *testing.T) {
	intToString := func(_ context.Context, in <-chan int) <-chan string {
		out := make(chan string)
		go func() {
			defer close(out)
			for v := range in {
				out <- string(rune('A' + v))
			}
		}()
		return out
	}

	toUpper := func(_ context.Context, in <-chan string) <-chan string {
		out := make(chan string)
		go func() {
			defer close(out)
			for v := range in {
				out <- v + "!"
			}
		}()
		return out
	}

	chained := Chain(intToString, toUpper)

	ctx := context.Background()
	in := sendAll(0, 1, 2) // A, B, C
	result := collect(chained(ctx, in))
	assert.Equal(t, []string{"A!", "B!", "C!"}, result)
}

func TestChain_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	slow := func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for v := range in {
				select {
				case out <- v:
					time.Sleep(100 * time.Millisecond)
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}

	identity := func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for v := range in {
				select {
				case out <- v:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}

	chained := Chain(slow, identity)

	in := make(chan int, 10)
	for i := range 10 {
		in <- i
	}
	close(in)

	out := chained(ctx, in)

	// Read one value then cancel.
	<-out
	cancel()

	// Drain remaining — channel should close.
	for range out {
	}
}

// --- Backpressure test ---

func TestFanIn_Backpressure(t *testing.T) {
	ctx := context.Background()

	// Unbuffered output means FanIn goroutines block until consumer reads.
	ch1 := make(chan int, 1)
	ch2 := make(chan int, 1)

	ch1 <- 1
	ch2 <- 2
	close(ch1)
	close(ch2)

	out := FanIn(ctx, ch1, ch2)

	// Don't read immediately — goroutines should block.
	time.Sleep(20 * time.Millisecond)

	result := collect(out)
	sort.Ints(result)
	assert.Equal(t, []int{1, 2}, result)
}

// --- Graceful shutdown test ---

func TestFanOut_GracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	before := goroutineCount()

	in := make(chan int)
	outs := FanOut(ctx, in, 3)

	// Cancel before sending anything.
	cancel()
	close(in)

	// All output channels should close.
	for _, ch := range outs {
		collect(ch)
	}

	time.Sleep(50 * time.Millisecond)
	after := goroutineCount()
	assert.LessOrEqual(t, after, before+2, "goroutine leak after shutdown")
}

// --- Concurrent stress test ---

func TestFanIn_ConcurrentProducers(t *testing.T) {
	ctx := context.Background()
	const numProducers = 10
	const itemsPerProducer = 100

	channels := make([]<-chan int, numProducers)
	for i := range numProducers {
		ch := make(chan int, itemsPerProducer)
		go func(base int) {
			defer close(ch)
			for j := range itemsPerProducer {
				ch <- base*itemsPerProducer + j
			}
		}(i)
		channels[i] = ch
	}

	out := FanIn(ctx, channels...)
	result := collect(out)

	assert.Len(t, result, numProducers*itemsPerProducer)

	// Verify all values present.
	sort.Ints(result)
	for i := range numProducers * itemsPerProducer {
		assert.Equal(t, i, result[i])
	}
}

// --- Integration: FanOut -> FanIn round-trip ---

func TestFanOutFanIn_RoundTrip(t *testing.T) {
	ctx := context.Background()
	in := sendAll(1, 2, 3)

	// Fan out to 3 consumers.
	outs := FanOut(ctx, in, 3)

	// Each consumer processes independently, then fan-in collects all.
	processed := make([]<-chan int, len(outs))
	for i, ch := range outs {
		processed[i] = ch
	}

	merged := FanIn(ctx, processed...)
	result := collect(merged)

	// 3 values x 3 consumers = 9 total.
	assert.Len(t, result, 9)

	sort.Ints(result)
	assert.Equal(t, []int{1, 1, 1, 2, 2, 2, 3, 3, 3}, result)
}

// --- Stage type compliance ---

func TestStage_TypeSignatureCompliance(t *testing.T) {
	// Verify that a plain function matches the Stage type.
	var s Stage[int, string] = func(_ context.Context, in <-chan int) <-chan string {
		out := make(chan string)
		go func() {
			defer close(out)
			for range in {
				out <- "ok"
			}
		}()
		return out
	}

	ctx := context.Background()
	in := sendAll(1)
	result := collect(s(ctx, in))
	assert.Equal(t, []string{"ok"}, result)
}

// --- Goroutine leak detection with runtime counting ---

func TestPipeline_NoLeaksAfterFullPipeline(t *testing.T) {
	// Force GC to stabilize goroutine count.
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	before := goroutineCount()

	ctx := context.Background()

	// Build a pipeline: source -> fanout(3) -> transform each -> fanin.
	source := sendAll(1, 2, 3, 4, 5)
	branches := FanOut(ctx, source, 3)

	transform := func(ctx context.Context, in <-chan int) <-chan int {
		out := make(chan int)
		go func() {
			defer close(out)
			for v := range in {
				select {
				case out <- v * v:
				case <-ctx.Done():
					return
				}
			}
		}()
		return out
	}

	transformed := make([]<-chan int, len(branches))
	for i, b := range branches {
		transformed[i] = transform(ctx, b)
	}

	merged := FanIn(ctx, transformed...)
	result := collect(merged)

	// 5 values x 3 branches = 15 results.
	assert.Len(t, result, 15)

	// Wait for goroutines to clean up.
	time.Sleep(100 * time.Millisecond)
	runtime.GC()
	after := goroutineCount()
	assert.LessOrEqual(t, after, before+2, "goroutine leak in full pipeline")
}

// --- FanIn with context-aware producers ---

func TestFanIn_ProducerRespectsContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Producers that respect context.
	makeCh := func(ctx context.Context) <-chan int {
		ch := make(chan int)
		go func() {
			defer close(ch)
			i := 0
			for {
				select {
				case ch <- i:
					i++
				case <-ctx.Done():
					return
				}
			}
		}()
		return ch
	}

	ch1 := makeCh(ctx)
	ch2 := makeCh(ctx)

	out := FanIn(ctx, ch1, ch2)

	// Read a few values.
	var mu sync.Mutex
	var received []int
	done := make(chan struct{})

	go func() {
		for v := range out {
			mu.Lock()
			received = append(received, v)
			if len(received) >= 10 {
				mu.Unlock()
				cancel()
				// Drain remaining.
				for range out {
				}
				close(done)
				return
			}
			mu.Unlock()
		}
		close(done)
	}()

	<-done
	mu.Lock()
	assert.GreaterOrEqual(t, len(received), 10)
	mu.Unlock()
}
