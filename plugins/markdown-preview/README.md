# markdown-preview

A Siply Tier 3 plugin that renders markdown files in a right-side panel.

## Features

- Renders `.md` files using the siply-ui MarkdownView component
- Headings, code blocks, lists, tables, horizontal rules, bold, italic, links
- Subscribes to `FileSelectedEvent` — automatically previews `.md` files selected from the tree-local panel
- Lazy-loaded — no filesystem I/O until the panel is first opened
- Shows placeholder text when no file is selected

## Usage

| Action | Key |
|--------|-----|
| Open markdown preview panel | `Ctrl+M` |
| Preview updates automatically | On file selection via tree-local |

## Installation

```bash
siply marketplace install markdown-preview
```

Or install as part of the standard profile:

```bash
siply profile install standard
```

## Author

Simply Devly — [siply.dev](https://siply.dev)
