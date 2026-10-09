# switchyard

Open-source account switcher with limit failover for Claude Code subscription
logins (Pro/Max). Keep several of your own logins as profiles; when one hits its
usage limit, switchyard continues the conversation in the next one.

**Status: early development.** Profiles, switching, `run` and the failover
(headless and interactive) work against the real `claude`. A real usage limit has
not been observed yet, so the limit detection is tested with simulated limits only.

## How it works

- A **profile** is a Claude config directory with its own login. switchyard
  starts `claude` with that profile's `CLAUDE_CONFIG_DIR`; it never reads, copies,
  logs or sends credentials.
- Profiles share `projects/`, settings, skills and the other entries listed in
  `link` (config) with your default `~/.claude` through symlinks (junctions on
  Windows), so a conversation can be resumed in another profile.
- `switchyard run` injects a status line and hooks per run with `--settings`
  (your own Claude settings are never changed). They record usage per profile and
  report a limit.
- At a limit (or when `proactive_threshold` is reached) the launcher ends `claude`
  and starts it again in the next profile, continuing the conversation when
  `carry_context` is on. Headless runs (`run -- -p "prompt"`) fail over the same way.

## Install

From source (Go 1.25 or newer):

```
go install github.com/LuGG000/switchyard/cmd/switchyard@latest
```

Or download the archive for Linux or Windows (amd64, arm64) from the
[releases page](https://github.com/LuGG000/switchyard/releases), unpack it and put
`switchyard` on your `PATH`.
`claude` must be installed and on the `PATH`.

## Usage

```
switchyard init                       # create the default config.toml
switchyard add work1                  # create a profile and log it in
switchyard list                       # profiles and their login state
switchyard status [--json]            # active profile, last use, cooldowns
switchyard run -- --model haiku       # run claude with the active profile
switchyard run -- -p "prompt"         # headless run that fails over at a limit
switchyard switch work2 --resume      # switch and continue the current conversation
switchyard switch work2 --fresh       # switch and start a new conversation
switchyard switch work2 --no-launch   # only change the active profile
switchyard handoff work2              # ask the running session to continue in work2
switchyard config                     # failover settings
switchyard config set mode auto       # switch at a limit without asking
switchyard config colors dark         # colors of the mod pane: default, dark or light
switchyard mod install                # install the optional Claude Code mod
switchyard login work1                # log a profile in again
switchyard update                     # check for a newer release
switchyard repair                     # re-create the shared links
switchyard doctor                     # check logins, settings and links
```

Without `--resume` or `--fresh`, `carry_context` decides. Carrying the context
makes the new account process it again with a cold prompt cache, and it counts
against that account's limit.

## Configuration

`switchyard init` writes `config.toml`; `switchyard config set <key> <value>` edits
one setting and a running session picks it up at its next limit.

| Key | Values | Meaning |
| --- | --- | --- |
| `mode` | `ask` (default), `auto` | at a limit, ask in the terminal or switch at once |
| `carry_context` | `true` (default), `false` | continue the conversation in the next profile |
| `strategy` | `sequential` (default), `most-headroom`, `round-robin` | how the next profile is chosen |
| `proactive_threshold` | `0` (off) to `100` | five-hour usage in percent that triggers a switch |
| `update_check` | `true` (default), `false` | mention a newer release after `status`, `list` and `doctor` |
| `color_*` | color or empty | colors of the mod pane, see [mod/README.md](mod/README.md) |

## Updates

Releases are tagged `vX.Y.Z` and listed on the [releases page](https://github.com/LuGG000/switchyard/releases)
with notes on what changed. To hear about them, use *Watch > Custom > Releases* on GitHub.

switchyard also tells you: after `status`, `list` and `doctor` it prints a one-line hint
when a newer release exists (checked at most once a day, only in a terminal), and
`switchyard update` checks right away. It never downloads or replaces itself; update with
the same command you installed with (`go install …@latest` or a new archive). Turn the hint
off with `switchyard config set update_check false` or `SWITCHYARD_NO_UPDATE_CHECK=1`. The
check only asks GitHub for the latest release and sends nothing about you.

## Optional mod

The mod adds a `/switchyard` pane inside Claude Code: usage bars per account,
buttons to continue in another account, and the failover mode and colors as
settings. switchyard works without it.

```
switchyard mod install
```

See [mod/README.md](mod/README.md). The mod API of Claude Code is in early access
and may change between releases.

## Documentation

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): packages and the decisions taken
- [docs/SPIKE.md](docs/SPIKE.md): what was verified against `claude`
- [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md): build, test and contribute
- [CONTRIBUTING.md](CONTRIBUTING.md), [SECURITY.md](SECURITY.md)

## Scope

Subscription logins only (`authMethod: claude.ai`). API-key, Console, Bedrock,
Vertex and Foundry accounts are explicitly out of scope. Supported: Linux and
Windows; WSL is not verified yet.

## Terms of service and risk

This project is not affiliated with or endorsed by Anthropic. It only launches
the official `claude` binary and never reads, copies, logs or sends credentials.
Anthropic's Consumer Terms do not explicitly address multiple accounts, and it
is unclear whether rotating accounts to work around usage limits is tolerated.
Use only your own accounts, one at a time. You accept the residual risk,
including possible account restrictions.

## License

Apache-2.0, see [LICENSE](LICENSE) and [NOTICE](NOTICE).
