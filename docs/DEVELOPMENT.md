# Development

## Build, install, check

```
go build -o ~/.local/bin/switchyard ./cmd/switchyard   # rebuild after every pull
gofmt -l .
go vet ./...
GOOS=windows go vet ./...
go test -race ./...
golangci-lint run        # same linter as CI; errcheck is the usual catch
```

CI (`.github/workflows/ci.yml`) runs tests on Linux (with `-race`) and Windows,
golangci-lint, govulncheck and a secret scan. All five jobs are required checks
for `main`.

## Git workflow

- `main` is protected: changes only through a pull request with green checks,
  linear history, no force push. Squash-merge.
- One small branch and PR per step (`feat/…`, `fix/…`, `docs/…`).
- Commit identity `LuGG000 <81622672+LuGG000@users.noreply.github.com>`
  (`git config --local`), Conventional Commits, no AI attribution.
- Close or comment on the matching GitHub issue and update `docs/STATUS.md`
  when a step is done.

## Tests without an account

- Unit tests use temp dirs and `SWITCHYARD_DATA_DIR` / `SWITCHYARD_CONFIG_DIR`.
- `fakeclaude/` is a small Go program standing in for `claude`. Behavior is set
  by environment variables (`FAKE_EXIT`, `FAKE_LOGGED_IN`, `FAKE_AUTH`, `FAKE_LIMIT_DIR` for a simulated limit); a normal
  run prints a JSON report of the arguments, config dir and which credential
  variables reached it. `internal/launcher` tests build it with `go build`.
  Some packages instead re-execute the test binary as a minimal fake claude
  (`TestMain`).
- To try the CLI against it: put the built `fakeclaude` as `claude` in a temp
  `PATH` directory and set `SWITCHYARD_DATA_DIR` to a temp dir.

## Tests with real accounts

Follow `AGENTS.md`: few short prompts, `--model haiku`, an empty temp directory.
Profiles are created with `switchyard add <name>` (browser login; the same Pro
account can be used for several profiles). Typical check:

```
mkdir -p /tmp/sytest && cd /tmp/sytest
switchyard run -p main -- --model haiku -p "Remember the word kiwi"
switchyard switch other --resume -- --model haiku -p "Which word did I ask you to remember?"   # answers kiwi
switchyard switch main --fresh -- --model haiku -p "Which word did I ask you to remember?"      # does not know it
```

Test sessions end up in `~/.claude/projects/-tmp-sytest` (shared `projects/`).
Clean up with `rm -r /tmp/sytest ~/.claude/projects/-tmp-sytest`, naming the
paths explicitly.

## Spike method for the statusLine (done, see `docs/SPIKE.md`)

A profile with its own `settings.json` (remove the symlink in that profile only,
never edit the user's file) containing `{"statusLine":{"type":"command","command":"echo USER-LINE"}}`
was run with `--settings` pointing to a capture script. The script read stdin
with `jq`, appended only the key structure (`type` of every value) and the
`rate_limits` numbers plus a timestamp to a log, and printed a marker line.
Never log text values such as paths, session content or account data.

## Open spikes

`#6` headless limit signal, `#7` terminating interactive `claude`, `#10` WSL.
They need a second independent Pro account (expected around 2026-10-24) or a
Windows/WSL machine; the issue bodies hold the acceptance criteria. Record the
results in `docs/SPIKE.md`.
