# Known Issues — Simply Devly (Alpha)

This document lists known issues, limitations, and workarounds for the current alpha release.
Issues are updated as bugs are fixed or workarounds are discovered.

---

## Providers

### Kimi (Moonshot AI) — Tool calls fail on multi-turn sessions

**Affects:** `siply` with `SIPLY_PROVIDER=kimi`
**Symptom:** `API error 400: Invalid request: the message at position N with role 'assistant' must not be empty`
**When it happens:** After the first tool call (e.g. writing a file), the second API request fails.
**Root cause:** Kimi's API rejects assistant messages that contain tool calls without text content. Siply's current message format produces this on turn 2+.
**Workaround:** Use Kimi via OpenRouter instead (`SIPLY_PROVIDER=openrouter`, `SIPLY_MODEL=moonshotai/kimi-k2`). OpenRouter normalizes the message format transparently.
**Status:** Fix planned (story backlog).

---

### OpenAI — Lower cache hit rate than Anthropic

**Affects:** `siply` with `SIPLY_PROVIDER=openai`
**Symptom:** Cache hit rate is ~10–15% on short tasks vs. ~93% on Anthropic.
**Root cause:** OpenAI uses automatic prefix caching (server-side, no client control). Only the stable prefix (system prompt + tools) is cached — the growing conversation history is not. Anthropic allows explicit cache breakpoints at arbitrary positions including conversation history.
**Impact:** OpenAI runs cost more than Anthropic runs of comparable complexity.
**Workaround:** Use Anthropic (`SIPLY_PROVIDER=anthropic`) for cost-sensitive tasks.
**Status:** Known limitation. Further optimization planned (PC-3.2 system prompt size reduction).

---

### Google OAuth — Login not available

**Affects:** `siply login --google`
**Symptom:** Google login is not implemented.
**Workaround:** Use GitHub login (`siply login`) — GitHub Device Flow is fully supported.
**Status:** Planned for a future release.

---

## Security

### JWT signature verification — Not yet enforced

**Affects:** License validator (Pro features)
**Detail:** JWT tokens from simply-market are validated for expiry and required claims, but cryptographic signature verification is not yet implemented. This is deferred until the simply-market JWKS endpoint is available.
**Impact:** Alpha users are not affected — Pro features are not yet live.
**Status:** Will be enforced before Pro launch.

---

## Providers — Cost Estimation

### Non-ASCII text — Token count estimate may be inaccurate

**Affects:** System prompt caching threshold detection for Chinese, Japanese, Korean, and emoji-heavy text.
**Symptom:** For CJK text, `estimateTokens()` may overestimate token count by up to 30% (byte-based, not character-based). This means the cache_control threshold may be triggered slightly early or late.
**Impact:** Minor cost difference. Caching still works — just the threshold detection is approximate.
**Workaround:** No workaround needed; the estimation is conservative (overestimates = cache is applied earlier = no missed savings).
**Status:** Fix planned (PC-3.2).

### SIPLY_MODEL — Whitespace in model name causes API error

**Affects:** Users setting `SIPLY_MODEL` environment variable with trailing/leading spaces.
**Symptom:** `API error: model not found` or similar from the provider.
**Workaround:** Ensure no whitespace around the model name: `export SIPLY_MODEL=claude-sonnet-4-6`
**Status:** Fix planned (PC-3.2).

---

## Plugin System

### Google login required for marketplace

**Affects:** `siply marketplace` (not yet public)
**Detail:** The marketplace and plugin publishing flow are not yet available in the alpha. Plugin install from local YAML configs and Go plugins via gRPC work fully.
**Status:** Marketplace planned for a future milestone.

---

## Performance

### First request in a session — No cache hits (expected)

**Affects:** All providers with prompt caching.
**Detail:** The first API call in any session always has 0 cache hits — this is normal. Caching kicks in from the second call onwards. Cold start is unavoidable by design.
**Impact:** For very short one-shot tasks (single request), caching provides no benefit.
**Status:** Expected behavior, not a bug.

---

## Reporting Issues

Found something not on this list? Please open an issue:
**GitHub:** [github.com/simplydevly/siply/issues](https://github.com/simplydevly/siply/issues)
**Discord:** Join the Simply Devly server and post in `#bug-reports`

When reporting, include:
- Output of `siply --version`
- Your `SIPLY_PROVIDER` and `SIPLY_MODEL` settings
- Steps to reproduce
- Error message or unexpected behavior
