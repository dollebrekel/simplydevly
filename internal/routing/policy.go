package routing

// TaskHints carries signals that inform routing decisions.
// The key convention is "task.category" with values "preprocess" or "primary".
const HintKeyCategory = "task.category"

// ProviderSelection is the result of a routing decision.
type ProviderSelection struct {
	Provider string
	Model    string // optional; "" means use provider default
	Reason   string // human-readable explanation
}

// RoutingPolicy selects a provider based on task hints.
type RoutingPolicy interface {
	Select(hints map[string]string) ProviderSelection
}
