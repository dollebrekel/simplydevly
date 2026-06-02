# Installation Guide — siply

This guide walks you through installing siply on Linux and macOS, and configuring it with your preferred AI provider.

---

## Requirements

- Linux (x86_64 or arm64) or macOS (Intel or Apple Silicon)
- A terminal
- At least one API key **or** [Ollama](https://ollama.com) installed locally (free)

> **Windows** — not officially tested yet. It may work under WSL2.

---

## Option 1: Install Script (Recommended)

The fastest way to get started:

```bash
curl -sSL https://raw.githubusercontent.com/dollebrekel/simplydevly/main/scripts/install.sh | sh
```

This downloads the latest binary for your platform, places it in `/usr/local/bin/siply`, and verifies the checksum.

After installing, verify:
```bash
siply --version
```

---

## Option 2: Download Binary Manually

Download the latest release from the [Releases page](https://github.com/dollebrekel/simplydevly/releases).

**Linux (x86_64):**
```bash
curl -L https://github.com/dollebrekel/simplydevly/releases/latest/download/siply-linux-amd64 -o siply
chmod +x siply
sudo mv siply /usr/local/bin/
```

**Linux (arm64):**
```bash
curl -L https://github.com/dollebrekel/simplydevly/releases/latest/download/siply-linux-arm64 -o siply
chmod +x siply
sudo mv siply /usr/local/bin/
```

**macOS (Apple Silicon):**
```bash
curl -L https://github.com/dollebrekel/simplydevly/releases/latest/download/siply-darwin-arm64 -o siply
chmod +x siply
sudo mv siply /usr/local/bin/
```

**macOS (Intel):**
```bash
curl -L https://github.com/dollebrekel/simplydevly/releases/latest/download/siply-darwin-amd64 -o siply
chmod +x siply
sudo mv siply /usr/local/bin/
```

---

## Option 3: Build from Source

Requires Go 1.22 or later.

```bash
# Clone the repository
git clone https://github.com/dollebrekel/simplydevly.git
cd simplydevly

# Build
make build

# Install
sudo mv bin/siply /usr/local/bin/
```

---

## Configuring an AI Provider

siply needs at least one AI provider to work. You have two choices:

### Option A: Use a Cloud Provider (requires API key)

**Anthropic (Claude)** — recommended for best results:
```bash
export ANTHROPIC_API_KEY=your-key-here
```

**OpenAI:**
```bash
export OPENAI_API_KEY=your-key-here
```

**OpenRouter** — access 200+ models with one key:
```bash
export OPENROUTER_API_KEY=your-key-here
```

**Kimi (Moonshot AI):**
```bash
export KIMI_API_KEY=your-key-here
```

To make environment variables persist, add the export line to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.).

---

### Option B: Use Ollama (Local, Free)

Ollama lets you run AI models on your own machine at no cost. No API key needed.

**Install Ollama:**
```bash
curl -fsSL https://ollama.com/install.sh | sh
```

**Pull a model:**
```bash
# A good starting model (7B, fast on most machines)
ollama pull llama3.2

# Larger model for better results (requires ~8GB RAM)
ollama pull qwen2.5-coder:7b
```

**Use it with siply:**

Ollama runs on `http://localhost:11434` by default. siply detects it automatically when you select Ollama as your provider.

---

## First Run

```bash
siply
```

On the first run, siply will ask you to select a provider and model. Your choice is saved to `~/.siply/config.yaml`.

**Or specify directly:**
```bash
# Use Anthropic with Claude Sonnet
siply --provider anthropic --model claude-sonnet-4-6

# Use local Ollama
siply --provider ollama --model llama3.2

# Run a one-shot task without the interactive TUI
siply run --task "write a README for this project"
```

---

## Configuration File

siply stores its configuration in `~/.siply/config.yaml`. You can edit it directly or use the `siply config` commands.

Key settings:

```yaml
# ~/.siply/config.yaml
default_provider: anthropic
default_model: claude-sonnet-4-6

providers:
  anthropic:
    model: claude-sonnet-4-6
  openai:
    model: gpt-4o
  ollama:
    host: http://localhost:11434
    model: llama3.2
```

---

## Project-Level Config

You can place a `siply.yaml` in any project directory to override global settings for that project:

```yaml
# myproject/siply.yaml
provider: anthropic
model: claude-sonnet-4-6
workspace: ./
```

---

## Shell Completion

Enable tab completion for siply commands:

**Bash:**
```bash
siply completion bash >> ~/.bashrc
source ~/.bashrc
```

**Zsh:**
```bash
siply completion zsh > "${fpath[1]}/_siply"
```

**Fish:**
```bash
siply completion fish > ~/.config/fish/completions/siply.fish
```

---

## Uninstall

```bash
sudo rm /usr/local/bin/siply
rm -rf ~/.siply
```

---

## Troubleshooting

**`siply: command not found`**
Make sure `/usr/local/bin` is in your `$PATH`:
```bash
echo $PATH | grep -q "/usr/local/bin" && echo "OK" || echo "Add /usr/local/bin to PATH"
```

**API key not found:**
siply looks for keys in environment variables first, then in `~/.siply/credentials.yaml`. Make sure your key is exported correctly.

**Ollama connection refused:**
Make sure Ollama is running:
```bash
ollama serve
```

**Permission denied on binary:**
```bash
chmod +x /usr/local/bin/siply
```

---

For more help, open an [issue on GitHub](https://github.com/dollebrekel/simplydevly/issues).
