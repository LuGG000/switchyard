# switchyard-mod

Optional mod for Claude Code. It talks to the `switchyard` command only
(`status --json`, `handoff`), so switchyard works without it.

- **No status line of its own.** The line under the prompt comes from the
  statusLine that `switchyard run` injects (`main · 5h 20% · 7d 18%`, the active
  account only), so the mod adds nothing there.
- **`/accounts`:** a pane with every account, its usage, and buttons to continue
  the conversation in another account or to start a new one there.
- **`/switch <account> [resume|fresh]`:** the same from the prompt.

A switch needs a restart of `claude` with the other login, which a mod cannot do.
It therefore only asks: `switchyard handoff` tells the launcher of
`switchyard run` to end claude and continue in the chosen account. Under a plain
`claude` nothing happens. If `switchyard` is missing from `PATH` or reports a
status schema the mod does not read, the mod stays quiet and `/accounts` says why.

## Install

```
switchyard mod install
```

or, in a Claude Code session:

```
/plugin install switchyard-mod --marketplace LuGG000/switchyard
```

The plugins directory is shared between profiles, so one installation covers all
of them. Try it from a checkout without installing: `claude --plugin-dir mod`.

## Develop

From the repository root:

```
claude plugin validate mod
claude plugin test mod
```

Type-check with `tsc -p mod` after Claude Code has loaded the mod once from a
folder it may write in (`claude --plugin-dir mod`, interactive): it then lays the
API types into `mod/.claude-plugin/types/`. They are generated, not committed.
