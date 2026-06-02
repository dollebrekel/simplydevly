---
baseline_commit: f487239206a1a71b4afdd4608f28b9bbcf4b5e48
---

# Story 11.14: Tabs in het Center-Venster (Multi-Session Center Tabs)

Status: done

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a siply TUI gebruiker,
I want meerdere gespreks-tabs in het center-venster (elk met een eigen sessie/agent), met hotkeys om te wisselen, een "+" voor een nieuwe tab, en een schakelaar om de tabbalk te tonen/verbergen,
so that ik parallel aan meerdere taken kan werken zonder de zij-panel-plugins opnieuw te laden of mijn context te verliezen.

## Context

- Feature B uit de Simply Devly Groeiplan-sessie (2026-06-02), ontworpen door Winston (System Architect).
- Bouwt voort op Epic 11 (Extension System & siply-ui), met name de panel-rendering overhaul (11-8 t/m 11-12) en de gelaagde keybinding-config (11-13).
- **E1 is beantwoord (JA):** een tweede center-sessie kan draaien zonder plugins te herladen. Plugins worden app-breed geladen en registreren uitsluitend in left/right/bottom slots (`internal/extensions/manager.go` — `RegisterPanel()` accepteert alleen `PanelLeft/PanelRight/PanelBottom/PanelOverlay`). Ze zijn agent-agnostisch. Het center is louter een `string` die `App.buildCenterContent()` produceert en aan `PanelManager.View()` doorgeeft (`internal/tui/app.go:682-683`, `:768-773`).
- **Ontwerpbeslissing (gebruiker, 2026-06-02):** de "+"-keuze bepaalt de **toolset** van het nieuwe gesprek (bv. volledig vs. alleen-lezen/research). Zij-panels blijven gedeeld over alle tabs. Tabs zijn ALLEEN voor het center.

## Acceptance Criteria

1. **Given** de TUI draait met het center-venster actief
   **When** de gebruiker de "nieuwe tab"-hotkey indrukt (voorgesteld: `ctrl+t`)
   **Then** wordt een nieuwe center-tab aangemaakt met een eigen, leeg gesprek (eigen agent + eigen REPL-panel)
   **And** de zij-panel-plugins worden NIET opnieuw geladen of geherstart
   **And** de nieuwe tab wordt de actieve tab.

2. **Given** er zijn meerdere center-tabs
   **When** de gebruiker de "wissel-tab"-hotkeys indrukt (voorgesteld: `ctrl+pgup` / `ctrl+pgdn`, en `alt+1`…`alt+9` om direct naar tab N te springen)
   **Then** wisselt ALLEEN het center-venster van actieve tab
   **And** de zij-panels (left/right/bottom) blijven ongewijzigd zichtbaar
   **And** de panel-focus-cyclus (`tab`/`shift+tab`) en de bestaande slot-tab-hotkeys (`ctrl+]`/`ctrl+[`) blijven ongewijzigd werken.

3. **Given** een agent in tab A produceert streaming-output terwijl tab B actief is
   **When** de output via de EventBus binnenkomt
   **Then** wordt de output toegevoegd aan het gesprek van tab A (niet tab B)
   **And** bij terugschakelen naar tab A is de volledige, correcte output zichtbaar
   **And** er lekt geen output van tab A naar de viewport van tab B.

4. **Given** meerdere center-tabs bestaan
   **When** de tabbalk zichtbaar is
   **Then** toont een tabbalk bovenin het center alle tabs met een actieve-indicator (highlight/onderstreping)
   **And** een "+"-affordance is zichtbaar/klikbaar om een nieuwe tab te openen
   **And** de tabbalk respecteert de huidige border-/breedte-instellingen van het center (geen layout-breuk).

5. **Given** de tabbalk wordt getoond
   **When** de gebruiker de "toon/verberg tabbalk"-hotkey indrukt (voorgesteld: `alt+t`)
   **Then** verdwijnt/verschijnt de tabbalk
   **And** de center-content-hoogte past zich correct aan (geen afgekapte of overlappende regels)
   **And** bij precies één tab mag de balk standaard verborgen zijn (configureerbaar).

6. **Given** de gebruiker opent een nieuwe tab via "+"
   **When** de tab wordt aangemaakt
   **Then** krijgt de gebruiker de keuze welke **toolset** dat gesprek mag gebruiken (minimaal: "Volledig" en "Alleen-lezen/Research")
   **And** de gekozen toolset bepaalt de tool-registry van de agent van die tab
   **And** de zij-panels blijven gedeeld (de keuze raakt geen plugins/panels).

