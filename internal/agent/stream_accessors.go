package agent

// Text returns the streamed text chunk. This accessor allows external packages
// (like cmd/siply/run.go) to extract text from stream.text events published
// by the agent via the EventBus.
func (e *streamTextEvent) Text() string { return e.text }
