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
- Conventional Commits; close or comment on the matching GitHub issue when a step is done.

## Tests without an account

- Unit tests use temp dirs and `SWITCHYARD_DATA_DIR` / `SWITCHYARD_CONFIG_DIR`.
- `fakeclaude/` is a small Go program standing in for `claude`. Behavior is set
  by environment variables (`FAKE_EXIT`, `FAKE_LOGGED_IN`, `FAKE_AUTH`, `FAKE_LIMIT_DIR` and `FAKE_THRESHOLD_DIR` for a simulated limit or threshold); a normal
  run prints a JSON report of the arguments, config dir and which credential
  variables reached it. `internal/launcher` tests build it with `go build`.
  Some packages instead re-execute the test binary as a minimal fake claude
  (`TestMain`).
- To try the CLI against it: put the built `fakeclaude` as `claude` in a temp
  `PATH` directory and set `SWITCHYARD_DATA_DIR` to a temp dir.

## Tests with real accounts

Keep it cheap: few short prompts, `--model haiku`, an empty temp directory.
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

Still unverified: the headless limit signal and the real StopFailure hook input
(both need a real usage limit on a second Pro account), terminating an
interactive `claude` on Unix, and WSL versus native Windows config dirs. The open
issues hold the acceptance criteria; record the results in `docs/SPIKE.md`.

## The mod

```
claude plugin validate mod            # manifest and hooks module
claude plugin validate .              # marketplace file
claude plugin test mod                # tests/*.test.ts with a mocked switchyard
tsc -p mod                            # needs the generated types, see mod/README.md
```

`claude --plugin-dir mod` loads the mod from the checkout for one session. To try
`switchyard mod install` without touching your Claude settings, point
`CLAUDE_CONFIG_DIR` at an empty temp directory and use
`--source <path to this repository>`.

## Releasing

1. `main` is green and the changes are merged.
2. Check the release config locally: `go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean` (the output in `dist/` is ignored by git).
3. Tag and push: `git tag -a v0.1.0 -m "v0.1.0" && git push origin v0.1.0`. The `release` workflow builds the archives and creates the GitHub release with notes grouped by Conventional Commit type (`docs:`, `ci:`, `chore:`, `test:` are left out).
4. `.deb`/`.rpm` packages and macOS archives are built by the same run. `packaging/aur/PKGBUILD` is updated by hand per release (bump `pkgver`, then `sha256sums` from the tag tarball). CI builds a snapshot release and checks the dependency licenses.
5. Versions follow semver; while the version is below 1.0.0 minor releases may change behavior. Update hints in `switchyard` read the latest non-draft, non-prerelease release.
