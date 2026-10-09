# Contributing

- Only permissive dependencies (MIT, BSD, Apache-2.0, ISC); check the license before adding one.
- Do not copy code from other account-switcher projects.
- Never read, copy or log credential files or tokens, and never print raw `claude auth status` output (it contains email and org data).
- Do not change the user's Claude Code settings; hooks and the status line are injected per run with `--settings`.
- Keep changes small and match the surrounding style. Code, comments and commits are in English; commits follow Conventional Commits.
- `gofmt -l .`, `go vet ./...` and `go test ./...` must pass; CI also runs golangci-lint, govulncheck and a race-enabled test run.
- Tests against real accounts: few short prompts, `--model haiku`, run from an empty temp directory.
- See `docs/DEVELOPMENT.md` for build, test and pull request workflow.
