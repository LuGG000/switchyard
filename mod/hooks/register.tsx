import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Page, Settings, Snapshot, Usage } from '../types'
import { palette } from './palette'
import {
  COLOR_SLOTS,
  describeSettings,
  isCoolingDown,
  PALETTES,
  parseCommand,
  NAMED_COLORS,
  parseConfig,
  parseCustomColor,
  parseStatus,
  problem,
  resetLabel,
  timeOfDay,
  usageBar,
  usageLevel,
  USAGE,
} from './status'

const PANE = 'switchyard'

const snapshot = atom({ plugin: 'switchyard-mod', key: 'snapshot' } as const, null)
const page = atom({ plugin: 'switchyard-mod', key: 'page' } as const, 'main')

type Engine = EngineInterface

/** Runs `switchyard` with `argv`; the text of a refusal when it fails, undefined when it worked. */
async function run($: Engine, argv: string[]): Promise<{ stdout: string } | { error: string }> {
  try {
    const result = await $.process.run(['switchyard', ...argv], { timeoutMs: 10_000 })

    return result.exitCode === 0
      ? { stdout: result.stdout }
      : { error: result.stderr.trim() || `switchyard ${argv[0]} failed (exit code ${result.exitCode})` }
  } catch {
    return { error: 'switchyard was not found in PATH' }
  }
}

/** Reads the failover settings; null when the installed switchyard has no `config --json` the mod reads. */
async function loadSettings($: Engine): Promise<Settings | null> {
  const result = await run($, ['config', '--json'])

  return 'stdout' in result ? parseConfig(result.stdout) : null
}

/** Reads the state of switchyard from its CLI; a missing or failing command is a snapshot, not an error. */
async function load($: Engine): Promise<Snapshot> {
  const result = await run($, ['status', '--json'])
  if ('error' in result) {
    return { kind: 'unavailable', reason: result.error }
  }
  const status = parseStatus(result.stdout)

  return status.kind === 'ok' ? { ...status, settings: await loadSettings($) } : status
}

async function refresh($: Engine): Promise<void> {
  const current = await load($)
  await update($, snapshot, () => current)
}

/** Runs a command that changes switchyard, then refreshes; the answer is the text to show. */
async function change($: Engine, argv: string[], done: string): Promise<string> {
  const result = await run($, argv)
  if ('error' in result) {
    return result.error
  }
  await refresh($)

  return done
}

const setSetting = ($: Engine, key: string, value: string) => change($, ['config', 'set', key, value], `${key} = ${value}`)
const setPalette = ($: Engine, name: string) => change($, ['config', 'colors', name], `Colors: ${name}`)

/** Asks the launcher to continue in `name`; the answer is the text to show. */
async function handoff($: Engine, name: string, flag?: '--resume' | '--fresh'): Promise<string> {
  const result = await run($, ['handoff', name, '--session', await $.session.id(), ...(flag ? [flag] : [])])
  if ('error' in result) {
    return result.error
  }

  return `Handoff to ${name} requested. If claude was started with switchyard run, it restarts in ${name}.`
}

async function open($: Engine, to: Page): Promise<void> {
  await update($, page, () => to)
  await refresh($)
  await $.ui.open({ id: PANE, title: 'switchyard' })
}

