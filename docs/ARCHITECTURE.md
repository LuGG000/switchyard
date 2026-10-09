# Architecture (as built)

`docs/PLAN.md` is the original plan (German) and still holds the rationale,
facts and phases. This file describes what exists in the code today and the
decisions taken while building it. Where the two differ, this file wins.

## Packages

| Package | Purpose |
| --- | --- |
| `cmd/switchyard` | cobra CLI: `init add login list status switch run repair doctor` and the hidden `hook` command |
| `internal/config` | `config.toml` loading, defaults, validation; data and config directory lookup |
| `internal/state` | `state.json` with inter-process file lock (`gofrs/flock`) and atomic writes |
| `internal/profiles` | profile directories, `claude auth login/status` wrappers, subscription validation |
| `internal/claudeenv` | environment for child `claude` processes: removes credential variables, sets `CLAUDE_CONFIG_DIR` |
| `internal/linker` | shares entries of the default config dir with profiles (symlink, Windows junction) |
| `internal/launcher` | starts `claude` for a profile, returns its exit code |
| `internal/selector` | picks the next profile (sequential, most-headroom, round-robin), honors cooldowns |
| `internal/detector` | recognizes a rate limit in headless output (`stream-json` events, error result, plain text) |
| `internal/headless` | `claude -p` failover loop: detect limit, cooldown, next profile, resume |
| `internal/interactive` | interactive failover loop: poll for a switch request, end claude, ask or switch, relaunch with resume |
| `internal/hooks` | statusLine and StopFailure hook handlers, builds the `--settings` JSON |
| `internal/doctor` | read-only health checks |
| `fakeclaude` | test double for the `claude` executable (see `docs/DEVELOPMENT.md`) |

Not yet written: `internal/platform` and the mod (phase 5). There is no `internal/ipc`:
the hooks reach the launcher through the state file (see Decisions).

## Files and directories

- Config: `<os.UserConfigDir>/switchyard/config.toml`, override with `SWITCHYARD_CONFIG_DIR`.
- Data: `$XDG_DATA_HOME/switchyard`, else `%LocalAppData%\switchyard`, else
  `~/.local/share/switchyard`; override with `SWITCHYARD_DATA_DIR`.
  - `profiles/<name>/`: one claude config dir per profile (own login).
  - `state.json` and `state.json.lock`.
- A profile name matches `^[a-z0-9][a-z0-9_-]{0,31}$`.

## Config keys (`config.toml`)

| Key | Default | Meaning |
| --- | --- | --- |
| `mode` | `ask` | `ask` or `auto` failover (headless runs are always automatic) |
| `carry_context` | `true` | continue the current conversation when switching |
| `strategy` | `sequential` | `sequential`, `most-headroom`, `round-robin` |
| `continue_prompt` | empty | sent as the first message of a resumed session |
| `proactive_threshold` | `0` | five-hour percentage that triggers a switch, 0 = off |
| `source_dir` | empty = `~/.claude` | claude config dir whose entries are shared |
| `link` | projects, settings.json, CLAUDE.md, skills, agents, commands, plugins | entries shared with every profile |

## State (`state.json`, schema version 1)

`active` (profile name), `profiles.<name>` with `last_used`, `cooldown_until`,
`five_hour` and `seven_day` (`used_percent`, `resets_at`, `updated_at`), and
`switch_request` (`profile`, `reason`, `session_id`, `requested_at`; written by a
hook, cleared by the launcher). New fields are added without bumping the
version; a file with a newer version is rejected.

## `status --json` (schema 1)

`{"schema":1,"version":"…","active":"…","profiles":[{"name","active","last_used","cooldown_until","five_hour","seven_day"}]}`.
It reads only the state file and never starts `claude`, so the mod can poll it.
`five_hour`/`seven_day` are `null` or `{used_percent, resets_at, updated_at}`.
Consumers must check `schema` first.

## Decisions

- **Switching.** `switch <name>` makes the profile active and starts `claude`
  with it. `--resume` adds `--continue` (the most recent conversation of the
  current directory, found through the shared `projects/`), `--fresh` starts a
  new one, `--no-launch` only changes the active profile. Without a flag
  `carry_context` decides. Carrying context prints the cold-cache hint (the new
  account reprocesses the whole conversation). Failover in phase 4 should resume
  by session ID instead; `session_id` and `transcript_path` are in the
  statusLine JSON.
