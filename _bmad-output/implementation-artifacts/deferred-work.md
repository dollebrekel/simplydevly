# Deferred Work

## Deferred from: code review of story 11-2 (2026-04-21)

- **computeLCS is O(n*m) time and memory, no size guard** — For large files (10k+ lines), the LCS algorithm allocates ~400MB. Needs Myers diff or size-capped fallback. [pkg/siplyui/diffview.go:computeLCS]
- **Form Values() silently overwrites duplicate labels** — If two fields share the same Label(), only the last value appears. Consider returning an error or using a different key strategy. [pkg/siplyui/form.go:Values]
- **tokenFromColor uses same color value for all depth levels** — Color256 and Color16 get the same TrueColor value. Requires proper color palette degradation mapping. [pkg/siplyui/theme.go:tokenFromColor]
- **NFR32: no version marker or semver annotation for public API** — No version constant or compatibility annotation in pkg/siplyui/. Track for API stability story. [pkg/siplyui/]
- **Bridge enum conversion without validation** — Direct type casting in BridgeRenderConfig without checking that enum values match between internal and public types. Low risk while enums stay in sync. [internal/tui/siplyui_bridge.go]
- **Card description not wrapped, only truncated** — Long descriptions are rendered as single line; no word-wrap for marketplace display. [pkg/siplyui/card.go:RenderCard]

## Deferred from: code review of story 11-3 (2026-04-21)

- **toTitleCase mutates input slice backing array** — `filtered := parts[:0]` shares backing array with `parts`. Works today but fragile if loop logic changes. [internal/extensions/scaffold.go:138-139]
- **AllKeybindings() called on every keypress** — Acquires read lock per keypress to iterate all registrations. Performance concern under many extensions. Optimize with cached keybind map when perf becomes measurable. [internal/tui/app.go:369]
- **Manager allows registrations before Start()** — Registrations succeed but events not published (m.started guard). Current usage always calls Init→Start first. Add guard if lifecycle becomes complex. [internal/extensions/manager.go]
- **Publish errors silently discarded** — All eventBus.Publish() errors ignored with `_ =`. Acceptable degradation but makes debugging hard. Add structured logging when observability improves. [internal/extensions/manager.go:132,178,218,309,312]
- **Panel position not required in manifest** — Empty position passes validation, defaults to PanelLeft. Consider requiring explicit position in future schema version. [internal/plugins/manifest.go:281-282]
- **Scaffold name not validated against manifest namePattern** — Invalid names only caught at manifest validation, confusing error message. Add early name validation. [internal/extensions/scaffold.go:21-25]
- **DevWatcher reload with cancelled context** — reload() doesn't check ctx.Err() before proceeding. Edge case during shutdown. Add guard when reliability becomes critical. [internal/plugins/devwatcher.go:134]
- **Scaffold always generates ctrl+e keybind** — Two scaffolded extensions conflict at runtime. Generate unique keybinds or omit default keybind. [internal/extensions/scaffold.go:101]
- **AC4: PluginLoaded auto-registration + EventBus-to-BubbleTea bridge + TUI refresh** — handlePluginLoaded is a stub (no auto-registration from manifest), no EventBus→tea.Program.Send() bridge, MenuChangedMsg/KeybindChangedMsg handlers are no-ops. Non-trivial: requires tea.Program threading + manifest access in event handler. Core registration lifecycle works via Go API; bridge is enhancement for future story. [internal/extensions/manager.go:359-361, internal/tui/app.go:203-207]

## Deferred from: code review of 11-4-tier-2-lua-plugin-support (2026-04-21)

