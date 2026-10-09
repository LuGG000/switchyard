# switchyard-mod

Optional mod for Claude Code. It talks to the `switchyard` command only
(`status --json`, `config`, `handoff`), so switchyard works without it.

- **No status line of its own.** The line under the prompt comes from the
  statusLine that `switchyard run` injects (`main · 5h 20% · 7d 18%`, the active
  account only), so the mod adds nothing there.
- **`/accounts`:** a pane with a Failover card (at a limit: switch automatically or ask me;
  conversation: take it along or start new; the current choice is ticked) and a card
  per account with its usage and buttons to continue
  the conversation in another account or to start a new one there.
- **`/switch <account> [resume|fresh]`:** the same from the prompt.
- **`/failover [auto|ask]`:** shows or sets what happens at a limit. The setting lives in
  switchyard's `config.toml` (`switchyard config set mode auto` does the same) and the
  status line shows it as `on limit: auto` or `on limit: ask`; a running session
  picks the change up at its next limit.

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

## Colors

The pane follows your Claude theme. To change its colors, use the mod's own
options (`/config`, the `switchyard-mod` rows, or `pluginConfigs` in your
settings): they belong to this mod alone, the theme and other mods are not
touched. Each takes a theme color (`success`, `warning`, `error`, `subtle`, ...),
a color name or hex (`#1e1e1e`); empty keeps the default.

| Option | Default | Colors |
| --- | --- | --- |
| `background` | engine's | background of the whole pane |
| `text` | theme | text |
| `usageLow` / `usageMedium` / `usageHigh` | success / warning / error | usage bars below 70%, 70-89%, from 90% |
| `borderActive` | success | border and name of the active account |
| `border` | subtle | border of the other cards |

The buttons are drawn by Claude Code and keep its look.
