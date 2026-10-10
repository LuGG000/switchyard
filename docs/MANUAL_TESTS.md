# Manual tests

Things the automated tests cannot see: the real TUI, a real `claude`, a real terminal. Each test
uses a simulated limit, so no real limit is needed. Keep prompts short and use `--model haiku`.
Both tests need two logged-in profiles (`switchyard add main`, `switchyard add second`; the same
account may be used for both) and `switchyard` in the `PATH`.

A limit is simulated with the real hook code. In a second terminal:

```
echo '{}' | switchyard hook stop-failure --profile main      # or the name of the active profile
```

It marks the profile as limited and writes the switch request, exactly as claude's StopFailure
hook does. Afterwards the profile stays in cooldown for 30 minutes; clear it with
`switchyard status` to see it and by deleting the profile's entry or waiting (or use another
`SWITCHYARD_DATA_DIR` for the test).

## 1. The buttons in the mod (any OS)

1. Install the mod (`switchyard mod install`, restart claude), or load it for one run with
   `switchyard run -p main -- --plugin-dir <checkout>/mod --model haiku`.
2. `switchyard config set mode ask`.
3. In the claude session: `/switchyard`, and send one short prompt ("Remember the word kiwi").
4. In the second terminal: simulate the limit (see above).

Expected, within a few seconds:

- the pane opens by itself (a toast says that main reached its limit) with a red card
  "main reached its limit", the seconds left, and the buttons **Continue in second**,
  **New conversation in second**, **Stay here**;
- claude keeps running (the prompt still accepts input);
- **Continue in second** ends claude and starts it again in `second`; asking "which word did I
  ask you to remember?" answers kiwi. **New conversation** starts empty.
- With **Stay here** nothing happens and claude keeps running.
- Without a click, after two minutes the terminal asks the same question.
- Close the mod's pane and wait ten seconds before simulating: then the terminal asks at once if
  the mod does not poll (e.g. claude started without the mod).

Note down anything odd: the look of the card (colors, widths), whether the pane opens in a narrow
terminal, whether clicks register.

## 2. Ending claude on Linux or macOS (issue #2)

1. `switchyard config set mode auto`, then `switchyard run -p main -- --model haiku`.
2. Send "Remember the word kiwi" and wait for the answer.
3. In the second terminal: simulate the limit.

Expected: claude ends within about five seconds (SIGTERM, then a kill after the grace period),
`second` starts with the conversation, and "which word did I ask you to remember?" answers kiwi.
Then check:

- how long it took from the simulated limit until the new claude appeared;
- after quitting everything with `/exit`: typed characters are echoed and Enter works in the
  terminal (the terminal state is restored); `stty -a | head -3` shows `icanon` and `echo` set;
- `ls ~/.claude/projects/` has one directory for your test directory and the newest `.jsonl` in it
  ends with a complete line (`tail -c 200 file.jsonl`).

Run it once more with `mode ask` and answer `q` at the terminal question; the terminal must be
usable afterwards.

## Cleaning up

Remove the test directory and its entry under `~/.claude/projects/` (name the paths explicitly)
and run `switchyard status`: if the simulated limit left a cooldown, wait it out or delete the
profile's `cooldown_until` in `state.json` while no switchyard is running.