7. **Given** er zijn meerdere center-tabs
   **When** de gebruiker een tab sluit (voorgesteld: `ctrl+w`)
   **Then** wordt de agent van die tab netjes afgesloten via `AgentCloser.Close()` (provider-resources vrijgegeven)
   **And** de focus gaat naar een naburige tab
   **And** de laatste tab kan niet gesloten worden (of sluiten = TUI-afsluiten, expliciet te kiezen — zie open vraag).

8. **Given** een model-switch via `/model` (Story 12.8) wordt uitgevoerd
   **When** de switch slaagt
   **Then** raakt deze ALLEEN de agent van de actieve tab (de andere tabs behouden hun eigen agent/model)
   **And** de statusbalk reflecteert het model van de actieve tab.

9. **Given** een gebruiker met één tab (geen multi-tab gebruik)
   **When** de TUI normaal gebruikt wordt
   **Then** is het gedrag identiek aan vóór deze story (geen regressie in rendering, focus, streaming, of model-switch).

## Tasks / Subtasks

- [x] **Task 1 — Center-tab state-model** (AC: 1, 2, 9)
  - [x] Introduceer een `centerTab`-struct die één sessie omvat: een `replPanel SubPanel`, een `agent AgentRunner`, een `agentBusy bool`, en een toolset-aanduiding.
  - [x] Vervang in `App` (`internal/tui/app.go:19-42`) de enkelvoudige velden `replPanel` (`:24`) en `agent` (`:36`) door een `tabs []*centerTab` + `activeTab int`. Behoud een helper `activeREPL()`/`activeAgent()` zodat bestaande call-sites minimaal wijzigen.
  - [x] Pas `SetREPLPanel` (`:63`) en `SetAgent` (`:124`) aan: de eerste wiring vult tab 0 (backwards compatible — single-tab pad blijft identiek).
  - [x] Pas `Init()` (`:130-135`) aan om de actieve tab's REPL te initialiseren.

- [x] **Task 2 — Per-tab message-routing (KERN)** (AC: 3, 9)
  - [x] Kies en documenteer de routerings-aanpak (zie Dev Notes → "Event-routing"): per-tab EventBus (voorkeur, isolatie) óf een tab/sessie-ID die door de events en `AgentOutputMsg` wordt geregen.
  - [x] Bij gekozen aanpak: voeg waar nodig een `TabID`/`SessionID` toe aan `AgentOutputMsg` (`internal/tui/messages.go:36-38`), `AgentDoneMsg` (`:46`), `AgentErrorMsg` (`:347-349`), en aan de EventBus-bridge (`cmd/siply/tui.go:770-774`).
  - [x] Werk de handlers in `App.Update()` bij: `SubmitMsg` (`app.go:179-204`), `AgentOutputMsg, AgentDoneMsg` (`:226-234`), `AgentErrorMsg` (`:217-224`), `CancelMsg` (`:206-215`) — zodat ze de JUISTE tab adresseren i.p.v. de enkele `replPanel`/`agent`.
  - [x] Zorg dat een nieuwe agent per tab zijn eigen bus/bridge krijgt zónder Tier2/Tier3-plugins opnieuw te laden.

- [x] **Task 3 — Nieuwe agent per tab (zonder plugin-reload)** (AC: 1, 6)
  - [x] Maak de agent-bootstrap herbruikbaar: hergebruik het pad uit `cmd/siply/tui.go:549-581` (`appstartup.Start()`) om per nieuwe tab een agent te maken, met gedeelde plugin-laag.
  - [x] Implementeer de toolset-keuze bij "+": bouw de `tools.Registry` van de nieuwe agent op basis van de keuze (volledig vs. alleen-lezen/research).
  - [x] Verifieer dat de gedeelde `AgentHooks` (tree-sitter, distillation, session-intelligence) NIET opnieuw geïnstantieerd hoeven te worden; beslis: gedeelde hooks (eenvoudig) of per-agent referenties.

- [x] **Task 4 — Tabbalk render + toon/verberg** (AC: 4, 5, 9)
  - [x] Render een center-tabbalk in `buildCenterContent()` (`app.go:768-773`): tabbalk bovenaan + de actieve tab's `replPanel.View()` eronder. Hergebruik het render-patroon van de bestaande slot-tabbalk (`internal/tui/panels/manager.go:~1110` `renderSlot`).
  - [x] Voeg een `centerTabBarVisible bool` toe (default: verborgen bij 1 tab, zichtbaar bij >1 — configureerbaar).
  - [x] Verreken de tabbalk-hoogte in de center-content-hoogte (voorkom afkapping); raak `CalculateLayoutWithPanels` (`app.go:148`) niet onnodig aan — los het binnen het center op.

