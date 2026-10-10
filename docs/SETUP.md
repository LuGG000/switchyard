# Setup guide

A complete walk-through from nothing to a working failover. The README has the five-line version.

## What you need

- **Claude Code** (`claude`) installed and on your `PATH`. Tested with 2.1.296; switchyard uses its
  `--settings` option, hooks and status line, so very old versions may not work.
- **Subscription logins** (Pro or Max), one per account. API keys, Console, Bedrock, Vertex and Foundry
  accounts are not supported.
- **Windows, Linux or macOS.** Windows is verified by hand, Linux and macOS are built and tested in CI
  (see [Scope](../README.md#scope)).
- **Go 1.26 or newer, only to build from source.** The release binaries need nothing else: one
  executable, no runtime dependencies.
- The optional mod needs a Claude Code version that supports mods (an early-access feature; tested with 2.1.296).

## 1. Install switchyard

Pick one:

- **Release archive** (all systems): download the file for your system from the
  [releases page](https://github.com/LuGG000/switchyard/releases), unpack it and put `switchyard`
  (`switchyard.exe` on Windows) in a folder that is on your `PATH`, for example `~/.local/bin` or
  `%USERPROFILE%\.local\bin`. To check the download, see "Verify a download" in the README.
- **deb / rpm** (Linux): install the package from the releases page with your package manager.
- **From source**: `go install github.com/LuGG000/switchyard/cmd/switchyard@latest`.
- **Arch**: the recipes are in `packaging/aur` and `packaging/aur-bin`; they are not on the AUR yet.

Open a new terminal and check both programs are found:

```
switchyard --version
claude --version
```

## 2. Create the config and the first profile

```
switchyard init
```

This writes `config.toml` (Windows `%AppData%\switchyard`, Linux `~/.config/switchyard`, macOS
`~/Library/Application Support/switchyard`). If you already use claude, `init` finds your subscription
login in `~/.claude` and offers it as the first profile, without a new login. You can also do it later:

```
switchyard add main --existing
```

## 3. Add the other accounts

```
switchyard add second
```

This creates a profile and runs claude's login for it; follow the browser prompt and sign in with the **other**
account. If the browser stays signed in to the first account, sign out there or use a private window. Repeat for
every further account (profile names: 1-32 lowercase letters, digits, `-` or `_`).

## 4. Check it

```
switchyard list       # every profile and its login ("pro", "max")
switchyard doctor     # logins, settings that could override them, shared links
```

`doctor` prints `ok`, `warn` or `error` per check and exits with 1 when it finds an error. Each profile shares
your sessions, settings, skills and plugins with `~/.claude`, so a conversation can continue in another account.

## 5. Start claude through switchyard

```
switchyard run
```

Options for claude go after `--` (`switchyard run -- --model opus`). To make a plain `claude` do this, add
the function from `switchyard shell-init <bash|zsh|fish|powershell>` to your shell startup file (see the README).

## 6. Decide what happens at a limit

```
switchyard config                      # show the settings
switchyard config set mode ask         # ask (default) or auto: switch without asking
switchyard config set carry_context true
switchyard config set strategy sequential
switchyard config set proactive_threshold 90   # switch at 90 % of the five-hour window, 0 = off
```

`carry_context` continues the conversation in the next account; that account then reads the whole conversation
again, which counts against its own limit. A running session picks up a change at its next limit.

## 7. Optional: the mod

```
switchyard mod install
```

Restart claude and type `/switchyard`: usage per account, buttons to switch, the "limit reached" question as
buttons while claude keeps running (mode `ask`), the failover settings and the colors. See `mod/README.md`.
The mod only shows and controls things; failover works without it, but only for claude started through
`switchyard run`.

## 8. Updates

`switchyard update` checks for a newer release and `switchyard update --install` installs it; a running claude
session is not interrupted. The mod is updated with `claude plugin update switchyard-mod@switchyard`.

## Removing switchyard

1. Remove the mod: `claude plugin uninstall switchyard-mod@switchyard`.
2. Delete the `switchyard` binary.
3. Delete the profile folder (`%LocalAppData%\switchyard` on Windows, `~/.local/share/switchyard` on
   Linux and macOS) and the config folder from step 2.

   **Careful:** a profile made with `--existing` is a link to your real `~/.claude`. Remove such a link alone
   first (Windows: `rmdir <data folder>\profiles\<name>`; Linux/macOS: `rm <data folder>/profiles/<name>`, no
   trailing slash and no `-r`). Deleting the folder recursively while the link exists can delete your
   sessions. The other profiles hold only that account's login and can be deleted normally.

switchyard never changes your own Claude settings; it adds hooks and the status line per run only.
