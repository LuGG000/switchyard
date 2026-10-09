# switchyard

Open-source account switcher for Claude Code subscription logins (Pro/Max).

**Status: early development.** Profiles, manual switching and `run` work against
`claude`; limit detection and automatic failover are not implemented yet. See
[docs/PLAN.md](docs/PLAN.md) and [docs/STATUS.md](docs/STATUS.md).

## Usage

```
switchyard init                       # create the default config.toml
switchyard add work1                  # create a profile and log it in
switchyard list                       # profiles and their login state
switchyard status [--json]            # active profile, last use, cooldowns
switchyard run -- --model haiku       # run claude with the active profile
switchyard switch work2 --resume      # switch and continue the current conversation
switchyard switch work2 --fresh       # switch and start a new conversation
switchyard switch work2 --no-launch   # only change the active profile
switchyard repair                     # re-create the shared links
```

Profiles share `projects/`, settings, skills and the other entries listed in
`link` (config) with the default `~/.claude` through symlinks (junctions on
Windows). Without `--resume` or `--fresh`, `carry_context` in `config.toml`
decides. Carrying the context makes the new account process it again with a cold
prompt cache, and it counts against that account's limit.

## Scope

Subscription logins only (`authMethod: claude.ai`). API-key, Console, Bedrock,
Vertex and Foundry accounts are explicitly out of scope.

## Terms of service and risk

This project is not affiliated with or endorsed by Anthropic. It only launches
the official `claude` binary and never reads, copies, logs or sends credentials.
Anthropic's Consumer Terms do not explicitly address multiple accounts, and it
is unclear whether rotating accounts to work around usage limits is tolerated.
Use only your own accounts, one at a time. You accept the residual risk,
including possible account restrictions.