- [x] **Task 5 — Hotkeys + keybinding-integratie** (AC: 1, 2, 5, 7)
  - [x] Voeg key-routing toe in `App.Update()` `tea.KeyPressMsg` (`app.go:480-593`), BOVEN de panel-manager-routing (`:544-555`), voor: nieuw (`ctrl+t`), sluiten (`ctrl+w`), wisselen (`ctrl+pgup`/`ctrl+pgdn`, `alt+1..9`), tabbalk toggle (`alt+t`).
  - [x] **Vermijd bezette keys:** `ctrl+c` (quit, `:484`), `ctrl+@`/`ctrl+space` (menu, `:497`), `ctrl+b` (borders, `:511`), `tab`/`shift+tab`/`alt+left`/`alt+right`/`ctrl+]`/`ctrl+[` (panel-focus & slot-tabs, `:546`).
  - [x] Registreer de nieuwe keybindings via de gelaagde keybinding-config (Story 11-13) i.p.v. hardcoded waar mogelijk; documenteer defaults.

- [x] **Task 6 — Tab sluiten + lifecycle** (AC: 7, 8)
  - [x] Implementeer sluiten: roep `AgentCloser.Close()` aan (zie patroon `app.go:304-309`), verwijder de tab, herbereken `activeTab`.
  - [x] Pas de model-switch (`ModelSwitchResultMsg`, `app.go:293-325`) aan zodat hij de agent van de ACTIEVE tab vervangt (niet globaal).
  - [x] Borg "laatste tab"-gedrag (zie open vraag).

- [x] **Task 7 — Tests** (AC: alle)
  - [x] Unit tests in `internal/tui/app_test.go`: tab aanmaken/wisselen/sluiten, routing van `AgentOutputMsg` naar juiste tab, single-tab regressie-pad.
  - [x] Test tabbalk render (zichtbaar/verborgen) en hoogte-verrekening.
  - [x] Test dat model-switch alleen de actieve tab raakt.
  - [x] `go test -race -parallel 4 ./...` (via `make test`) moet groen zijn.

### Review Findings

- [x] [Review][Patch] Model switch in a secondary tab installs an agent wired to the shared tab-0 EventBus, so streamed output and tool events route back to tab 0 instead of the active tab after `/model` succeeds. [cmd/siply/tui_model.go:116]
- [x] [Review][Patch] Center-tab hotkeys are handled as hardcoded built-ins and are absent from the layered keybinding resolver / Learn defaults, so user keybinding overrides cannot affect them and Learn still documents conflicting meanings for `Ctrl+T` and `Ctrl+W`. [internal/tui/app.go:681]
- [x] [Review][Patch] Required center-tab implementation files are still untracked (`cmd/siply/tui_tabs.go`, `internal/tui/center_tabs.go`, `internal/tui/center_tabs_test.go`), while staged files reference their symbols; a staged-only commit would not build. [git status]

## Dev Notes

### Huidige staat van de te wijzigen bestanden (GELEZEN — niet aannemen, dit is geverifieerd)

**`internal/tui/app.go`** — de root Bubble Tea `App`-model.
- `App`-struct (`:19-42`) heeft ENKELVOUDIGE `replPanel SubPanel` (`:24`) en `agent AgentRunner` (`:36`), plus `agentBusy` (`:37`) en `modelSwitching` (`:38`). Dit zijn de velden die multi-tab moeten worden.
- `buildCenterContent()` (`:768-773`) retourneert simpelweg `a.replPanel.View()`. Hier komt de tabbalk + actieve-tab-render.
- `Update()` routeert `AgentOutputMsg, AgentDoneMsg` (`:226-234`) naar de ENE `replPanel`. `SubmitMsg` (`:179-204`) start de ENE `agent` en zet `agentBusy`. `CancelMsg` (`:206-215`) stopt de ENE agent. Deze moeten tab-bewust worden.
- Model-switch handlers `ModelOpenMsg`/`ModelListResultMsg`/`ModelSelectedMsg`/`ModelSwitchResultMsg` (`:257-325`) vervangen nu globaal `a.agent` (`:304-309`); moeten de actieve tab raken.
- Key-routing (`tea.KeyPressMsg`, `:480-593`): bezette keys staan hierboven in Task 5. Panel-navigatie gaat naar `panelManager` op `:544-555`. Nieuwe center-tab-keys moeten hiervóór worden afgevangen.
- `WindowSizeMsg` (`:140-177`) zet center-breedte op de replPanel (`:162-164`); bij multi-tab moet elke tab's replPanel de juiste size krijgen.

