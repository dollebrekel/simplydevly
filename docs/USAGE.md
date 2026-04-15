# Usage Guide — siply

This guide covers the most common ways to use siply in your day-to-day development workflow.

---

## Table of Contents

- [Interactive Mode](#interactive-mode)
- [One-Shot Mode](#one-shot-mode)
- [Commands Reference](#commands-reference)
- [Tools](#tools)
- [Plugins](#plugins)
- [Keybindings](#keybindings)
- [Configuration](#configuration)
- [Tips and Workflows](#tips-and-workflows)

---

## Interactive Mode

Start siply with no arguments to enter the interactive TUI:

```bash
siply
```

The TUI has four main areas:

| Area | Description |
|------|-------------|
| **Chat panel** | Your conversation with the AI agent |
| **Activity feed** | Live feed of every tool call the agent makes |
| **Diff view** | Shows file changes before they are applied |
| **Status bar** | Provider, model, token usage, session info |

Type your request in the input field and press `Enter`. The agent responds and uses tools to get things done.

### Starting a session in a specific project

```bash
# Start siply from within your project directory
cd /path/to/your/project
siply
```

siply automatically detects a `siply.yaml` in the project root and applies project-specific settings.

---

## One-Shot Mode

Run a task without the interactive TUI and exit when done:

```bash
siply run --task "add error handling to all functions in cmd/"
```

Useful for:
- Scripting or automation
- Quick tasks where you don't need to continue a conversation
- CI/CD pipelines (use with care — review changes)

```bash
# Use a specific provider for this run
siply run --task "refactor auth.go to use the new interface" --provider anthropic

# Pipe output to a file
siply run --task "generate documentation for internal/api/" > output.md
```

---

## Commands Reference

```
siply [flags]                    Start interactive mode
siply run --task "..."           Run a one-shot task
siply config                     Show current configuration
siply config set <key> <value>   Update a config value
siply version                    Show version information
siply completion <shell>         Generate shell completion script
```

### Global Flags

```
--provider string    AI provider to use (anthropic, openai, ollama, openrouter, kimi)
--model string       Model name to use
--workspace string   Working directory (default: current directory)
--debug              Enable debug output
```

---

## Tools

Every tool call is shown in the activity feed as the agent runs. You always know what the agent is doing.

### Read
Reads a file and returns its contents.
```
Tool: Read
  path: internal/auth/handler.go
  lines: 1-50
```

### Write
Creates or overwrites a file.
```
Tool: Write
  path: internal/auth/handler_new.go
```

### Edit
Makes a targeted string replacement in a file. Safer than Write for small changes.
```
Tool: Edit
  path: internal/auth/handler.go
  old: "func Login("
  new: "func Authenticate("
```

### Bash
Runs shell commands. Commands that look dangerous require your explicit approval.
```
Tool: Bash
  command: go test ./...
```

### Glob
Finds files matching a pattern.
```
Tool: Glob
  pattern: internal/**/*.go
```

### Grep
Searches file contents with a regex pattern.
```
Tool: Grep
  pattern: "func.*Handler"
  glob: "**/*.go"
```

---

## Permission System

Before the agent runs potentially dangerous commands, it asks for your permission.

There are three permission levels:
- **allow** — run without asking
- **ask** — prompt before running (default for Bash)
- **deny** — never run

You can set permissions per command class in your config:

```yaml
# ~/.siply/config.yaml
permissions:
  bash_network: ask      # curl, wget, etc.
  bash_system: deny      # rm -rf, sudo, etc.
  bash_build: allow      # go build, make, etc.
```

---

## Plugins

### Installing a Plugin

```bash
# Install a plugin from the registry
siply plugin install model-presets

# Install from a local path
siply plugin install ./my-plugin.yaml
```

### Listing Installed Plugins

```bash
siply plugin list
```

### Updating Plugins

```bash
siply plugin update model-presets
siply plugin update --all
```

### YAML Config Plugins

YAML plugins can define model presets, prompt templates, and keybinding overrides. Example:

```yaml
# my-preset.yaml
name: fast-mode
description: Use fast, cheap models for quick tasks
config:
  provider: openai
  model: gpt-4o-mini
  max_tokens: 2000
```

---

## Keybindings

| Key | Action |
|-----|--------|
| `Enter` | Send message |
| `Ctrl+C` | Cancel current operation |
| `Ctrl+D` | Exit siply |
| `Tab` | Switch focus between panels |
| `?` | Show keybinding help overlay |
| `Ctrl+L` | Clear chat |
| `↑` / `↓` | Scroll activity feed |
| `d` | Toggle diff view |

---

## Configuration

### Global Config

Located at `~/.siply/config.yaml`:

```yaml
default_provider: anthropic
default_model: claude-sonnet-4-6

providers:
  anthropic:
    model: claude-sonnet-4-6
    max_tokens: 8096
  openai:
    model: gpt-4o
    max_tokens: 4096
  ollama:
    host: http://localhost:11434
    model: llama3.2

permissions:
  bash_network: ask
  bash_system: deny
  bash_build: allow

ui:
  theme: default
  show_token_usage: true
```

### Project Config

Place `siply.yaml` in your project root to override settings per project:

```yaml
# myproject/siply.yaml
provider: anthropic
model: claude-sonnet-4-6
workspace: ./src
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ANTHROPIC_API_KEY` | Anthropic API key |
| `OPENAI_API_KEY` | OpenAI API key |
| `OPENROUTER_API_KEY` | OpenRouter API key |
| `KIMI_API_KEY` | Kimi (Moonshot) API key |
| `SIPLY_CONFIG` | Override config file path |
| `SIPLY_PROVIDER` | Override default provider |
| `SIPLY_MODEL` | Override default model |

---

## Tips and Workflows

### Refactoring a function

```
You: refactor the parseConfig function in config/loader.go to return an error instead of panicking
```

siply will read the file, understand the context, make the changes, and show you a diff before applying.

### Adding tests

```
You: write unit tests for the functions in internal/auth/validator.go, following the existing test style in the project
```

siply reads the existing tests first to match your style.

### Understanding unfamiliar code

```
You: explain how the plugin loading system works, trace the flow from siply plugin install to the plugin being active
```

### Code review

```
You: review the changes in internal/providers/openai/ and flag any issues with error handling or concurrency
```

### Working with Ollama (local models)

For tasks that don't need the strongest model, local models are free and fast:

```bash
siply --provider ollama --model qwen2.5-coder:7b
```

---

## Session Management

siply saves your conversation history per project. To resume where you left off:

```bash
siply resume
```

To start fresh:
```bash
siply --new-session
```

---

## More Help

- [Installation Guide](INSTALL.md)
- [Roadmap](ROADMAP.md)
- [GitHub Issues](https://github.com/dollebrekel/simplydevly/issues) — bug reports
- [GitHub Discussions](https://github.com/dollebrekel/simplydevly/discussions) — questions and feature requests