export const register: Register = on => {
  on('session.start', async ($, e, next) => {
    await $.command.register({
      name: 'switchyard',
      description: 'Accounts, failover and style of switchyard',
      argumentHint: '[style | switch <account> [resume|fresh] | mode [auto|ask]]',
    })

    return next(e)
  })

  on('turn.complete', async ($, e, next) => {
    void refresh($)

    return next(e)
  })

  on('command.run', { command: 'switchyard' }, async ($, e) => {
    const command = parseCommand(e.args)
    switch (command.kind) {
      case 'open':
        await open($, command.page)

        return { text: command.page === 'style' ? 'Style page opened.' : 'switchyard pane opened.' }
      case 'switch':
        return { text: await handoff($, command.name, command.flag) }
      case 'mode': {
        if (command.mode !== null) {
          return { text: await setSetting($, 'mode', command.mode) }
        }
        await refresh($)
        const current = await read($, snapshot)

        return { text: current?.kind === 'ok' && current.settings ? describeSettings(current.settings) : 'switchyard settings are not available.' }
      }
      default:
        return { text: USAGE }
    }
  })

  on('ui.render', { component: 'Pane', requestId: PANE }, async ($, e) => {
    const elements = $.ui.resolve(e)
    const { Box, Button, Text } = elements
    // Mobile has no text fields or pickers; there the colors are set with the palettes or the command.
    const fields = 'Input' in elements && 'Select' in elements ? { Input: elements.Input, Select: elements.Select } : null
    const current = await read($, snapshot)
    const shown = await read($, page)
    const now = await $.clock.now()
    const hint = problem(current)
    const settings = current?.kind === 'ok' ? current.settings : null
    const colors = palette(settings?.colors)
    // A background has to fill the whole pane, not just the height of its content.
    const fill = colors.background
      ? { backgroundColor: colors.background, width: e.props.bodyColumns, minHeight: e.props.scroll.bodyRows }
      : {}
    const tint = (percent: number): string => ({ ok: colors.low, warn: colors.medium, high: colors.high })[usageLevel(percent)]
    const dim = (text: string) => <Text color={colors.text} dimColor>{text}</Text>

    const toPage = (key: string, label: string, to: Page) => (
      <Button key={key} label={label} onPress={() => update($, page, () => to)} />
    )
    const refreshButton = <Button key="refresh" label="Refresh" onPress={() => refresh($)} />
    // The engine's own close mark is small; this one is a full button.
    const closeButton = <Button key="close" label="Close" role="dismiss" onPress={() => $.ui.close({ id: PANE })} />

    if (current?.kind !== 'ok' || hint) {
      return (
        <Box flexDirection="column" {...fill}>
          {dim(hint ?? '')}
          {refreshButton}
          {closeButton}
        </Box>
      )
    }

    const bar = (label: string, percent: number, extra: string) => (
      <Box key={label}>
        {dim(`${label} `)}
        <Text color={tint(percent)}>{usageBar(percent)}</Text>
        <Text color={colors.text}> {String(Math.round(percent)).padStart(3)}%</Text>
        {dim(` ${extra}`)}
      </Box>
    )
    const usageRow = (label: string, usage: Usage | null) =>
      usage ? bar(label, usage.used_percent, resetLabel(usage.resets_at, now)) : <Box key={label}>{dim(`${label} no data yet`)}</Box>

    if (shown === 'style') {
      const choice = (name: string) => (
        <Button
          key={`colors-${name}`}
          label={name === 'default' ? 'theme' : name}
          onPress={async () => $.ui.toast(await setPalette($, name))}
        />
      )
      const colorOptions = (value: string) => [
        { value: 'theme', label: 'theme' },
        ...NAMED_COLORS.map(name => ({ value: name, label: name })),
        // A color typed in, such as a hex value, stays selectable.
        ...(value !== '' && !(NAMED_COLORS as readonly string[]).includes(value) ? [{ value, label: value }] : []),
      ]
      const pick = async (key: string, value: string) => $.ui.toast(await setSetting($, key, value === 'theme' ? '' : value))
      const custom = async (text: string) => {
        const parsed = parseCustomColor(text)
        $.ui.toast(
          parsed
            ? await setSetting($, parsed.key, parsed.value)
            : `Type a color name or hex after the slot: ${COLOR_SLOTS.map(s => s.id).join(', ')}`,
        )
      }

      return (
        <Box flexDirection="column" gap={1} {...fill}>
          <Box gap={1}>
            <Text bold color={colors.text}>Style</Text>
            {toPage('back', 'Back', 'main')}
            {closeButton}
          </Box>
          <Box flexDirection="column" borderStyle="round" borderColor={colors.border} paddingX={1}>
            <Text bold color={colors.text}>Palette</Text>
            <Box gap={1}>{PALETTES.map(choice)}</Box>
            {dim('Only this pane changes, never your Claude theme or other mods.')}
          </Box>
          <Box flexDirection="column" borderStyle="round" borderColor={colors.border} paddingX={1}>
            <Text bold color={colors.text}>Preview</Text>
            {bar('below 70%', 30, '')}
            {bar('70-89%  ', 75, '')}
            {bar('from 90% ', 95, '')}
          </Box>
          {settings && fields && (
            <Box flexDirection="column" borderStyle="round" borderColor={colors.border} paddingX={1}>
              <Text bold color={colors.text}>Colors</Text>
              {COLOR_SLOTS.map(slot => {
                const value = slot.get(settings.colors)

                return (
                  <fields.Select
                    key={`color-${slot.id}`}
                    label={`${slot.label.padEnd(14)} `}
                    options={colorOptions(value)}
                    value={value === '' ? 'theme' : value}
                    onSelect={(v: string) => pick(slot.key, v)}
                  />
                )
              })}
              <fields.Input
                key="custom-color"
                label="Custom "
                placeholder="slot color, e.g. background #1e1e1e"
                submitLabel="apply"
                onSubmit={custom}
              />
              {dim(`slots: ${COLOR_SLOTS.map(s => s.id).join(', ')}; color: a name, hex or theme`)}
            </Box>
          )}
          {settings && !fields && dim("Set a color with: switchyard config set color_background '#1e1e1e'")}
        </Box>
      )
    }

    const choice = (key: string, label: string, isCurrent: boolean, setting: string, value: string) => (
      <Button
        key={key}
        label={isCurrent ? `✓ ${label}` : label}
        variant={isCurrent ? 'primary' : undefined}
        onPress={async () => $.ui.toast(await setSetting($, setting, value))}
      />
    )

    return (
      <Box flexDirection="column" gap={1} {...fill}>
        <Box gap={1}>
          <Text bold color={colors.text}>switchyard</Text>
          {toPage('to-style', 'Style', 'style')}
          {refreshButton}
          {closeButton}
        </Box>
        {settings && (
          <Box flexDirection="column" borderStyle="round" borderColor={colors.border} paddingX={1}>
            <Text bold color={colors.text}>Failover</Text>
            <Box gap={1}>
              {dim('At a limit')}
              {choice('mode-auto', 'switch automatically', settings.mode === 'auto', 'mode', 'auto')}
              {choice('mode-ask', 'ask me', settings.mode === 'ask', 'mode', 'ask')}
            </Box>
            <Box gap={1}>
              {dim('Conversation')}
              {choice('carry-on', 'take it along', settings.carry_context, 'carry_context', 'true')}
              {choice('carry-off', 'start new', !settings.carry_context, 'carry_context', 'false')}
            </Box>
          </Box>
        )}
        {current.profiles.length === 0 && dim('No accounts yet. Create one with: switchyard add <name>')}
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
              {p.active && dim(' active')}
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
        {dim('A switch restarts claude (needs switchyard run).')}
      </Box>
    )
  })
}
