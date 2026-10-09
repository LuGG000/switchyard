import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Snapshot } from '../types'
import { describeProfile, parseStatus, parseSwitchArgs, problem, statusLine } from './status'

const PANE = 'accounts'
const POLL_MS = 30_000

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

async function refresh($: Engine): Promise<Snapshot> {
  const current = await load($)
  await update($, snapshot, () => current)
  $.ui.status(statusLine(current, await $.clock.now()))

  return current
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
    void refresh($)
    $.clock.every(POLL_MS, () => refresh($))

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

    return (
      <Box flexDirection="column">
        {current.profiles.length === 0 && <Text dimColor>No accounts yet. Create one with: switchyard add &lt;name&gt;</Text>}
        {current.profiles.map(p => (
          <Box key={p.name}>
            <Text bold={p.active}>
              {p.active ? '* ' : '  '}
              {p.name}{' '}
            </Text>
            <Text dimColor>{describeProfile(p, now)} </Text>
            {!p.active && (
              <Button
                key={`switch-${p.name}`}
                label="Continue here"
                onPress={async () => $.ui.toast(await handoff($, p.name, '--resume'))}
              />
            )}
            {!p.active && (
              <Button
                key={`fresh-${p.name}`}
                label="New conversation"
                onPress={async () => $.ui.toast(await handoff($, p.name, '--fresh'))}
              />
            )}
          </Box>
        ))}
        <Button key="refresh" label="Refresh" onPress={() => refresh($)} />
      </Box>
    )
  })
}
