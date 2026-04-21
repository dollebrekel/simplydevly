# tree-local

A Siply Tier 3 plugin that renders a local project file tree in a left-side panel.

## Features

- Browse the current working directory in a collapsible tree panel
- File type icons (Go, JavaScript, Markdown, Python, Rust, etc.)
- Git status indicators: `[M]` modified, `[A]` added, `[?]` untracked, `[D]` deleted
- Lazy-loaded — no filesystem scanning until the panel is first opened
- Publishes `FileSelectedEvent` when a file is selected, enabling integration with other plugins (e.g. markdown-preview) and the agent context

## Usage

| Action | Key |
|--------|-----|
| Open file tree panel | `Ctrl+T` |
| Navigate | Arrow keys |
| Select file | `Enter` |
| Expand/collapse dir | `Space` |
| Refresh tree | `r` |

## Installation

```bash
siply marketplace install tree-local
```

Or install as part of the standard profile:

```bash
siply profile install standard
```

## Author

Simply Devly — [siply.dev](https://siply.dev)