- **Failover choice.** The user chooses automatic (`auto`) or confirmed (`ask`)
  failover, and whether context is carried over. Manual switching with or
  without context is always possible.
- **Signals.** `run` and `switch` pass `--settings <json>` to claude with a
  `StopFailure` hook (matcher `rate_limit`) and a `statusLine` that call
  `switchyard hook statusline|stop-failure --profile <name>`. The user's
  settings files are never modified. Hooks from `--settings` are added to the
  user's hooks, but a `statusLine` replaces the user's, so the statusline hook
  runs the profile's own `statusLine` command itself and prints its output
  (spike results in `docs/SPIKE.md`).
- **Usage source.** Interactive: the statusLine JSON (`rate_limits.five_hour`,
  `seven_day` with `used_percentage` and `resets_at`), delivered after the first
  response of a session and refreshed per turn. Headless (`-p`): the statusLine
  is not called; use the `rate_limit_event` of `--output-format stream-json`
  (`utilization` as a fraction, `resetsAt`).
- **Cooldown (provisional).** The StopFailure hook sets `cooldown_until` to the
  reset of a window that is used up, else to the nearest known future reset,
  else now plus 30 minutes. Revisit once a real limit is observed (spike #6).
- **Linker.** It never overwrites real content. It replaces only empty
  directories and dangling links; anything else is a conflict that `repair` and
  `doctor` report. Windows uses directory junctions (no admin rights) and, for
  files, a symlink or hard link. Never remove a junction with a recursive
  delete.
- **Environment.** Child processes lose `ANTHROPIC_API_KEY`,
  `ANTHROPIC_AUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN`, Bedrock/Vertex/Foundry
  variables, and get `CLAUDE_CONFIG_DIR` set to the profile.
- **Secrets.** `claude auth status` output is parsed for `loggedIn`,
  `authMethod`, `subscriptionType`, `configDirectory` only. `doctor` reads
  only key names from `settings.json`. `.credentials.json` is never touched.
- **Headless failover.** `run` treats the claude arguments as headless when they
  contain `-p` or `--print` (`switchyard run -- -p "…"`; `-p` before the `--` is
  `--profile`). The run is automatic whatever `mode` says, since there is no
  terminal to ask. An attempt counts as limited when claude exits non-zero and
  `internal/detector` saw a limit signal in stdout or stderr. The profile then
  gets its usage from the `rate_limit_event` and a cooldown (same rule as the
  StopFailure hook), the selector picks the next profile and the run is repeated.
  With `carry_context` the repeat gets `--resume <session_id>` (session ID from the
  `stream-json` output) or `--continue` if the output has none; the caller's own
  resume options are replaced. Without it the original arguments run again.
  Stdin is recorded and replayed so a piped prompt survives the repeat; output of
  the limited attempt has already been passed on. The signals are provisional
  until a real limit is observed (spike #6).
- **Interactive failover.** `run` and `switch` start claude through
  `internal/interactive`. The StopFailure hook (cooldown, `reason: rate_limit`)
  and the statusLine hook (`reason: threshold` once the five-hour usage reaches
  `proactive_threshold`) write a `switch_request` with the session ID into
  `state.json`; the launcher polls it every 250 ms. This replaces the planned
  socket / named pipe: no extra dependency, no platform-specific code, and
  the state file is already the shared, locked channel. A request counts only
  for the profile that was launched and only if it is newer than the launch. A
  threshold request is ignored while no other profile is out of cooldown, so a
  working session is not ended for nothing.
  On a request the launcher ends claude (SIGTERM, kill after 5 s; Windows kills
  at once), puts the profile into cooldown and asks the selector for the next
  one. `auto` switches and prints a notice. `ask` shows the cold-cache hint and
  asks: `y` switch and continue the conversation, `f` switch with a new one,
  `w` wait for the reset, `q` quit; Enter follows `carry_context`. If every
  profile is locked, `auto` fails with the earliest reset time and `ask` offers
  wait or quit. A carried conversation relaunches with `--resume <session_id>`
  (or `--continue` without an ID) plus `continue_prompt` as the first message.
  The termination mechanism is provisional until spike #7.