- **Registry dubbele manifest load TOCTOU** — LocalRegistry.Load() parst manifest, dan Tier2Loader.Load() parst opnieuw van disk. Laag risico maar TOCTOU window. Pre-existing pattern uit Tier1/Tier3 loaders. [registry.go → tier2_loader.go]
- **httpClient geen connection/rate limiet per plugin** — Globale http.Client zonder MaxConnsPerHost of per-plugin rate limiting. Buggy plugin kan honderden connections openen. Vereist rate limiting design. [lua_http.go]
- **Cross-process state file corruptie** — stateFileMu beschermt alleen binnen één process. Twee siply instances schrijven naar dezelfde state.json zonder file locking. Vereist flock of advisory locking. [lua_api.go:244]
- **ContentFunc hardcodeert 80x24 dimensies** — PanelConfig.ContentFunc is `func() string` zonder width/height parameters. Uitbreiden naar `func(w, h int) string` vereist wijziging van alle panel implementaties. [lua_panels.go, internal/core/panel.go]
- **siply.panel.update() is een no-op** — Panel invalidation vereist TUI-side support (dirty-flag of event bridge van EventBus naar tea.Program). Gerelateerd aan AC4 PluginLoaded auto-registration deferred item. [lua_panels.go:121]

## Deferred from: code review of story 11-5 (2026-04-21)

- **Stale tree cache — no auto-refresh on filesystem changes** — By design per spec ("NO real-time file watching"). User must use "refresh" action. Consider TTL-based invalidation in future. [plugins/tree-local/main.go:302-309]
- **Git rename status code 'R' not shown** — `gitStatusIndicator('R')` falls to default empty string. Renamed files show no badge. Minor cosmetic gap. [plugins/tree-local/main.go:62-75]
- **Silently truncated trees at depth 10 with no indicator** — Deep subtrees return nil, parent dir appears empty. Consider adding "[truncated]" child node. [plugins/tree-local/main.go:113-114]
- **Ctrl+T keybinding not wired to tree-local panel** — TUI app.go:311 has placeholder. Requires panel activation flow, PanelManager integration, and position assignment (left). Related to AC4 deferred from 11-3. [internal/tui/app.go:311-313]
- **Panel position not configured for flagship extensions** — tree-local should be left, markdown-preview right. Depends on Ctrl+T wiring story above. [plugins/tree-local, plugins/markdown-preview]

## Deferred from: code review of story 12-1 (2026-04-24)

- **Cobra PersistentPreRunE chaining limitation** — If parent commands have their own PersistentPreRunE, Cobra only fires one (does not chain). The offline guard wraps PersistentPreRunE on guarded commands, which could silently skip the guard if a parent overrides it. Pre-existing Cobra limitation, not introduced by this change. [cmd/siply/offline_guard.go:26-38]

## Deferred from: code review of 12-2-provider-arbitrage-multi-provider-routing (2026-04-24)

- W1: **Event bus publish error silently discarded** — `provider.go:196` ignores eventBus.Publish() error. Pre-existing pattern across codebase (also noted in 11-3 review).
- W2: **Init/Start rollback map iteration order** — `provider.go:57-86` iterates map for rollback, non-deterministic order. Pre-existing code, not introduced by this diff.
- W3: **RoutingProvider no concurrency protection** — Fields set at construction, assumed immutable by convention. No mutex or documentation enforces this.
- W4: **Cost comparison equally weights input and output prices** — `cost_policy.go:81` sums input+output equally. Real workloads are input-heavy. Design choice, not a bug.
- W5: **SetOffline UTF-8 truncation** — `statusline.go:155-157` byte-slices model names, may break multi-byte UTF-8 runes. Pre-existing in SetOffline, not this diff.

## Deferred from: code review of 12-3-embedded-tree-sitter-go-python (2026-04-25)

