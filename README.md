# Simply Devly

> Terminal-native AI coding agent. Transparent. Extensible.

[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22%2B-00ADD8?logo=go)](https://golang.org)
[![Status: Alpha](https://img.shields.io/badge/Status-Alpha-orange)]()

**Simply Devly** is an open-source AI coding agent that runs entirely in your terminal. CLI command: `siply`.

Unlike most AI coding tools, Simply Devly shows you every action it takes — every file read, every edit, every tool call — in real time. You always know what the agent is doing.

No IDE required. No Electron. No browser. Just your terminal.

---

## Why Simply Devly?

Most AI coding tools hide what they're doing. Simply Devly doesn't.

- **Transparent** — live feed of every tool call as it happens
- **Extensible** — plugin system with YAML config plugins and compiled Go plugins
- **Terminal-native** — full TUI, works anywhere your terminal does
- **Your API keys** — connect your own provider; your code stays on your machine

---

## What's in the Alpha

### Multi-Provider Support

Connect your preferred AI provider with your own API key:

- **Anthropic** (Claude Sonnet 4.6, Opus 4.6, Haiku)
- **OpenAI** (GPT-4o, GPT-4.1)
- **Ollama** — run models locally for free (Llama, Qwen, Mistral, and more)
- **OpenRouter** — one key, access to many models
- **Kimi** (Moonshot AI)

Prompt caching is active on all supported providers, which reduces costs on longer sessions.

### Tool System

The agent uses these tools to do its work. Every call is visible in the activity feed:

| Tool | What it does |
|------|-------------|
| `Read` | Read file contents |
| `Write` | Create or overwrite a file |
| `Edit` | Make a targeted change in a file |
| `Bash` | Run shell commands |
| `Glob` | Find files by pattern |
| `Grep` | Search file contents |

### Terminal UI

- Live activity feed — see every tool call as it runs
- Inline diff view — review file changes before they're applied
- Status bar with provider info and token usage
- Markdown rendering in the terminal
- Menu overlays and keybinding help

### Plugin System

- **YAML config plugins** — model presets, prompt templates, custom keybindings
- **Compiled Go plugins** — full extensions loaded via gRPC for advanced use cases
- Install, update, rollback, and pin plugin versions

### Other

- One-shot mode: `siply run --task "..."` — run a task and exit
- Permission system — dangerous commands require explicit approval
- Workspace and project-level config
- Credential storage — local only, never sent anywhere
- Shell completion (bash / zsh / fish)
- GitHub OAuth login (Device Flow)
- Memory system — the agent can store and recall context within a project

---

## Quick Start

### Install

```bash
curl -sSL https://raw.githubusercontent.com/dollebrekel/simplydevly/main/scripts/install.sh | sh
```

Or build from source (requires Go 1.22+):

```bash
git clone https://github.com/dollebrekel/simplydevly.git
cd simplydevly
make build
sudo mv bin/siply /usr/local/bin/
```

### Set an API key

```bash
# Anthropic (recommended)
export ANTHROPIC_API_KEY=your-key-here

# Or use Ollama for free local models
curl -fsSL https://ollama.com/install.sh | sh
ollama pull llama3.2
```

### Run

```bash
# Interactive mode
siply

# One-shot task
siply run --task "explain what this codebase does"
```

See the [Installation Guide](docs/INSTALL.md) for full setup including Ollama.

---

## The Simply Ecosystem

Simply Devly (`siply`) is the core of a broader set of tools:

| Project | Description | Status |
|---------|-------------|--------|
| **Simply Devly** (this repo) | AI coding agent — CLI: `siply` | Alpha |
| [simply-bench](https://github.com/dollebrekel/simply-bench) | Benchmark tool — compare AI coding agents head-to-head | Alpha |
| simply-market | Plugin marketplace | Planned |
| simply-ui | Web dashboard | Planned |

---

## Documentation

| Document | Description |
|----------|-------------|
| [Install Guide](docs/INSTALL.md) | Full installation for Linux, macOS, from source |
| [Usage Guide](docs/USAGE.md) | Commands, configuration, workflows |
| [Roadmap](docs/ROADMAP.md) | What's coming next |
| [Contributing](CONTRIBUTING.md) | How to contribute |

---

## Project Status

**Simply Devly is in alpha.** The core agent works and has been used in real development workflows. Expect rough edges. The plugin API may change before v1.0. Windows support has not been tested yet.

Bug reports go in [Issues](https://github.com/dollebrekel/simplydevly/issues). Feature requests and questions go in [Discussions](https://github.com/dollebrekel/simplydevly/discussions).

---

## Free and Open Source

Simply Devly is free and open source under Apache 2.0. A Pro tier is planned for the future with advanced features. The free core agent will always remain open source.

---

## License

Apache 2.0 — see [LICENSE](LICENSE).

---

## Contributing

Contributions are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR. All commits must be signed off (DCO).

---

*Built by a solo developer. Feedback and bug reports make this better.*
