import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { Page, Pending, Settings, Snapshot, Usage } from '../types'
import { DEFAULT_COLOR, describeColor, palette } from './palette'
import {
  COLOR_SLOTS,
  describeSettings,
  isCoolingDown,
  PALETTES,
  parseCommand,
  NAMED_COLORS,
  parseConfig,
  parseDecision,
  parseColorEntry,
  parseStatus,
  problem,
  resetLabel,
  secondsLeft,
  timeOfDay,
  usageBar,
  usageLevel,
  USAGE,
} from './status'

const PANE = 'switchyard'

const snapshot = atom({ plugin: 'switchyard-mod', key: 'snapshot' } as const, null)
const page = atom({ plugin: 'switchyard-mod', key: 'page' } as const, 'main')
const slotAtom = atom({ plugin: 'switchyard-mod', key: 'slot' } as const, 'background')
const decisionAtom = atom({ plugin: 'switchyard-mod', key: 'decision' } as const, null)

type Engine = EngineInterface

/** Runs `switchyard` with `argv`; the text of a refusal when it fails, undefined when it worked. */
async function run($: Engine, argv: string[], timeoutMs = 10_000): Promise<{ stdout: string } | { error: string }> {
  try {
    const result = await $.process.run(['switchyard', ...argv], { timeoutMs })

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

/**
 * Reads the limit that waits for an answer. Every call also tells the launcher that this mod is
 * present, which is what makes it keep claude running at a limit and wait for the buttons.
 */
async function pollDecision($: Engine): Promise<Pending | null> {
  const result = await run($, ['decision', '--json'], 5_000)
  const pending = 'stdout' in result ? parseDecision(result.stdout) : null
  await update($, decisionAtom, () => pending)

  return pending
}

let announced = ''

/** One round of polling: a new question opens the pane and says so. */
async function watchDecision($: Engine): Promise<void> {
  const pending = await pollDecision($)
  if (pending && pending.expires_at !== announced) {
    announced = pending.expires_at
    $.ui.toast(`${pending.profile} reached its limit. Choose in the switchyard pane.`)
    await $.ui.open({ id: PANE, title: 'switchyard' })
  }
}

/** The release the user was told about, so a session mentions each one once. */
let announcedUpdate = ''

async function refresh($: Engine): Promise<void> {
  const current = await load($)
  await update($, snapshot, () => current)
  await pollDecision($)
  const pending = current.kind === 'ok' ? current.update : null
  if (pending && pending.version !== announcedUpdate) {
    announcedUpdate = pending.version
    $.ui.toast(`switchyard ${pending.version} is available. Run /switchyard update, or press Update now in the pane.`)
  }
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

/** Answers the limit that waits for a decision; the text to show. */
async function decide($: Engine, argv: string[], done: string): Promise<string> {
  const result = await run($, ['decision', 'answer', ...argv])
  await pollDecision($)

  return 'error' in result ? result.error : done
}

/** Asks the launcher to continue in `name`; the answer is the text to show. */
async function handoff($: Engine, name: string, flag?: '--resume' | '--fresh'): Promise<string> {
  const result = await run($, ['handoff', name, '--session', await $.session.id(), ...(flag ? [flag] : [])])
  if ('error' in result) {
    return result.error
  }

  return `Handoff to ${name} requested. If claude was started with switchyard run, it restarts in ${name}.`
}

/**
 * Replaces the switchyard binary with the latest release. It runs in the background:
 * claude and this session stay open, and the hooks use the new binary from the next call.
 */
async function installUpdate($: Engine, version: string): Promise<string> {
  $.ui.toast(`Updating switchyard to ${version} ...`)
  const result = await run($, ['update', '--install'], 120_000)
  if ('error' in result) {
    return result.error
  }
  await refresh($)

  return `switchyard ${version} installed. This session keeps running; restart switchyard run to use it for the launcher too.`
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
      argumentHint: '[style | switch <account> [resume|fresh] | mode [auto|ask] | update]',
    })

    // Polling is what tells the launcher a mod is here; it also picks up a limit that waits for the buttons.
    $.clock.every(2_000, () => {
      void watchDecision($)
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
      case 'update': {
        await refresh($)
        const current = await read($, snapshot)
        const pending = current?.kind === 'ok' ? current.update : null

        return { text: pending ? await installUpdate($, pending.version) : 'switchyard is up to date.' }
      }
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
    // Mobile has no text fields; there any other color is set with the command.
    const Input = 'Input' in elements ? elements.Input : null
    const current = await read($, snapshot)
    const shown = await read($, page)
    const question = await read($, decisionAtom)
    const chosenId = await read($, slotAtom)
    const chosen = COLOR_SLOTS.find(s => s.id === chosenId) ?? COLOR_SLOTS[0]
    const now = await $.clock.now()
    const hint = problem(current)
    const settings = current?.kind === 'ok' ? current.settings : null
    const chosenValue = settings ? chosen.get(settings.colors) : ''
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

    const pending = current.update

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
          label={name}
          onPress={async () => $.ui.toast(await setPalette($, name))}
        />
      )
      const pick = async (key: string, value: string) => $.ui.toast(await setSetting($, key, value === 'default' ? '' : value))
      const custom = async (text: string) => {
        const parsed = parseColorEntry(text, chosen.key)
        $.ui.toast(
          parsed
            ? await setSetting($, parsed.key, parsed.value)
            : 'Type a hex code such as #00ff88 or a color name, or "<slot> <color>".',
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
          {settings && (
            <Box flexDirection="column" borderStyle="round" borderColor={colors.border} paddingX={1}>
              <Text bold color={colors.text}>Colors</Text>
              {dim('Pick a color to change, then a new value:')}
              <Box gap={1} flexWrap="wrap">
                {COLOR_SLOTS.map(slot => (
                  <Button
                    key={`slot-${slot.id}`}
                    label={slot.label}
                    variant={slot.id === chosen.id ? 'primary' : undefined}
                    onPress={() => update($, slotAtom, () => slot.id)}
                  />
                ))}
              </Box>
              <Box flexDirection="column">
                {COLOR_SLOTS.map(slot => {
                  const value = slot.get(settings.colors)
                  const isChosen = slot.id === chosen.id

                  return (
                    <Box key={`value-${slot.id}`} gap={1}>
                      <Text color={value.trim() || DEFAULT_COLOR[slot.id]}>■</Text>
                      <Text bold={isChosen} color={colors.text}>{`${isChosen ? '▸' : ' '} ${slot.label.padEnd(13)}`}</Text>
                      {dim(describeColor(value, slot.id))}
                    </Box>
                  )
                })}
              </Box>
              <Box gap={1} flexWrap="wrap">
                {['default', ...NAMED_COLORS].map(name => {
                  const isCurrent = name === 'default' ? chosenValue === '' : chosenValue.toLowerCase() === name

                  return (
                    <Button key={`pick-${name}`} variant={isCurrent ? 'primary' : undefined} onPress={() => pick(chosen.key, name)}>
                      {isCurrent ? '✓ ' : ''}
                      <Text color={name === 'default' ? DEFAULT_COLOR[chosen.id] : name}>■</Text> {name}
                    </Button>
                  )
                })}
              </Box>
              {Input && (
                <Input
                  key="custom-color"
                  label={`Hex for ${chosen.label} `}
                  placeholder="#rrggbb, a color name, or default"
                  submitLabel="apply"
                  onSubmit={custom}
                />
              )}
              {dim(Input ? `Applies to "${chosen.label}". Other slot: "<slot> <color>" with ${COLOR_SLOTS.map(s => s.id).join(', ')}.` : "Any other color: switchyard config set color_background '#1e1e1e'")}
            </Box>
          )}
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
        <Box gap={1} flexWrap="wrap">
          <Text bold color={colors.text}>switchyard</Text>
          {toPage('to-style', 'Style', 'style')}
          {refreshButton}
          {closeButton}
        </Box>
        {question && (
          <Box flexDirection="column" borderStyle="round" borderColor={colors.high} paddingX={1}>
            <Text bold color={colors.high}>{question.profile} reached its limit</Text>
            {dim(`claude keeps running while you choose (${secondsLeft(question.expires_at, now)} s left, then the terminal asks).`)}
            {question.options.map(name => (
              <Box key={name} gap={1} flexWrap="wrap">
                <Button
                  key={`decide-${name}`}
                  label={`Continue in ${name}`}
                  variant="primary"
                  onPress={async () => $.ui.toast(await decide($, ['switch', name, '--resume'], `Continuing in ${name} ...`))}
                />
                <Button
                  key={`decide-fresh-${name}`}
                  label={`New conversation in ${name}`}
                  onPress={async () => $.ui.toast(await decide($, ['switch', name, '--fresh'], `Starting fresh in ${name} ...`))}
                />
              </Box>
            ))}
            <Box gap={1}>
              <Button key="decide-stay" label="Stay here" onPress={async () => $.ui.toast(await decide($, ['stay'], 'Staying in this account.'))} />
            </Box>
          </Box>
        )}
        {pending && (
          <Box flexDirection="column" borderStyle="round" borderColor={colors.medium} paddingX={1}>
            <Text bold color={colors.medium}>Update available: switchyard {pending.version}</Text>
            {dim('Installs in the background. This session keeps running.')}
            <Box gap={1}>
              <Button
                key="update-install"
                label="Update now"
                variant="primary"
                onPress={async () => $.ui.toast(await installUpdate($, pending.version))}
              />
            </Box>
          </Box>
        )}
        {settings && (
          <Box flexDirection="column" borderStyle="round" borderColor={colors.border} paddingX={1}>
            <Text bold color={colors.text}>Failover</Text>
            {dim('At a limit')}
            <Box gap={1} flexWrap="wrap">
              {choice('mode-auto', 'switch automatically', settings.mode === 'auto', 'mode', 'auto')}
              {choice('mode-ask', 'ask me', settings.mode === 'ask', 'mode', 'ask')}
            </Box>
            {dim('Conversation')}
            <Box gap={1} flexWrap="wrap">
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
              <Box gap={1} marginTop={1} flexWrap="wrap">
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
