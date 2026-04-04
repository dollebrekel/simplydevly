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
	"siply.dev/siply/internal/tools"
)

// ansiPattern matches ANSI escape sequences for stripping in non-TTY mode.
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func newRunCmd() *cobra.Command {
	var taskFlag string
	var yoloFlag, autoAcceptFlag bool

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run a one-shot task non-interactively",
		RunE: func(cmd *cobra.Command, args []string) error {
			if taskFlag == "" {
				return fmt.Errorf("run: --task flag is required")
			}
			return executeRun(cmd.Context(), taskFlag, yoloFlag, autoAcceptFlag)
		},
	}
	cmd.Flags().StringVar(&taskFlag, "task", "", "Task description to execute")
	cmd.Flags().BoolVar(&yoloFlag, "yolo", false, "Skip all permission confirmations")
	cmd.Flags().BoolVar(&autoAcceptFlag, "auto-accept", false, "Auto-accept non-destructive actions")
	_ = cmd.MarkFlagRequired("task")
	return cmd
}

func executeRun(ctx context.Context, task string, yolo, autoAccept bool) error {
	// Bootstrap credential store.
	credStore := &credential.EnvStore{}

	// Bootstrap provider.
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
		providerName = "anthropic"
	}

	switch providerName {
	case "anthropic":
		return anthropic.New(credStore), nil
	case "openai":
		return openai.New(credStore), nil
	case "ollama":
		return ollama.New(credStore), nil
	case "openrouter":
		return openrouter.New(credStore), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: anthropic, openai, ollama, openrouter)", providerName)
	}
}

// stripANSI removes ANSI escape sequences from a string.
func stripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}
