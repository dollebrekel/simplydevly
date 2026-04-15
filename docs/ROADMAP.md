# Roadmap — Simply Devly

Honest roadmap for a solo developer shipping in public. Items without a timeline are on the list, not on a deadline.

---

## What exists today (Alpha)

The core agent is fully working:

- Interactive TUI with streaming output and live activity feed
- One-shot task mode (`siply run --task "..."`)
- Multi-provider support — Anthropic, OpenAI, Ollama (local/free), OpenRouter, Kimi
- Full tool system — Read, Write, Edit, Bash, Glob, Grep
- Inline diff view for all file changes
- Permission system with security checks on shell commands
- YAML config plugins and compiled Go plugins (via gRPC)
- Plugin install, update, rollback, pin
- Project and global config with workspace management
- Shell completion (bash / zsh / fish)
- GitHub OAuth login (Device Flow)
- Basic memory — the agent retains context within a project session
- Prompt caching on all supported providers (reduces token costs on longer sessions)
- Open core architecture — the foundation for free vs. Pro is in place

---

## Coming Next

The following are actively planned. No fixed dates — this is a solo project.

### Plugin Marketplace (`simply-market`)

A place to discover, install, and share plugins. Browse model presets, prompt templates, and tool extensions made by the community. Planned alongside a `siply publish` command so anyone can contribute.

### More AI Providers

Google Gemini, Mistral, and others based on what the community asks for.

### Binary releases & package managers

Proper automated releases for Linux and macOS. Homebrew formula and AUR package so installation is just `brew install siply` or `yay -S siply`.

### Stability and polish

Alpha means rough edges. Improving error messages, edge cases, and the first-run experience before any wider launch.

---

## Further Out

These are on the horizon but not the current focus:

### Skills and agent configs

Save and share your siply setup — model preferences, custom prompts, keybindings — as installable profiles.

### Extension system

A proper panel and extension API so the TUI can be extended with custom views. simply-ui (a lightweight web dashboard) lives here too.

### Cost optimisation (Pro)

We're working on significant reductions in cloud token costs through smarter routing and local processing. The details are still being figured out, but the goal is to make Pro pay for itself. More when it's ready.

### Scheduled and automated tasks

Run siply on a schedule or as part of a pipeline. Daemon mode for longer-running workflows.

### Pro tier

A paid tier for teams and power users who need more. The free core agent stays free and open source — always.

---

## Desktop App

A native desktop version of Simply Devly is planned for the future. The goal is full Windows support — without requiring WSL2 or a terminal emulator setup. Same agent, same tools, but with native file system access and a proper desktop UI.

This is further out on the horizon. The terminal version comes first.

---

## Not Planned

To keep Simply Devly focused:

- **No hosted version** — your code stays on your machine
- **No telemetry by default** — opt-in only, if at all

---

## Feedback

The roadmap moves based on what people actually need. Open a [GitHub Discussion](https://github.com/dollebrekel/simplydevly/discussions) to suggest or vote on features.

---

*Last updated: April 2026*