**`internal/tui/messages.go`** — interfaces & messages (in `tui`-package om import-cycles te vermijden).
- `AgentRunner` (`:16-19`) + `AgentCloser` (`:22-25`): `Close(ctx)` bestaat al → gebruik voor tab-sluiten.
- `AgentOutputMsg{Text}` (`:36-38`), `AgentDoneMsg{}` (`:46`), `AgentErrorMsg{Err}` (`:347-349`): GEEN tab/sessie-ID → uitbreiden indien gekozen voor ID-routing.
- `SubPanel` (`:50-56`) is de interface die elke tab's REPL implementeert (`Init/Update/View/SetSize/SetBordered`).
- `PanelManager` (`:287-294`): `View(width, height, centerContent string)` — center blijft een string-parameter; tabbalk hoeft NIET in PanelManager (kan in `buildCenterContent`).
- **Let op:** `AgentStatusUpdateMsg` (`:362-369`) heeft al een `AgentID string` — een bestaand precedent voor agent-identificatie dat hergebruikt/uitgebreid kan worden voor tab-routing.

**`cmd/siply/tui.go`** — bootstrap & EventBus→BubbleTea bridge.
- EventBus-bridge `EventStreamText` → `prog.Send(tui.AgentOutputMsg{Text: te.Text()})` (`:770-774`). Dit is de plek waar tab/sessie-context verloren gaat: de bridge weet niet welke tab de event veroorzaakte. KERN van het routeringsprobleem.
- Agent-bootstrap via `appstartup.Start()` (`:549-581`) → herbruikbaar per tab.

**`internal/tui/panels/manager.go`** — slot-tabs als referentie-implementatie.
- `slot`-struct heeft `activeTab int` (`:~38`); `renderSlot()` rendert een tabbalk bij >1 panel (`:~1110`); `switchTab()` (`:~968-985`); focus-constanten (`:~29-35`); `ctrl+]`/`ctrl+[` routing (`:~413-416`). Hergebruik dit render-/switch-patroon voor het center.

### Event-routing — de centrale ontwerpbeslissing

Twee haalbare aanpakken (kies en documenteer in de implementatie):

- **(A) Per-tab EventBus (voorkeur — echte isolatie).** Elke tab krijgt een eigen agent met eigen EventBus; de bridge in `tui.go` wordt per tab opgezet en stuurt een tab-getagde `AgentOutputMsg`. Plugins blijven op de gedeelde app-bus/laag. Voordeel: geen kruisbesmetting, geen ID-filtering in de hot path. Nadeel: meer wiring bij tab-creatie.
- **(B) Gedeelde bus + sessie-ID.** Eén bus; events dragen een sessie/agent-ID; de bridge zet die op `AgentOutputMsg.TabID`; `App.Update()` routeert op ID. Voordeel: minder wiring. Nadeel: ID moet overal consistent meegegeven worden; risico op lek bij vergeten plekken. Hergebruik het bestaande `AgentID`-precedent (`messages.go:362-369`).

### Wat NIET mag breken (regressie-bewaking)

- Single-tab gebruik moet byte-voor-byte identiek renderen en reageren (AC 9). Wire tab 0 zo dat het bestaande pad ongewijzigd blijft als er nooit een 2e tab wordt geopend.
- De zij-panel-plugins, hun `ContentReceiver`/`panelViewport` en de slot-tabs (`ctrl+]`/`ctrl+[`) zijn orthogonaal — NIET aanraken.
- De panel-focus-cyclus (`tab`/`shift+tab`/`alt+left`/`alt+right`) blijft via `panelManager` lopen (`app.go:546-555`).

### Project Structure Notes

- Module: `siply.dev/siply` (Go 1.26.1). Test: `go test -race -parallel 4 ./...` (`make test`). Lint: golangci-lint (CI).
- TUI-conventies: panels muteren via pointer-receiver en retourneren alleen `tea.Cmd` uit `Update` (zie `SubPanel`-doc, `messages.go:48-49`). Interfaces leven in het `tui`-package om import-cycles te vermijden — nieuwe types/velden voor tabs horen ook daar of in `app.go`.
- Laad `docs/go-best-practices.md` secties `shared`, `frontend-tui` en `testing` vóór implementatie (verplicht volgens `project-context.md`).
- Text-lengte van user-visible strings: `utf8.RuneCountInString()`, nooit `len()`.

