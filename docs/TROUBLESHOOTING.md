# Troubleshooting

Start with `switchyard doctor`: it checks the logins, the settings of every profile and the shared links, and
prints a fix for most findings. Nothing in it changes your files.

## `claude executable not found in PATH`

switchyard starts the official `claude`; it must be installed and found through `PATH` in the same terminal.
Check with `claude --version`, open a new terminal after installing, and on Windows make sure the folder is
in the user `PATH`. `switchyard remove` is the only profile command that works without `claude`.

## A profile shows "logged out" or "not logged in"

The login of that profile has expired or never finished. Log it in again:

```
switchyard login <name>
```

`switchyard list` shows the state of every profile. Each profile has its own login; logging in to one does
not affect the others.

## "login is ..., not a claude.ai subscription"

`claude auth status` reports another method than a Pro or Max login (`api_key`, `oauth_token`). The usual
causes are credential variables in your environment (`ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`,
`CLAUDE_CODE_OAUTH_TOKEN`), or an `apiKeyHelper` or `env` entry in the profile's `settings.json`. `doctor`
names which one. `switchyard run` removes the variables from the environment of the claude it starts, but a
plain `claude` does not, so unset them in your shell too. Only subscription logins are supported.

## `doctor` reports a broken link, a copy or a conflict

Profiles share `projects/`, `settings.json` and a few more entries with your default config directory through
links. A tool can replace a link by a copy (claude rewrites `settings.json` and so replaces a hard link on
Windows), or the shared entry can move. Run:

```
switchyard repair
```

It re-creates links that are broken, missing or identical copies. A real conflict, where the profile holds a
different file, is never overwritten: look at both files and remove the one you do not want, then run
`repair` again.

## Every profile is at its limit (exit code 75)

`switchyard run` and `switchyard switch` stop with the earliest reset time when no profile is available, and
exit with code 75 so a script can wait and retry. `switchyard status` shows the cooldowns and the usage per
profile. A cooldown from a simulated limit ends by itself; how to clear it earlier is in
[docs/MANUAL_TESTS.md](MANUAL_TESTS.md).

## It asks instead of switching, or a run stops with exit code 76

Automatic switches are limited: `min_switch_interval_minutes` after the last one and `max_auto_switches_per_day`
per 24 hours (README, "Cautious switching"). Over a limit the `auto` mode asks, and a headless run stops with
the time when switching is allowed again. Change or turn off a limit with `switchyard config set
min_switch_interval_minutes <n>` or `max_auto_switches_per_day <n>` (0 = off).

## The mod does not show up or the buttons do nothing

- Install it with `switchyard mod install`, then start claude through `switchyard run` and open `/switchyard`.
  A claude started directly has no failover and no mod data.
- The mod needs a Claude Code version with mod support (tested with 2.1.296).
- At a limit the question is asked in the mod for two minutes; without an answer, or when no mod is running,
  switchyard asks in the terminal. Failover works without the mod.

## `add --existing` cannot confirm the login (macOS)

On macOS claude may keep the login in the Keychain under a name that depends on the config directory, so the
login check can fail for the adopted `~/.claude`. Log in once through the profile: `switchyard login <name>`.
This is not verified on macOS.

## A profile cannot be removed

`switchyard remove <name>` removes a profile and never follows links. If the link points to a directory that
is gone, the profile is not found any more; remove the dangling link by hand (Windows: `rmdir <data
folder>\profiles\<name>`, Linux and macOS: `rm <data folder>/profiles/<name>`, without a trailing slash and
without `-r`). Never delete the data folder recursively while a profile made with `--existing` still links to
your real `~/.claude`.

## Where switchyard keeps its files

- Profiles and `state.json`: `%LocalAppData%\switchyard` on Windows, `~/.local/share/switchyard` on Linux and
  macOS (`$XDG_DATA_HOME/switchyard` if that is set; override: `SWITCHYARD_DATA_DIR`).
- `config.toml`: `switchyard/` in your user config folder (`%AppData%` on Windows, `~/.config` on Linux, `~/Library/Application Support` on macOS; override: `SWITCHYARD_CONFIG_DIR`).

## Still stuck

Open an issue with the output of `switchyard doctor` and `switchyard --version`. Do not paste credentials,
tokens or the raw output of `claude auth status`; it contains your email address.
