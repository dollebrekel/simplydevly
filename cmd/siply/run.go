package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"siply.dev/siply/internal/agent"
	"siply.dev/siply/internal/core"
	"siply.dev/siply/internal/credential"
	"siply.dev/siply/internal/events"
	"siply.dev/siply/internal/permission"
	"siply.dev/siply/internal/providers/anthropic"
	"siply.dev/siply/internal/providers/ollama"
	"siply.dev/siply/internal/providers/openai"
	"siply.dev/siply/internal/providers/openrouter"
	"siply.dev/siply/internal/routing"
	"siply.dev/siply/internal/tools"
)

const defaultProviderName = "anthropic"

// ansiPattern matches ANSI escape sequences for stripping in non-TTY mode.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func newRunCmd() *cobra.Command {
	var taskFlag string
	var yoloFlag, autoAcceptFlag, routingFlag bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a one-shot task non-interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			if taskFlag == "" {
				return fmt.Errorf("run: --task flag is required")
			}
			return executeRun(cmd.Context(), taskFlag, yoloFlag, autoAcceptFlag, routingFlag)
		},
	}
	cmd.Flags().StringVar(&taskFlag, "task", "", "Task description to execute")
	cmd.Flags().BoolVar(&yoloFlag, "yolo", false, "Skip all permission confirmations")
	cmd.Flags().BoolVar(&autoAcceptFlag, "auto-accept", false, "Auto-accept non-destructive actions")
	cmd.Flags().BoolVar(&routingFlag, "routing", false, "Enable smart model routing")
	_ = cmd.MarkFlagRequired("task")
	return cmd
}

func executeRun(ctx context.Context, task string, yolo, autoAccept, routingEnabled bool) error {
	// Bootstrap credential store.
	credStore := &credential.EnvStore{}

	// Check routing: enabled by flag or SIPLY_ROUTING_ENABLED env var.
	if !routingEnabled && strings.EqualFold(os.Getenv("SIPLY_ROUTING_ENABLED"), "true") {
		routingEnabled = true
	}

	// Bootstrap provider (with optional routing).
	provider, err := bootstrapProvider(credStore)
	if err != nil {
		return fmt.Errorf("run: bootstrap provider: %w", err)
	}

	// Bootstrap permission evaluator.
	cfg := permission.DefaultConfig()
	if yolo {
		cfg.Mode = permission.ModeYolo
	} else if autoAccept {
		cfg.Mode = permission.ModeAutoAccept
	}
	perm, err := permission.NewEvaluator(cfg)
	if err != nil {
		return fmt.Errorf("run: bootstrap permission: %w", err)
	}

	// Bootstrap tool registry.
	registry := tools.NewRegistry(perm)

	// Bootstrap event bus.
	eventBus := events.NewBus()

	// Wire routing if enabled.
	if routingEnabled {
		routed, routeErr := bootstrapRouting(credStore, provider, eventBus)
		if routeErr != nil {
			return fmt.Errorf("run: bootstrap routing: %w", routeErr)
		}
		provider = routed
	}

	// Bootstrap remaining dependencies.
	tokenCounter := &agent.NoopTokenCounter{}
	statusCollector := &agent.NoopStatusCollector{}
	contextMgr := agent.NewTruncationCompactor()

	// Initialize all lifecycle components.
	components := []struct {
		name string
		lc   core.Lifecycle
	}{
		{"credential-store", credStore},
		{"provider", provider},
		{"permission", perm},
		{"tools", registry},
		{"events", eventBus},
		{"status", statusCollector},
		{"context", contextMgr},
	}

	for _, c := range components {
		if err := c.lc.Init(ctx); err != nil {
			return fmt.Errorf("run: init %s: %w", c.name, err)
		}
	}
	for _, c := range components {
		if err := c.lc.Start(ctx); err != nil {
			return fmt.Errorf("run: start %s: %w", c.name, err)
		}
	}
	defer func() {
		stopCtx := context.Background()
		for i := len(components) - 1; i >= 0; i-- {
			_ = components[i].lc.Stop(stopCtx)
		}
	}()

	// Detect TTY for output formatting.
	isTTY := term.IsTerminal(int(os.Stdout.Fd()))

	// Subscribe to stream.text events to collect agent output.
	var output strings.Builder
	eventBus.Subscribe("stream.text", func(_ context.Context, ev core.Event) {
		if te, ok := ev.(interface{ Text() string }); ok {
			output.WriteString(te.Text())
		}
	})

	// Build agent deps.
	deps := agent.AgentDeps{
		Provider: provider,
		Tools:    registry,
		Events:   eventBus,
		Tokens:   tokenCounter,
		Context:  contextMgr,
		Status:   statusCollector,
		Perm:     perm,
	}

	ag := agent.NewAgent(deps)
	if err := ag.Init(ctx); err != nil {
		return fmt.Errorf("run: init agent: %w", err)
	}
	if err := ag.Start(ctx); err != nil {
		return fmt.Errorf("run: start agent: %w", err)
	}
	defer func() {
		_ = ag.Stop(context.Background())
	}()

	// Execute the task.
	runErr := ag.Run(ctx, task)

	// Write collected output to stdout.
	text := output.String()
	if text != "" {
		if !isTTY {
			text = stripANSI(text)
		}
		fmt.Fprint(os.Stdout, text)
	}

	if runErr != nil {
		return fmt.Errorf("run: agent execution: %w", runErr)
	}
	return nil
}