### References

- [Source: internal/tui/app.go:19-42] App-struct met enkelvoudige replPanel/agent
- [Source: internal/tui/app.go:226-234] Globale AgentOutputMsg/AgentDoneMsg routing
- [Source: internal/tui/app.go:293-325] Model-switch vervangt globaal a.agent
- [Source: internal/tui/app.go:480-593] Key-routing + bezette hotkeys
- [Source: internal/tui/app.go:768-773] buildCenterContent (tabbalk-injectiepunt)
- [Source: internal/tui/messages.go:36-46] AgentOutputMsg/AgentDoneMsg zonder tab-ID
- [Source: internal/tui/messages.go:22-25] AgentCloser.Close voor tab-sluiten
- [Source: internal/tui/messages.go:362-369] Bestaand AgentID-precedent
- [Source: cmd/siply/tui.go:770-774] EventBus→AgentOutputMsg bridge (routeringsprobleem)
- [Source: cmd/siply/tui.go:549-581] appstartup.Start agent-bootstrap (herbruikbaar per tab)
- [Source: internal/tui/panels/manager.go:~1110] renderSlot tabbalk-patroon (hergebruik)
- [Source: internal/extensions/manager.go] RegisterPanel beperkt tot left/right/bottom/overlay (E1-bewijs)
- [Source: _bmad-output/planning-artifacts/ADR-001-no-tmux-pure-go-terminal.md] Pure-Go multiplexing fundament

### Previous Story Intelligence (Epic 11)

- 11-13 (gelaagde keybinding-config) is `done`: gebruik dat systeem voor de nieuwe center-tab-keys i.p.v. hardcoden.
- 11-8 t/m 11-12 (panel-rendering overhaul, `done`): gebruik `lipgloss.JoinHorizontal/JoinVertical` en de Compositor-patronen; GEEN `strings.Replace` voor layout.
- Recente layout-fixes (commits `b627c88`, `fa628e4`, `dd63ac7`, `f487239` op `feature/simply-devly-tui-layout`) raakten `panels/manager.go`, `panels/repl.go`, `statusline/statusline.go` — wees voorzichtig met de borderless-center- en divider-logica die daar net gestabiliseerd is.

## Out of Scope

- Tabs in de zij-panels (left/right/bottom) — die hebben al tabs (`ctrl+]`/`ctrl+[`).
- Per-tab eigen set zij-panels/plugins (gebruiker koos: zij-panels blijven gedeeld).
- Volledig losse siply-instanties per tab (E1: we delen de plugin-laag bewust).
- Slepen/herordenen van tabs met de muis (kan latere story zijn).
- Persistentie van open tabs over herstarts heen (latere story).
- De Feature D modelbeheer-uitbreidingen (aparte story in Epic 12).

## Open Questions (voor SM/PO)

1. **Laatste tab sluiten:** mag `ctrl+w` op de enige tab de TUI afsluiten, of moet sluiten dan geweigerd worden? (AC 7)
2. **Toolset-presets:** zijn "Volledig" en "Alleen-lezen/Research" voldoende voor v1, of willen we ook een per-tab tool-multiselect? (AC 6)
3. **Hotkey-defaults:** akkoord met `ctrl+t` (nieuw), `ctrl+w` (sluiten), `ctrl+pgup`/`ctrl+pgdn` + `alt+1..9` (wisselen), `alt+t` (tabbalk toggle)? Allen vrij van bestaande bindings.
4. **Event-routing:** voorkeur voor aanpak (A) per-tab EventBus of (B) gedeelde bus + sessie-ID?

## Dev Agent Record

### Agent Model Used

claude-opus-4-8 (Claude Code dev-story workflow)

### Debug Log References

- Build: `go build ./...` — OK
- Tests: `make test` (`go test -race -parallel 4 ./...`) — all packages green; `go test -race ./cmd/siply/` — OK
- Lint: `golangci-lint run ./internal/tui/... ./cmd/siply/... ./internal/app/... ./internal/tools/...` — exit 0
- Note: `TestNewRunCmd_RoutingFlag` flakes only when a local provider (Ollama) happens to answer during a multi-package run; verified identical behavior on the baseline commit — pre-existing environment sensitivity, not caused by this story.

### Decisions (Open Questions resolved with user, 2026-06-02)

