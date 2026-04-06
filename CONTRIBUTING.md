# Contributing to Simply Devly

Welcome! We're glad you're interested in contributing to Simply Devly. This document explains the process for contributing to this project.

## Developer Certificate of Origin (DCO)

This project uses the [Developer Certificate of Origin](https://developercertificate.org/) (DCO) instead of a Contributor License Agreement (CLA). By contributing, you certify that you have the right to submit the work under the project's open-source license.

### How to sign off your commits

Add a `Signed-off-by` line to every commit message:

```
feat: add new provider adapter

Signed-off-by: Your Name <your.email@example.com>
```

Git can do this automatically with the `-s` flag:

```bash
git commit -s -m "feat: add new provider adapter"
```

If you forgot to sign off, you can amend your last commit:

```bash
git commit --amend -s
```

> **Note:** All commits must be signed off. PRs with unsigned commits will not be merged.

## Getting Started

### Prerequisites

- Go 1.22 or later
- Make

### Development Setup

```bash
# Clone the repository
git clone https://github.com/simplydevly/simplydevly.git
cd simplydevly

# Build the project
make build

# Run tests
make test

# Run linter
make lint
```

## How to Contribute

### Reporting Issues

- Search existing issues before creating a new one
- Use the issue templates when available
- Include reproduction steps, expected behavior, and actual behavior

### Submitting Changes

1. **Fork** the repository
2. **Create a branch** from `main` for your change:
   ```bash
   git checkout -b feat/your-feature
   ```
3. **Make your changes** — keep commits focused and atomic
4. **Run tests** before pushing:
   ```bash
   make test
   make lint
   make license-check
   ```
5. **Sign off** all commits (see DCO section above)
6. **Open a Pull Request** against `main`

### PR Process

- PRs are reviewed by maintainers
- All CI checks must pass (tests, lint, license headers)
- Keep PRs focused — one feature or fix per PR
- Update documentation if your change affects public APIs

## Code Style

- Follow existing patterns in the codebase
- Run `golangci-lint` before submitting — the project's `.golangci.yml` defines the rules
- All new `.go` files must include the Apache 2.0 SPDX license header:
  ```go
  // SPDX-License-Identifier: Apache-2.0
  // Copyright 2026 Simply Devly contributors
  ```

## Code of Conduct

Be respectful and constructive. We follow the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/) code of conduct.

## Questions?

Open a [discussion](https://github.com/simplydevly/simplydevly/discussions) or an issue — we're happy to help.
