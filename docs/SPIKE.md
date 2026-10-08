# Phase 0 spike results

Tested with `claude` 2.1.295 on Windows 11, using throw-away `CLAUDE_CONFIG_DIR`
directories that are not logged in. No credentials were read and no prompt was
sent to a real account. Never record emails, org IDs or org names here.

## Verified

### (e) `claude auth status` per config dir and env scrubbing

- `CLAUDE_CONFIG_DIR=<dir> claude auth status` prints JSON and exits 1 when not
  logged in (`loggedIn: false`, `authMethod: "none"`). For a subscription login
  it reports `authMethod: "claude.ai"` and `subscriptionType` (e.g. `pro`).
- The output also contains `email`, `orgId` and `orgName`. switchyard must parse
  only `loggedIn`, `authMethod`, `subscriptionType` and `configDirectory`, and
  must never log the raw output.
- Env credentials override the config dir login, confirmed for an empty dir:
  - `ANTHROPIC_API_KEY` -> `authMethod: "api_key"`
  - `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN` -> `authMethod: "oauth_token"`
- With these variables removed from the environment, the result is
  `authMethod: "none"` again. Env scrubbing in the launcher works, and `doctor`
  can rely on `authMethod == "claude.ai"` as the subscription check.

### (a) `--settings` merge semantics for `hooks`

- Hooks from `--settings` are added to the hooks from the config dir's
  `settings.json`; both fire (tested with `SessionStart`). User hooks are not
  replaced, so no merge file is needed for hooks.
- `SessionStart` hooks fire even when not logged in, which makes hook wiring
  testable without an account.

## Open (need a logged-in test profile or a TUI)

- (a) `statusLine` is a single command, so `--settings` most likely replaces the
  user's. Needs a TUI run to confirm; the plan is to chain the user's command
  inside `switchyard hook statusline`.
- (b) Headless limit signal (JSON and exit code at a subscription limit).
- (c) Clean external termination of an interactive `claude` (signal on
  Unix, Windows equivalent) and session consistency afterwards.
- (d) `--resume` between two profiles via shared `projects/` and via path.
- (f) `rate_limits` in the statusline JSON for Pro accounts and update frequency.
- (g) WSL versus native Windows config dirs.

Items (b) to (g) require logging in a dedicated test profile; see the GitHub
issues labelled `phase-0`.