1. **Event-routing:** Approach (A) — per-tab EventBus. Each new tab gets its own `events.Bus` and a bridge (`bridgeTabBus`) that tags `AgentOutputMsg`/`FeedEntryMsg` with the tab id. Tab 0 keeps the shared bus + existing bridge unchanged, so the single-tab path is byte-for-byte identical (AC 9).
2. **Last tab close:** refused — `ctrl+w` on the only tab is a no-op with an info feedback message.
3. **Toolset presets:** "Full" and "Read-only/Research" (the latter = `file_read`, `search`, `web`). Implemented via new `appstartup.Options.AllowedTools` + `tools.Registry.Retain`.
4. **Hotkeys:** `ctrl+t`/`ctrl+w` from the original proposal collided with existing bindings (tree-local plugin uses `ctrl+t`; the input uses `ctrl+w` for delete-word). User chose the conflict-free **alt family** (2026-06-02): `alt+n` (new, opens toolset chooser), `alt+w` (close), `ctrl+pgup`/`ctrl+pgdn` (switch, wrap), `alt+1..9` (jump), `alt+t` (toggle bar). All added to the Learn keybinding view under a new "Center Tabs" category.

### Completion Notes List

- `App` now holds `tabs []*centerTab` + `activeTab`; the single `replPanel`/`agent`/`agentBusy` fields were replaced by per-tab state with `active*()`/`tabByID()` helpers. `SetREPLPanel`/`SetAgent` lazily populate tab 0, so existing wiring is unchanged.
- Tab-tagged routing added to `AgentOutputMsg`, `AgentDoneMsg`, `AgentErrorMsg`, `FeedEntryMsg` (`TabID`, zero value = tab 0). `App.Update` routes each to the originating tab; the shared activity feed still logs every tab's tool activity.
- New tabs are built by an injected `CenterTabFactory` (`cmd/siply` `centerTabFactory`) that shares the plugin layer (hooks) + slash dispatcher — no plugin reload (E1). Per-tab agent built via `appstartup.Start` with its own bus.
- Tab bar (`renderCenterTabBar`) renders above the active REPL with an active-indicator and clickable `+`; its height is subtracted from each tab's content height (`resizeTabs`). Hidden by default at one tab; `alt+t` toggles.
- Toolset chooser (`renderTabChooser`) is a centered modal shown after `ctrl+t` / `+`.
- Model switch (`ModelSwitchResultMsg`) and the busy-guard now target the active tab only (AC 8).
- Tab close releases the agent via `AgentCloser.Close` and stops the per-tab bus (`tabAgent.Close`); the last tab cannot be closed.
- Best-effort mouse hit-testing on the tab bar (`handleTabBarClick`) runs only after the panel manager declines the click, so the recently-stabilised divider/focus logic is untouched.

### File List

- internal/tui/messages.go (modified — TabID fields, ToolsetChoice, NewCenterTabMsg, CenterTabFactory)
- internal/tui/app.go (modified — centerTab model, tab helpers, tab-aware Update/View, key & mouse routing)
- internal/tui/center_tabs.go (new — tab lifecycle, tab bar & chooser rendering, resize, mouse hit-test)
- internal/tui/center_tabs_test.go (new — center-tab unit tests)
- internal/tui/app_test.go (modified — SetAgent assertion uses activeAgent())
- internal/tui/menu/keybindings.go (modified — new "Center Tabs" Learn category)
- internal/tui/menu/keybindings_test.go (modified — 6 categories + names)
- internal/tui/menu/resolver_test.go (modified — plugin category index after Center Tabs)
- internal/app/startup.go (modified — Options.AllowedTools + registry Retain)
- internal/tools/registry.go (modified — Registry.Retain)
- cmd/siply/tui.go (modified — wire centerTabFactory, SetProgram in setup callback)
- cmd/siply/tui_tabs.go (new — centerTabFactory, tabAgent, bridgeTabBus)

### Change Log

- 2026-06-02: Implemented Story 11.14 — multi-session center tabs (per-tab agent/bus/REPL, shared side-panel plugins). Per-tab EventBus routing, tab bar + toolset chooser, per-tab model switch and lifecycle. All ACs satisfied; full suite + race + lint green.
- 2026-06-02: Resolved hotkey conflicts — switched new/close tab from `ctrl+t`/`ctrl+w` to `alt+n`/`alt+w` (kept tree-panel `ctrl+t` and input delete-word `ctrl+w` intact). Documented all center-tab hotkeys in the Learn keybinding view under a new "Center Tabs" category.
