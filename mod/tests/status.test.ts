import { expect, test } from 'claude-code/testing'

import {
  describeSettings,
  parseConfig,
  parseStatus,
  parseCommand,
  parseColorEntry,
  problem,
  resetLabel,
  usageBar,
  usageLevel,
} from '../hooks/status'

const NO_COLORS = { background: '', text: '', low: '', medium: '', high: '', border_active: '', border: '' }

const OK = JSON.stringify({
  schema: 1,
  version: '0.1.0',
  active: 'main',
  profiles: [
    {
      name: 'main',
      active: true,
      cooldown_until: null,
      five_hour: { used_percent: 20.4, resets_at: '2026-10-09T15:00:00Z', updated_at: '2026-10-09T11:00:00Z' },
      seven_day: { used_percent: 18, resets_at: '2026-10-13T12:00:00Z', updated_at: '2026-10-09T11:00:00Z' },
    },
    { name: 'zweit', active: false, cooldown_until: '2026-10-09T15:00:00Z', five_hour: null, seven_day: null },
  ],
})

test('reads the status of the supported schema', async () => {
  const snapshot = parseStatus(OK)
  expect(snapshot.kind).toBe('ok')
  if (snapshot.kind === 'ok') {
    expect(snapshot.active).toBe('main')
    expect(snapshot.profiles.length).toBe(2)
    expect(snapshot.profiles[0]?.five_hour?.used_percent).toBe(20.4)
    expect(snapshot.profiles[1]?.five_hour).toBe(null)
  }
})

test('a schema it does not read is incompatible, not an error', async () => {
  expect(parseStatus(JSON.stringify({ schema: 2, profiles: [] }))).toEqual({ kind: 'incompatible', schema: 2 })
  expect(parseStatus(JSON.stringify({ profiles: [] }))).toEqual({ kind: 'incompatible', schema: null })
})

test('output that is not a status report is unavailable', async () => {
  for (const text of ['', 'not json', '[]', '"x"']) {
    expect(parseStatus(text).kind).toBe('unavailable')
  }
})

test('malformed profiles are dropped, the rest is kept', async () => {
  const snapshot = parseStatus(JSON.stringify({ schema: 1, active: 'a', profiles: [{ name: 'a' }, 5, { active: true }] }))
  expect(snapshot.kind).toBe('ok')
  if (snapshot.kind === 'ok') {
    expect(snapshot.profiles.map(p => p.name)).toEqual(['a'])
  }
})

test('problems are described for the pane', async () => {
  expect(problem(null)).toMatch(/Reading/)
  expect(problem({ kind: 'unavailable', reason: 'command not found' })).toMatch(/command not found/)
  expect(problem({ kind: 'incompatible', schema: 9 })).toMatch(/schema 9; this mod reads schema 1/)
  expect(problem(parseStatus(OK))).toBeUndefined()
})

test('parses the arguments of /switchyard', async () => {
  expect(parseCommand('')).toEqual({ kind: 'open', page: 'main' })
  expect(parseCommand('  style ')).toEqual({ kind: 'open', page: 'style' })
  expect(parseCommand('switch zweit')).toEqual({ kind: 'switch', name: 'zweit' })
  expect(parseCommand('switch  zweit  fresh ')).toEqual({ kind: 'switch', name: 'zweit', flag: '--fresh' })
  expect(parseCommand('SWITCH zweit --resume')).toEqual({ kind: 'switch', name: 'zweit', flag: '--resume' })
  expect(parseCommand('mode')).toEqual({ kind: 'mode', mode: null })
  expect(parseCommand('mode AUTO')).toEqual({ kind: 'mode', mode: 'auto' })
  for (const bad of ['switch', 'switch a b c', 'switch zweit later', 'mode never', 'mode auto ask', 'style now', 'accounts', 'x']) {
    expect(parseCommand(bad)).toEqual({ kind: 'help' })
  }
})

test('usage levels turn from calm to high', async () => {
  expect([0, 69, 70, 89, 90, 100].map(usageLevel)).toEqual(['ok', 'ok', 'warn', 'warn', 'high', 'high'])
})

test('the usage bar is filled in proportion and never overflows', async () => {
  expect(usageBar(0)).toBe('░░░░░░░░░░░░')
  expect(usageBar(50)).toBe('██████░░░░░░')
  expect(usageBar(100)).toBe('████████████')
  expect(usageBar(250)).toBe('████████████')
  expect(usageBar(-5)).toBe('░░░░░░░░░░░░')
  expect(usageBar(50, 4)).toBe('██░░')
})

test('a reset is named by time today and by weekday and time later', async () => {
  const now = new Date(2026, 9, 9, 12, 0).getTime()
  expect(resetLabel(new Date(2026, 9, 9, 15, 30).toISOString(), now)).toBe('resets 15:30')
  expect(resetLabel(new Date(2026, 9, 12, 8, 5).toISOString(), now)).toBe('resets Mon 08:05')
  expect(resetLabel(new Date(2026, 9, 9, 9, 0).toISOString(), now)).toBe('')
  expect(resetLabel('', now)).toBe('')
})

test('reads the failover settings of the supported schema', async () => {
  const config = (extra: object) => JSON.stringify({ schema: 1, mode: 'auto', carry_context: false, strategy: 'sequential', proactive_threshold: 0, ...extra })
  expect(parseConfig(config({}))).toEqual({ mode: 'auto', carry_context: false, colors: { background: '', text: '', low: '', medium: '', high: '', border_active: '', border: '' } })
  expect(parseConfig(config({ colors: { background: '#111', high: 5 } }))?.colors).toEqual({ background: '#111', text: '', low: '', medium: '', high: '', border_active: '', border: '' })
  expect(parseConfig(config({ schema: 2 }))).toBeNull()
  expect(parseConfig(config({ mode: 'sometimes' }))).toBeNull()
  expect(parseConfig(config({ carry_context: 'yes' }))).toBeNull()
  expect(parseConfig('not json')).toBeNull()
})

test('the settings are described in a sentence', async () => {
  expect(describeSettings({ mode: 'auto', carry_context: false, colors: NO_COLORS })).toBe('At a limit: auto (switches without asking). Conversation: started new.')
  expect(describeSettings({ mode: 'ask', carry_context: true, colors: NO_COLORS })).toBe('At a limit: ask (asks in the terminal). Conversation: taken along.')
})

test('a typed color is for the chosen slot unless it names its own', async () => {
  expect(parseColorEntry('#00ff88', 'color_border_active')).toEqual({ key: 'color_border_active', value: '#00ff88' })
  expect(parseColorEntry(' 1E1E1E ', 'color_background')).toEqual({ key: 'color_background', value: '#1E1E1E' })
  expect(parseColorEntry('magenta', 'color_text')).toEqual({ key: 'color_text', value: 'magenta' })
  expect(parseColorEntry('background #1e1e1e', 'color_text')).toEqual({ key: 'color_background', value: '#1e1e1e' })
  expect(parseColorEntry('  ACTIVE   magenta ', 'color_text')).toEqual({ key: 'color_border_active', value: 'magenta' })
})

test('default and theme clear a color', async () => {
  expect(parseColorEntry('default', 'color_high')).toEqual({ key: 'color_high', value: '' })
  expect(parseColorEntry('high theme', 'color_text')).toEqual({ key: 'color_high', value: '' })
})

test('entries that are neither a color nor a slot with a color are refused', async () => {
  for (const bad of ['', '   ', 'sky blue', 'nope red', 'background red extra']) {
    expect(parseColorEntry(bad, 'color_text')).toBeNull()
  }
})
