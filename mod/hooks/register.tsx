import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Snapshot, Usage } from '../types'
import {
  isCoolingDown,
  parseStatus,
  parseSwitchArgs,
  problem,
  resetLabel,
  timeOfDay,
  usageBar,
  usageLevel,
} from './status'
import type { Level } from './status'

const PANE = 'accounts'

/** Theme colors of the usage levels, so the pane follows the person's theme. */
const COLOR: Record<Level, string> = { ok: 'success', warn: 'warning', high: 'error' }

const snapshot = atom({ plugin: 'switchyard-mod', key: 'snapshot' } as const, null)

type Engine = EngineInterface

/** Reads the state of switchyard from its CLI; a missing or failing command is a snapshot, not an error. */
async function load($: Engine): Promise<Snapshot> {
  try {
    const run = await $.process.run(['switchyard', 'status', '--json'], { timeoutMs: 10_000 })
    if (run.exitCode !== 0) {
      return { kind: 'unavailable', reason: `exit code ${run.exitCode}` }
    }

    return parseStatus(run.stdout)
  } catch {
    return { kind: 'unavailable', reason: 'command not found' }
  }
}

async function refresh($: Engine): Promise<void> {
  const current = await load($)
  await update($, snapshot, () => current)
}

/** Asks the launcher to continue in `name`; the answer is the text to show. */
async function handoff($: Engine, name: string, flag?: '--resume' | '--fresh'): Promise<string> {
  const argv = ['switchyard', 'handoff', name, '--session', await $.session.id()]
  if (flag) {
    argv.push(flag)
  }
  try {
    const run = await $.process.run(argv, { timeoutMs: 10_000 })
    if (run.exitCode !== 0) {
      return run.stderr.trim() || `switchyard handoff failed (exit code ${run.exitCode})`
    }
  } catch {
    return 'switchyard was not found in PATH'
  }

  return `Handoff to ${name} requested. If claude was started with switchyard run, it restarts in ${name}.`
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'accounts',
      description: 'Show the switchyard accounts and their usage',
    })
    await $.command.register({
      name: 'switch',
      description: 'Continue this session in another switchyard account',
      argumentHint: '<account> [resume|fresh]',
    })

    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    void refresh($)

    return next(e)
  })

  on('command.run', { command: 'accounts' }, async $ => {
    await refresh($)
    await $.ui.open({ id: PANE, title: 'Accounts' })

    return { text: 'Accounts pane opened.' }
  })

  on('command.run', { command: 'switch' }, async ($, e) => {
    const parsed = parseSwitchArgs(e.args)
    if (!parsed) {
      return { text: 'Usage: /switch <account> [resume|fresh]' }
    }

    return { text: await handoff($, parsed.name, parsed.flag) }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Button, Text } = $.ui.resolve(e)
    const current = await read($, snapshot)
    const now = await $.clock.now()
    const hint = problem(current)

    if (current?.kind !== 'ok' || hint) {
      return (
        <Box flexDirection="column">
          <Text dimColor>{hint}</Text>
          <Button key="refresh" label="Refresh" onPress={() => refresh($)} />
        </Box>
      )
    }

    const usageRow = (label: string, usage: Usage | null) => (
      <Box key={label}>
        <Text dimColor>{label} </Text>
        {usage ? (
          <Box>
            <Text color={COLOR[usageLevel(usage.used_percent)]}>{usageBar(usage.used_percent)}</Text>
            <Text> {String(Math.round(usage.used_percent)).padStart(3)}%</Text>
            <Text dimColor> {resetLabel(usage.resets_at, now)}</Text>
          </Box>
        ) : (
          <Text dimColor>no data yet</Text>
        )}
      </Box>
    )

    return (
      <Box flexDirection="column" gap={1}>
        {current.profiles.length === 0 && <Text dimColor>No accounts yet. Create one with: switchyard add &lt;name&gt;</Text>}
        {current.profiles.map(p => (
          <Box
            key={p.name}
            flexDirection="column"
            borderStyle="round"
            borderColor={p.active ? 'success' : 'subtle'}
            paddingX={1}
          >
            <Box>
              <Text bold color={p.active ? 'success' : undefined}>
                {p.active ? '● ' : '○ '}
                {p.name}
              </Text>
              {p.active && <Text dimColor> active</Text>}
              {p.cooldown_until !== null && isCoolingDown(p, now) && (
                <Text color="error"> limit until {timeOfDay(p.cooldown_until)}</Text>
              )}
            </Box>
            {usageRow('5h', p.five_hour)}
            {usageRow('7d', p.seven_day)}
            {!p.active && (
              <Box gap={1} marginTop={1}>
                <Button
                  key={`switch-${p.name}`}
                  label="Continue here"
                  variant="primary"
                  onPress={async () => $.ui.toast(await handoff($, p.name, '--resume'))}
                />
                <Button
                  key={`fresh-${p.name}`}
                  label="New conversation"
                  onPress={async () => $.ui.toast(await handoff($, p.name, '--fresh'))}
                />
              </Box>
            )}
          </Box>
        ))}
        <Box gap={1}>
          <Button key="refresh" label="Refresh" onPress={() => refresh($)} />
          <Text dimColor>A switch restarts claude (needs switchyard run).</Text>
        </Box>
      </Box>
    )
  })
}
