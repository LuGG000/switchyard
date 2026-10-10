# Manual tests

Things the automated tests cannot see: the real TUI, a real `claude`, a real terminal. Tests 1 and 2
use a simulated limit, so no real limit is needed; test 3 needs a real one. Keep prompts short and use `--model haiku`.
Tests 1 and 2 need two logged-in profiles (`switchyard add main`, `switchyard add second`; the same
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

## 3. A real usage limit (issue #1)

This is the one thing nothing here has seen yet: what `claude` really reports and does when a
subscription account is used up. It needs two independent accounts, one of them near its limit. Use
only your own accounts, one at a time, and keep prompts short. Do not paste emails, organization data,
tokens or whole outputs anywhere; the commands below keep only the fields that matter.

**Getting to the limit.** Check `switchyard status` (or `/switchyard`): the test is cheap when an account
is already above 90% of the five-hour window. If it is not, work normally until it is; there is no
shortcut. Turn the thresholds off for the test (`switchyard config set proactive_threshold 0`) so that
only the real limit triggers anything, and use `mode ask` first.

### 3a. Headless: what the limit looks like

Run one headless prompt in the profile that is near its limit, with claude itself, until it fails:

```
# bash
CLAUDE_CONFIG_DIR=<data dir>/profiles/main claude -p --output-format stream-json --verbose --model haiku "say ok" > out.jsonl; echo "exit=$?"
# PowerShell
$env:CLAUDE_CONFIG_DIR = "<data dir>\profiles\main"; claude -p --output-format stream-json --verbose --model haiku "say ok" > out.jsonl; "exit=$LASTEXITCODE"
```

Then keep only the interesting lines (no `jq` needed in PowerShell):

```
Get-Content out.jsonl | ForEach-Object { $e = $_ | ConvertFrom-Json; if ($e.type -in 'rate_limit_event','result' -or $e.subtype -eq 'api_retry') { [pscustomobject]@{ type=$e.type; subtype=$e.subtype; error=$e.error; is_error=$e.is_error; result=$e.result; status=$e.rate_limit_info.status; kind=$e.rate_limit_info.rateLimitType; five=$e.rate_limit_info.unifiedWindows.five_hour.utilization; seven=$e.rate_limit_info.unifiedWindows.seven_day.utilization } } }
# bash: jq -c 'select(.type=="rate_limit_event" or .type=="result" or .subtype=="api_retry") | {type,subtype,error,is_error,result,status:.rate_limit_info.status,kind:.rate_limit_info.rateLimitType}' out.jsonl
```

Write down: the `status` values before and at the limit (`allowed`, a warning value, a rejected value),
the exit code, the text of the `result` line, and whether an `api_retry` with `error: rate_limit` appears.
`internal/detector` counts a limit only with a non-zero exit and a status that does not start with
`allowed`, or the text of the error; if the real output differs, that is the finding.

Then the same through switchyard, with a second profile that is not limited:
`switchyard run -p main -- -p "say ok"`. Expected: `profile main reached its limit, continuing with second`,
and `switchyard status` shows `main` in cooldown until the real reset time.

### 3b. Interactive: what claude does at a limit, and the StopFailure input

Capture what claude passes to the `StopFailure` hook, without switchyard in between (the file may hold
more than switchyard reads; look at it and share only the key names and the `session_id` shape):

```
# bash
claude --settings '{"hooks":{"StopFailure":[{"matcher":"rate_limit","hooks":[{"type":"command","command":"cat > /tmp/stopfailure.json"}]}]}}' --model haiku
# PowerShell: use a command that writes stdin to a file, e.g. "powershell -NoProfile -Command \"$input | Out-File $env:TEMP\stopfailure.json\""
```

Send prompts until the limit is hit. Write down: whether the hook fired at all, the keys of its input,
what the screen shows (a message, a prompt to wait or retry, whether claude stays open), whether the
status line (`/switchyard` or `switchyard status --short`) shows 100% and the reset time, and how long
it took.

Then repeat with switchyard, `switchyard run -p main -- --model haiku` in `mode auto`: the session should
end, the second profile should start with the conversation, and `switchyard status` should show the
real cooldown. Try `mode ask` with the mod's buttons as in test 1.

### 3c. What to bring back

The values and observations from 3a and 3b (no secrets). Add them to `docs/SPIKE.md` (summary table and a
section), correct the detector or the hook where the real behavior differs, and update the README
status. Afterwards remove the test conversations and clear any leftover cooldown (see below).

## Cleaning up

Remove the test directory and its entry under `~/.claude/projects/` (name the paths explicitly)
and run `switchyard status`: if the simulated limit left a cooldown, wait it out or delete the
profile's `cooldown_until` in `state.json` while no switchyard is running.
