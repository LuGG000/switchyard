# switchyard-mod

Optional mod for Claude Code. It talks to the `switchyard` command only
(`status --json`, `config`, `handoff`), so switchyard works without it.

One command, `/switchyard`, opens a pane with two pages:

- **Overview** (the default): a Failover card (at a limit: switch automatically
  or ask me; conversation: take it along or start new; the current choice is
  ticked) and a card per account with its usage bars, reset time and buttons to
  continue the conversation there or to start a new one. The buttons **Style**
  and **Refresh** are at the top.
- **Style:** palettes (theme, dark, light), a preview of the usage colors and a picker for
  every color (theme or a common color name), plus a field for any other color (type
  the slot and the color, for example `background #1e1e1e`). **Back** returns to the
  overview.

For the keyboard the command takes arguments:

```
/switchyard                                  the pane
/switchyard style                            the pane on the style page
/switchyard switch <account> [resume|fresh]  continue in another account
/switchyard mode [auto|ask]                  show or set what happens at a limit
```

The line under the prompt comes from the statusLine that `switchyard run`
injects (`main · 5h 20% · 7d 18% · on limit: ask`, the active account only), so
the mod adds nothing there.

A switch needs a restart of `claude` with the other login, which a mod cannot do.
It therefore only asks: `switchyard handoff` tells the launcher of
`switchyard run` to end claude and continue in the chosen account. Under a plain
`claude` nothing happens. If `switchyard` is missing from `PATH` or reports a
status schema the mod does not read, the mod stays quiet and the pane says why.

## Colors

The pane follows your Claude theme. The colors are set in switchyard, the same
place as the other settings, so the Claude theme and other mods are not touched:

```
switchyard config colors dark                     # a palette: default (theme), dark or light
switchyard config set color_background '#1e1e1e'  # a single color
switchyard config set color_background ''         # back to the theme's
```

The Style page does the same with buttons, pickers and a field. A color is a theme color
(`success`, `warning`, `error`, `subtle`, ...), a color name or hex; empty means
the theme's.

| Setting | Default | Colors |
| --- | --- | --- |
| `color_background` | engine's | background of the whole pane |
| `color_text` | theme | text |
| `color_low` / `color_medium` / `color_high` | success / warning / error | usage bars below 70%, 70-89%, from 90% |
| `color_border_active` | success | border and name of the active account |
| `color_border` | subtle | border of the other cards |

The buttons are drawn by Claude Code and keep its look.

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