// bootstrapProvider creates a provider adapter based on SIPLY_PROVIDER env var.
func bootstrapProvider(credStore core.CredentialStore) (core.Provider, error) {
	providerName := os.Getenv("SIPLY_PROVIDER")
	if providerName == "" {
		providerName = defaultProviderName
	}
	return buildProvider(providerName, credStore)
}

// bootstrapRouting creates a RoutingProvider wrapping the primary provider
// and a preprocess provider configured via environment variables.
func bootstrapRouting(credStore core.CredentialStore, primary core.Provider, eventBus core.EventBus) (core.Provider, error) {
	preprocessProviderName := os.Getenv("SIPLY_PREPROCESS_PROVIDER")
	if preprocessProviderName == "" {
		// No preprocess provider configured — return primary as-is (no routing).
		fmt.Fprintln(os.Stderr, "[routing] warning: routing enabled but SIPLY_PREPROCESS_PROVIDER not set — routing disabled")
		return primary, nil
	}

	preprocessModel := os.Getenv("SIPLY_PREPROCESS_MODEL")
	primaryName := os.Getenv("SIPLY_PROVIDER")
	if primaryName == "" {
		primaryName = defaultProviderName
	}

	// Build providers map. Use synthetic keys when preprocess and primary share
	// the same provider name (e.g., Anthropic Sonnet + Anthropic Haiku) so the
	// map has 2 entries and routing bypass is not triggered.
	providers := make(map[string]core.Provider)
	preprocessKey := preprocessProviderName
	primaryKey := primaryName

	if preprocessProviderName == primaryName {
		// Same provider, different model — use synthetic keys.
		primaryKey = primaryName + "-primary"
		preprocessKey = preprocessProviderName + "-preprocess"
		providers[primaryKey] = primary
		providers[preprocessKey] = primary // same adapter, model override differentiates
	} else {
		providers[primaryKey] = primary
		preprocess, err := buildProvider(preprocessProviderName, credStore)
		if err != nil {
			return nil, fmt.Errorf("routing: preprocess provider: %w", err)
		}
		providers[preprocessKey] = preprocess
	}

	cfg := routing.RoutingConfig{
		Rules: []routing.RoutingRule{
			{Category: routing.CategoryPreprocess, Provider: preprocessKey, Model: preprocessModel},
			{Category: routing.CategoryPrimary, Provider: primaryKey},
		},
		DefaultProvider: primaryKey,
		Enabled:         true,
	}

	// Subscribe to routing decision events for transparency.
	eventBus.Subscribe("routing.decision", func(_ context.Context, ev core.Event) {
		if re, ok := ev.(*routing.RoutingDecisionEvent); ok {
			model := re.SelectedModel
			if model == "" {
				model = "(default)"
			}
			fmt.Fprintf(os.Stderr, "[routing] → provider=%s model=%s category=%s\n",
				re.SelectedProvider, model, re.Category)
		}
	})

	return routing.NewRoutingProvider(routing.RoutingProviderConfig{
		Providers:       providers,
		Policy:          routing.NewConfigPolicy(cfg),
		DefaultProvider: primaryKey,
		EventBus:        eventBus,
	}), nil
}

// buildProvider creates a provider adapter by name.
func buildProvider(name string, credStore core.CredentialStore) (core.Provider, error) {
	switch name {
	case "anthropic":
		return anthropic.New(credStore), nil
	case "openai":
		return openai.New(credStore), nil
	case "ollama":
		return ollama.New(credStore), nil
	case "openrouter":
		return openrouter.New(credStore), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: anthropic, openai, ollama, openrouter)", name)
	}
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