- W1: **HookFailedEvent mist specifieke "full-context"/"higher" velden + statusbar (AC#8)** — Het hook systeem publiceert generieke HookFailedEvent bij failure, maar de spec vereist fallback="full-context", costEffect="higher", en statusbar "⚠ Code intelligence offline". Afhankelijk van D2 fix (AgentHooks verbinden met agent loop).
- W2: **Onbegrensde inotify watches op grote monorepos** — StartWatcher voegt elke directory toe aan fsnotify. Op monorepos met duizenden directories kan dit fs.inotify.max_user_watches uitputten (standaard 8192 op Linux). Overweeg inotify-limiet check of recursive-watcher strategie. [cache.go:132-153]
- W3: **Geen benchmark test voor <30ms Python parsing (AC#2)** — Geen performance test die valideert dat 10K-regel Python bestanden in <30ms geparsed worden. Tree-sitter is snel genoeg in de praktijk, maar geen test afdwinging.
- W4: **Plugin binary grootte niet geverifieerd (AC#4)** — Spec vereist ~3-5 MB. -ldflags "-s -w" aanwezig maar geen CI check. [Makefile:plugin-build-tree-sitter]

## Deferred from: code review of story 12-4 (2026-04-25)

- F1: **Cache maxSize=0 causes index-out-of-range panic** — NewCache(0) → Put() tries c.order[0] on empty slice. Latent: currently hardcoded to 100, only fires if maxSize becomes configurable. [plugins/context-distillation/cache.go:37]
- F2: **Concurrent Initialize leaks gRPC connections on partial failure** — If grpc.NewClient succeeds but a later step in Initialize() fails, p.hostConn is set but p.initialized remains false. A retry overwrites p.hostConn without closing the old connection. Rare scenario. [plugins/context-distillation/main.go:87-95]

## Deferred from: code review of 12-5-persistent-session-intelligence (2026-04-25)

- W1: **Consolidation triggers frequently after threshold** — Once distillate count exceeds maxDistillates, consolidation runs on nearly every session-end (replaces N with 1, count drops to 2, increments back to N+1 within N sessions). By-design behavior. [plugins/session-intelligence/consolidator.go]
- W2: **No concurrent tests + race detector disabled** — CGO_ENABLED=0 prevents -race flag. Mutex logic in handlePreQuery untested under concurrency. Test infrastructure concern. [Makefile]
- W3: **estimateTokens crude byte/4 approximation** — len(s)/4 is inaccurate for non-ASCII/CJK content. Known limitation, matches context-distillation pattern. [plugins/session-intelligence/distiller.go]
- W4: **parseDistillateContent fragile JSON extraction** — first { to last } can span unrelated content if LLM wraps response. Best-effort extraction, acceptable for LLM output. [plugins/session-intelligence/distiller.go]
- W5: **No typed SessionIntelligenceEvent struct in events/types.go** — Event published as raw JSON via gRPC, no corresponding Go struct. Works but inconsistent with other typed events. [internal/events/types.go]

## Deferred from: code review of 12-6-execution-sandbox-linux-macos (2026-04-25)

- W1: **BuildOptions deduplicates across read/write — write path silently dropped** — If same path in both `ExtraReadPaths` and `ExtraWritePaths`, it becomes read-only. The more permissive (write) should win. Pre-existing in config.go design. [internal/sandbox/config.go:69-80]
- W2: **cleanupCgroup uses os.Remove on non-empty cgroup directory** — cgroup dirs contain kernel pseudo-files; `os.Remove` always fails with ENOTEMPTY. Needs `os.RemoveAll` or proper cgroup teardown. Pre-existing cgroup lifecycle issue. [internal/sandbox/sandbox_linux.go:337-339]

## Deferred from: code review of 12-7-checkpoint-rewind (2026-04-25)

- **readMeta full JSON deserialization** — `checkpoint.Manager.List()` reads and deserializes the entire checkpoint JSON for each step file just to extract metadata (timestamp, tool name, message count, file count). For sessions with many checkpoints and large conversation histories, this is O(n × message_size) I/O. Could be optimized with a separate metadata header, streaming JSON parser, or SQLite index. Low priority — only affects CLI `checkpoint list` latency, not agent loop performance. [internal/checkpoint/manager.go:305-333]

## Deferred from: code review of 12-9-tui-agent-integration (2026-04-26)

- **EventBus Message Ordering Race** — AgentOutputMsg (via EventBus async subscriber) and AgentDoneMsg (via tea.Cmd return) may race. Late text chunks could arrive after done signal. Pre-existing EventBus design limitation. [cmd/siply/tui.go bridgeEventBus]
- **Ollama Probe Uses nil CredStore** — Probe created with `ollama.New(nil)` while real provider uses credStore. Behavior could diverge if credentials affect Ollama behavior. Pre-existing from story 12-1. [cmd/siply/tui.go:89]
- **Non-Local --model Flag Silently Ignored** — `--model` without `--local` sets ModelOverride to empty string. Same pattern as run.go (intentional). No CLI warning. [cmd/siply/tui.go:735-745]
- **Test Coverage Gaps** — No tests for streaming EventBus→REPL flow, Ollama routing, or multi-turn conversation. All 11 tests are unit-level with mockAgentRunner. [internal/tui/app_test.go]
- **AC1: Status bar spinner/token count** — AC1 vereist spinner/token count in status bar tijdens streaming. NoopStatusCollector gebruikt. UX-polish, verdient eigen story. [cmd/siply/tui.go bootstrapTUIAgent, internal/tui/statusline/]
- **Rapid Submit Race** — Geen guard tegen dubbel indienen terwijl agent loopt. Error is niet silent maar UX is verwarrend. Hoort bij input-state management story. [internal/tui/app.go:160]

## Deferred from: code review of story 12-10 (2026-04-26)

- **executeBuiltinCommand blokkeert main goroutine** — `cmd.Run()` is blocking zonder timeout/context. Als subprocess hangt, bevriest de hele TUI. Hoort bij async command execution story. [internal/tui/panels/repl.go:636-640]
- **Error dedup scant volledige message history** — Bij command failure wordt alle 2000 messages gescand op "Error:" prefix, niet alleen output van huidige command. Stale errors van eerdere commands kunnen nieuwe errors onderdrukken. [internal/tui/panels/repl.go:656-668]

## Deferred from: code review of story 12-11 (2026-04-27)

- **Tool block border breedte mismatch top vs bottom** — Top border berekent tot width-1, bottom tot width-2 visual chars. 1-char cosmetisch verschil. [internal/tui/panels/repl.go:436-476]
- **Token counter telt runes niet LLM tokens** — `len([]rune(msg.Text))` telt Unicode karakters maar display zegt "tokens". By-design benadering, story notes bevestigen dit. [internal/tui/panels/repl.go:189]
- **Lange tool namen niet afgekapt in renderToolBlock** — Tool naam langer dan panel breedte veroorzaakt visuele overflow. Zeldzaam edge case. [internal/tui/panels/repl.go:452-453]

## Deferred from: code review of story 12-11, round 2 (2026-05-06)

- **D4: sendClickToPlugin trunceert row naar byte()** — rows >255 wrappen door `[]byte{byte(row)}`. Uitzonderlijk edge case, panels worden zelden >255 rijen. [manager.go:1280]
- **D5: applyAutoCollapse expandeert panels niet bij breder terminal** — Panels blijven collapsed na terminal vergroot. Handmatig togglen nodig. UX verbetering. [manager.go:1028-1051]
- **D6: Panel viewport niet geresized bij WindowSizeMsg** — Scroll bounds raken stale na terminal resize. Pre-existing. [manager.go:394-395]
- **D7: executeBuiltinCommand blokkeert TUI synchroon zonder timeout** — cmd.Run() zonder context/timeout. Pre-existing (ook genoteerd in 12-10 review). [repl.go:803-831]
- **D8: Unclosed markdown delimiters produceren misrendered text** — `**bold` zonder sluit geeft `*` + italic. Pre-existing markdown renderer beperking. [markdown.go:253-265]
- **D9: Nested/indented list items niet herkend** — "  - item" niet gedetecteerd als list. Pre-existing. [markdown.go:124-126]
- **D10: /layout zonder subcommand spawnt onverwacht subprocess** — Handler is nil, valt door naar executeBuiltinCommand("siply", "layout"). UX verbetering. [slashcmds.go + repl.go:369-373]
- **D11: SaveLayoutToConfig read-modify-write niet atomic** — Twee siply instances kunnen config.yaml corrumperen. Pre-existing. [manager.go:822-856]
- **D12: overlayEntry met OverlayZ=0 botst met base dock layer z-index** — Undefined compositing volgorde. Edge case. [manager.go:583]
- **D13: appendMessage concateneert assistant text onbeperkt** — Memory pressure bij zeer lange agent responses. Pre-existing. [repl.go:422-423]
- **D14: refreshOverlayItems LoadAll op elke "/" toets** — Geen debounce, herlaadt alle skills van disk. Performance concern bij veel skills. [repl.go:843-844]

## Deferred from: code review of story 11-13 (2026-04-27)

- **Composite display strings matchen niet met user overrides** — System keybindings gebruiken composite display strings (e.g., `↑ / ↓`, `Ctrl+A / Ctrl+E`) als key identifiers. User overrides met individuele keys (e.g., `ctrl+a`) matchen niet. Pre-existing DefaultKeyBindings() design. [internal/tui/menu/keybindings.go]
- **Loader keybinding accessors retourneren mutable pointer** — GlobalKeybindings()/ProjectKeybindings() retourneren gedeelde *KeybindingConfig zonder defensive copy. Inconsistent met Config() die wél kopieert. Geen huidige callers muteren de pointer. [internal/config/loader.go:185-195]

## Deferred from: code review of tui-split-input-output-chat-bubbles (2026-05-07)

- **Variable-height agent status overflows fixed vpHeight budget** — SetSize() berekent vpHeight als `height - 4 - 2` (bordered), maar AgentStatusPanel.Render() kan meerdere regels produceren (1 header + N agents + 1 tip). Bij 5+ agents overschrijdt de totale content de panel hoogte. Pre-existing issue, niet veroorzaakt door deze wijziging. [internal/tui/panels/repl.go:SetSize, View]

## Deferred from: code review of tui-layout-fixes (2026-05-30)

- **agentStatus.Render(r.width) kan panel border wrapping veroorzaken** — agentView wordt gerenderd met volledige `r.width`, maar panel innerWidth is `width-2` bij bordered mode. Lange agent status tekst kan extra regels produceren door border wrapping. Pre-existing issue. [internal/tui/panels/repl.go:View, r.agentStatus.Render]
- **Narrow terminal overlay width overflow** — SlashOverlay.SetSize clamp naar minimum 20, maar bij vpWidth < 20 is de overlay breder dan de beschikbare ruimte. Truncatie gebruikt `s.width` (20) i.p.v. werkelijke terminal breedte. Pre-existing issue. [internal/tui/panels/slashoverlay.go:SetSize, View]

## Deferred from: code review of tui-layout-fixes-2 (2026-06-01)

- **[OPGELOST 2026-06-01] Onzichtbare DiffView slokt navigatietoetsen op** — De 31-mei status-bar fix verwijderde alle rendering/sizing van `a.diffView` uit `app.go`, maar liet de key-routing intact, waardoor de onzichtbare diff view `tab/esc/e/up/down/k/j` afving en navigatie bevroor. Fix: de `diffView.HandleKey`-onderscheppingsblok verwijderd; die toetsen stromen nu door naar de panelManager (met verklarende NOTE op die plek). [internal/tui/app.go key-routing]
- **ActivityFeed ontvangt data maar wordt nergens getoond** — Zelfde klasse: `activityFeed.HandleFeedEntry/HandleFeedState/HandleFeedback` worden nog aangeroepen maar de feed wordt niet meer gerenderd. Stille feature-verdwijning (geen crash). Bevestig of de feed bedoeld was te migreren naar het panel-systeem; zo niet, verwijder de verweesde handlers/setters/velden. Pre-existing. [internal/tui/app.go:387-427]
