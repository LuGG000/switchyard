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

## Update: results with two logged-in profiles (same Pro account)

Profile A is the default `~/.claude`, profile B a separate config dir logged in
with the same account. Tests ran from an empty temp directory with `--model haiku`.

### (d) `--resume` between profiles: verified

- Without sharing, `claude --resume <id>` in profile B fails with
  `No conversation found with session ID`.
- `--resume <absolute path to the .jsonl>` in profile B works without any
  linking; the conversation context is restored. Plan B (path based) is viable.
- With B's `projects/` replaced by a directory junction to A's `projects/`,
  `--resume <id>` in B works, and the turn is appended to the same `.jsonl`.
- Creating the junction needs no admin rights on Windows
  (`New-Item -ItemType Junction`).
- Decision: the linker shares `projects/` via symlink (Unix) / junction
  (Windows). The path based resume stays as fallback.

### (f) Rate limit data: partly verified, different source than planned

- `claude -p --output-format stream-json --verbose` emits a `rate_limit_event`
  with `rate_limit_info.unifiedWindows.five_hour` and `.seven_day`, each with
  `utilization` (fraction 0..1) and `resetsAt` (Unix seconds), plus `status`
  and `rateLimitType`. This gives usage tracking for headless runs without the
  statusLine.
- The statusLine command is not invoked in `-p` mode, so its JSON (and update
  frequency) can only be captured in the interactive TUI. Still open.

### (b) Headless limit signal

- The same `rate_limit_event` carries `status` (`allowed` observed). The value
  and result JSON at an actual limit are still unobserved; needs a real limit hit
  (planned once a second account exists).

### (g) WSL

- WSL is not installed on this machine, so only native Windows was tested.

### Still open (need a TUI or a real limit)

- (a) statusLine precedence of `--settings`, (c) clean termination of an
  interactive session, (f) statusLine JSON and frequency: need an interactive run.
- (b) value of `status` at a real limit, (g) WSL behavior.

## Update: statusLine in the interactive TUI (Linux, claude 2.1.295)

Tested with a Pro profile that has its own user-level `settings.json` containing
`statusLine` (`echo USER-LINE`), `--model haiku`, from an empty temp directory.
A capture script logged only the JSON structure and the `rate_limits` numbers.

### (a) statusLine precedence: verified

- Without `--settings` the TUI shows the user's statusLine.
- With `--settings '{"statusLine":{...}}'` the injected command replaces the
  user's statusLine; the user's command is not run.
- Decision: `switchyard hook statusline` runs the user's own statusLine command
  with the same stdin and prints its output, so injecting ours does not remove
  the user's status line.

### (f) statusLine JSON and frequency: verified

- The command receives a JSON object on stdin. Top-level keys: `context_window`,
  `cost`, `cwd`, `effort`, `exceeds_200k_tokens`, `fast_mode`, `model`,
  `output_style`, `scratchpad_dir`, `session_id`, `thinking`, `transcript_path`,
  `version`, `workspace`.
- `rate_limits` is `null` or absent right after start and appears once the first
  response has arrived. Shape:
  `rate_limits.five_hour` and `rate_limits.seven_day`, each with
  `used_percentage` (whole number) and `resets_at` (Unix seconds).
- `session_id` and `transcript_path` are available, which phase 4 can use to
  resume by ID.
- The command is called at startup and after assistant responses (four calls in
  about two minutes with two prompts); no calls were observed while idle.
  The values are therefore as fresh as the last turn.
