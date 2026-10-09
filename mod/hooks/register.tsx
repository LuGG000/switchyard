import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Settings, Snapshot, Usage } from '../types'
import {
  describeSettings,
  isCoolingDown,
  parseConfig,
  parseFailoverArg,
  PALETTES,
  parseStatus,
  parseSwitchArgs,
  problem,
  resetLabel,
  timeOfDay,
  usageBar,
  usageLevel,
} from './status'
import { palette } from './palette'

const PANE = 'accounts'


const snapshot = atom({ plugin: 'switchyard-mod', key: 'snapshot' } as const, null)

type Engine = EngineInterface

/** Reads the failover settings; null when the installed switchyard has no `config --json` the mod reads. */
async function loadSettings($: Engine): Promise<Settings | null> {
  try {
    const run = await $.process.run(['switchyard', 'config', '--json'], { timeoutMs: 10_000 })

    return run.exitCode === 0 ? parseConfig(run.stdout) : null
  } catch {
    return null
  }
}

/** Reads the state of switchyard from its CLI; a missing or failing command is a snapshot, not an error. */
async function load($: Engine): Promise<Snapshot> {
  try {
    const run = await $.process.run(['switchyard', 'status', '--json'], { timeoutMs: 10_000 })
    if (run.exitCode !== 0) {
      return { kind: 'unavailable', reason: `exit code ${run.exitCode}` }
    }
    const status = parseStatus(run.stdout)

    return status.kind === 'ok' ? { ...status, settings: await loadSettings($) } : status
  } catch {
    return { kind: 'unavailable', reason: 'command not found' }
  }
}

async function refresh($: Engine): Promise<void> {
  const current = await load($)
  await update($, snapshot, () => current)
}

/** Changes one failover setting; the answer is the text to show. */
async function changeSetting($: Engine, key: string, value: string): Promise<string> {
  try {
    const run = await $.process.run(['switchyard', 'config', 'set', key, value], { timeoutMs: 10_000 })
    if (run.exitCode !== 0) {
      return run.stderr.trim() || `switchyard config failed (exit code ${run.exitCode})`
    }
  } catch {
    return 'switchyard was not found in PATH'
  }
  await refresh($)

  return `${key} = ${value}`
}

/** Sets all pane colors to a palette of switchyard; the answer is the text to show. */
async function changeColors($: Engine, name: string): Promise<string> {
  try {
    const run = await $.process.run(['switchyard', 'config', 'colors', name], { timeoutMs: 10_000 })
    if (run.exitCode !== 0) {
      return run.stderr.trim() || `switchyard config colors failed (exit code ${run.exitCode})`
    }
  } catch {
    return 'switchyard was not found in PATH'
  }
  await refresh($)

  return `Colors: ${name}`
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
    await $.command.register({
      name: 'failover',
      description: 'Show or set what happens at a limit: switch automatically or ask',
      argumentHint: '[auto|ask]',
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

  on('command.run', { command: 'failover' }, async ($, e) => {
    const choice = parseFailoverArg(e.args)
    if (choice === null) {
      return { text: 'Usage: /failover [auto|ask]' }
    }
    if (choice === '') {
      await refresh($)
      const current = await read($, snapshot)

      return { text: current?.kind === 'ok' && current.settings ? describeSettings(current.settings) : 'switchyard settings are not available.' }
    }

    return { text: await changeSetting($, 'mode', choice) }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const { Box, Button, Text } = $.ui.resolve(e)
    const current = await read($, snapshot)
    const now = await $.clock.now()
    const hint = problem(current)
    const colors = palette(current?.kind === 'ok' ? current.settings?.colors : null)
    const tint = (percent: number): string => ({ ok: colors.low, warn: colors.medium, high: colors.high })[usageLevel(percent)]
    // A background has to fill the whole pane, not just the height of its content.
    const fill = colors.background
      ? { backgroundColor: colors.background, width: e.props.bodyColumns, minHeight: e.props.scroll.bodyRows }
      : {}

    if (current?.kind !== 'ok' || hint) {
      return (
        <Box flexDirection="column" {...fill}>
          <Text color={colors.text} dimColor>{hint}</Text>
          <Button key="refresh" label="Refresh" onPress={() => refresh($)} />
        </Box>
      )
    }

    const usageRow = (label: string, usage: Usage | null) => (
      <Box key={label}>
        <Text color={colors.text} dimColor>{label} </Text>
        {usage ? (
          <Box>
            <Text color={tint(usage.used_percent)}>{usageBar(usage.used_percent)}</Text>
            <Text color={colors.text}> {String(Math.round(usage.used_percent)).padStart(3)}%</Text>
            <Text color={colors.text} dimColor> {resetLabel(usage.resets_at, now)}</Text>
          </Box>
        ) : (
          <Text color={colors.text} dimColor>no data yet</Text>
        )}
      </Box>
    )

    const choice = (key: string, label: string, isCurrent: boolean, setting: string, value: string) => (
      <Button
        key={key}
        label={isCurrent ? `✓ ${label}` : label}
        variant={isCurrent ? 'primary' : undefined}
        onPress={async () => $.ui.toast(await changeSetting($, setting, value))}
      />
    )
    const settings = current.settings

    return (
      <Box flexDirection="column" gap={1} {...fill}>
        {settings && (
          <Box flexDirection="column" borderStyle="round" borderColor={colors.border} paddingX={1}>
            <Text bold color={colors.text}>Failover</Text>
            <Box gap={1}>
              <Text color={colors.text} dimColor>At a limit</Text>
              {choice('mode-auto', 'switch automatically', settings.mode === 'auto', 'mode', 'auto')}
              {choice('mode-ask', 'ask me', settings.mode === 'ask', 'mode', 'ask')}
            </Box>
            <Box gap={1}>
              <Text color={colors.text} dimColor>Conversation</Text>
              {choice('carry-on', 'take it along', settings.carry_context, 'carry_context', 'true')}
              {choice('carry-off', 'start new', !settings.carry_context, 'carry_context', 'false')}
            </Box>
            <Box gap={1}>
              <Text color={colors.text} dimColor>Colors</Text>
              {PALETTES.map(name => (
                <Button
                  key={`colors-${name}`}
                  label={name === 'default' ? 'theme' : name}
                  onPress={async () => $.ui.toast(await changeColors($, name))}
                />
              ))}
            </Box>
          </Box>
        )}
        {current.profiles.length === 0 && <Text color={colors.text} dimColor>No accounts yet. Create one with: switchyard add &lt;name&gt;</Text>}
        {current.profiles.map(p => (
          <Box
            key={p.name}
            flexDirection="column"
            borderStyle="round"
            borderColor={p.active ? colors.borderActive : colors.border}
            paddingX={1}
          >
            <Box>
              <Text bold color={p.active ? colors.borderActive : colors.text}>
                {p.active ? '● ' : '○ '}
                {p.name}
              </Text>
              {p.active && <Text color={colors.text} dimColor> active</Text>}
              {p.cooldown_until !== null && isCoolingDown(p, now) && (
                <Text color={colors.high}> limit until {timeOfDay(p.cooldown_until)}</Text>
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
          <Text color={colors.text} dimColor>A switch restarts claude (needs switchyard run).</Text>
        </Box>
      </Box>
    )
  })
}
