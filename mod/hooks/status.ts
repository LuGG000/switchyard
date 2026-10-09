import type { Colors, Page, ProfileStatus, Settings, Snapshot, UpdateInfo } from '../types'

/** The `status --json` schema this mod reads. */
export const SUPPORTED_SCHEMA = 1

type Raw = Record<string, unknown>

const isRecord = (value: unknown): value is Raw =>
  typeof value === 'object' && value !== null && !Array.isArray(value)

function usage(value: unknown): ProfileStatus['five_hour'] {
  if (!isRecord(value) || typeof value.used_percent !== 'number') {
    return null
  }

  return {
    used_percent: value.used_percent,
    resets_at: typeof value.resets_at === 'string' ? value.resets_at : '',
    updated_at: typeof value.updated_at === 'string' ? value.updated_at : '',
  }
}

function profile(value: unknown): ProfileStatus | null {
  if (!isRecord(value) || typeof value.name !== 'string') {
    return null
  }

  return {
    name: value.name,
    active: value.active === true,
    cooldown_until: typeof value.cooldown_until === 'string' ? value.cooldown_until : null,
    five_hour: usage(value.five_hour),
    seven_day: usage(value.seven_day),
  }
}

/** Reads the output of `switchyard status --json`; anything else is a snapshot that says why. */
export function parseStatus(stdout: string): Snapshot {
  let report: unknown
  try {
    report = JSON.parse(stdout)
  } catch {
    return { kind: 'unavailable', reason: 'switchyard status did not print JSON' }
  }
  if (!isRecord(report)) {
    return { kind: 'unavailable', reason: 'switchyard status did not print an object' }
  }
  if (report.schema !== SUPPORTED_SCHEMA) {
    return { kind: 'incompatible', schema: typeof report.schema === 'number' ? report.schema : null }
  }
  const profiles = Array.isArray(report.profiles)
    ? report.profiles.map(profile).filter((p): p is ProfileStatus => p !== null)
    : []

  return {
    kind: 'ok',
    active: typeof report.active === 'string' ? report.active : '',
    profiles,
    settings: null,
    update: parseUpdate(report.update),
  }
}

/** Whether a cooldown still holds at `now` (milliseconds since the epoch). */
export function isCoolingDown(p: ProfileStatus, now: number): boolean {
  return p.cooldown_until !== null && Date.parse(p.cooldown_until) > now
}

function clock(iso: string): string {
  const date = new Date(iso)
  const pad = (n: number) => String(n).padStart(2, '0')

  return `${pad(date.getHours())}:${pad(date.getMinutes())}`
}

/** The one line the pane shows when switchyard cannot be read. */
export function problem(snapshot: Snapshot | null): string | undefined {
  switch (snapshot?.kind) {
    case undefined:
      return 'Reading switchyard ...'
    case 'unavailable':
      return `switchyard is not available (${snapshot.reason}). Install it and put it in PATH.`
    case 'incompatible':
      return `switchyard reports status schema ${snapshot.schema ?? 'unknown'}; this mod reads schema ${SUPPORTED_SCHEMA}. Update switchyard and the mod.`
    default:
      return undefined
  }
}

export type Level = 'ok' | 'warn' | 'high'

/** How full a window is: calm below 70%, a warning below 90%, then high. */
export function usageLevel(percent: number): Level {
  if (percent >= 90) {
    return 'high'
  }

  return percent >= 70 ? 'warn' : 'ok'
}

/** A bar of `width` cells filled up to `percent`. */
export function usageBar(percent: number, width = 12): string {
  const filled = Math.min(width, Math.max(0, Math.round((percent / 100) * width)))

  return '█'.repeat(filled) + '░'.repeat(width - filled)
}

const DAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

/** When a window resets: the time today, else the weekday and time. Empty without a reset. */
export function resetLabel(iso: string, now: number): string {
  const at = Date.parse(iso)
  if (Number.isNaN(at) || at <= now) {
    return ''
  }
  const date = new Date(at)
  const time = clock(iso)

  return date.toDateString() === new Date(now).toDateString()
    ? `resets ${time}`
    : `resets ${DAYS[date.getDay()]} ${time}`
}

/** The local time of day of an ISO timestamp. */
export function timeOfDay(iso: string): string {
  return clock(iso)
}

/** The `config --json` schema this mod reads. */
export const SUPPORTED_CONFIG_SCHEMA = 1

/** Reads the output of `switchyard config --json`; null when it is not something the mod reads. */
export function parseConfig(stdout: string): Settings | null {
  let report: unknown
  try {
    report = JSON.parse(stdout)
  } catch {
    return null
  }
  if (!isRecord(report) || report.schema !== SUPPORTED_CONFIG_SCHEMA) {
    return null
  }
  if ((report.mode !== 'auto' && report.mode !== 'ask') || typeof report.carry_context !== 'boolean') {
    return null
  }

  return { mode: report.mode, carry_context: report.carry_context, colors: parseColors(report.colors) }
}

/** The settings in a sentence, for `/failover`. */
export function describeSettings(s: Settings): string {
  const mode = s.mode === 'auto' ? 'auto (switches without asking)' : 'ask (asks in the terminal)'

  return `At a limit: ${mode}. Conversation: ${s.carry_context ? 'taken along' : 'started new'}.`
}

/** The colors of the config; a missing or malformed entry is empty, which means the theme's color. */
function parseColors(value: unknown): Colors {
  const text = (key: string): string => (isRecord(value) && typeof value[key] === 'string' ? value[key] : '')

  return {
    background: text('background'),
    text: text('text'),
    low: text('low'),
    medium: text('medium'),
    high: text('high'),
    border_active: text('border_active'),
    border: text('border'),
  }
}

/** The palettes `switchyard config colors` knows, in display order. */
export const PALETTES = ['default', 'dark', 'light'] as const

/** What `/switchyard` was asked to do. */
export type Command =
  | { kind: 'open'; page: Page }
  | { kind: 'switch'; name: string; flag?: '--resume' | '--fresh' }
  /** `mode: null` shows the current setting. */
  | { kind: 'mode'; mode: 'auto' | 'ask' | null }
  | { kind: 'update' }
  | { kind: 'help' }

export const USAGE = [
  'Usage: /switchyard [style]',
  '       /switchyard switch <account> [resume|fresh]',
  '       /switchyard mode [auto|ask]',
  '       /switchyard update',
].join('\n')

/** Reads the arguments of `/switchyard`; anything it does not know is help. */
export function parseCommand(args: string): Command {
  const [word, ...rest] = args.trim().split(/\s+/).filter(Boolean)
  const arg = rest[0]?.toLowerCase()
  switch (word?.toLowerCase()) {
    case undefined:
      return { kind: 'open', page: 'main' }
    case 'style':
      return rest.length === 0 ? { kind: 'open', page: 'style' } : { kind: 'help' }
    case 'update':
      return rest.length === 0 ? { kind: 'update' } : { kind: 'help' }
    case 'mode':
      if (rest.length === 0) {
        return { kind: 'mode', mode: null }
      }

      return rest.length === 1 && (arg === 'auto' || arg === 'ask') ? { kind: 'mode', mode: arg } : { kind: 'help' }
    case 'switch': {
      const name = rest[0]
      if (!name || rest.length > 2) {
        return { kind: 'help' }
      }
      const option = rest[1]?.toLowerCase().replace(/^--/, '')
      if (option === undefined) {
        return { kind: 'switch', name }
      }

      return option === 'fresh' || option === 'resume'
        ? { kind: 'switch', name, flag: option === 'fresh' ? '--fresh' : '--resume' }
        : { kind: 'help' }
    }
    default:
      return { kind: 'help' }
  }
}

/** The colors of the pane that can be set, with the switchyard setting behind each. */
export const COLOR_SLOTS = [
  { id: 'background', label: 'background', key: 'color_background', get: (c: Colors) => c.background },
  { id: 'text', label: 'text', key: 'color_text', get: (c: Colors) => c.text },
  { id: 'low', label: 'usage low', key: 'color_low', get: (c: Colors) => c.low },
  { id: 'medium', label: 'usage medium', key: 'color_medium', get: (c: Colors) => c.medium },
  { id: 'high', label: 'usage high', key: 'color_high', get: (c: Colors) => c.high },
  { id: 'active', label: 'active border', key: 'color_border_active', get: (c: Colors) => c.border_active },
  { id: 'border', label: 'other borders', key: 'color_border', get: (c: Colors) => c.border },
] as const

/** The color names offered in the pickers; any other color is typed in. */
export const NAMED_COLORS = ['white', 'black', 'gray', 'red', 'green', 'yellow', 'blue', 'magenta', 'cyan'] as const

/** Words that clear a color, so the default applies. */
const CLEARS = ['default', 'theme']

/**
 * Reads what was typed into the color field. A single color (`#00ff88`, `red`)
 * is for the chosen slot, `<slot> <color>` names its slot; a bare six-digit hex
 * gets its `#`. Null when it is neither.
 */
export function parseColorEntry(text: string, chosen: (typeof COLOR_SLOTS)[number]['key']): { key: string; value: string } | null {
  const words = text.trim().split(/\s+/).filter(Boolean)
  const slot = words.length === 2 ? COLOR_SLOTS.find(s => s.id === words[0]?.toLowerCase()) : undefined
  const color = words.length === 2 ? (slot ? words[1] : undefined) : words.length === 1 ? words[0] : undefined
  if (!color) {
    return null
  }
  const value = CLEARS.includes(color.toLowerCase()) ? '' : /^[0-9a-f]{6}$/i.test(color) ? `#${color}` : color

  return { key: slot?.key ?? chosen, value }
}

/** A newer release from `status --json`; null when there is none or the field is odd. */
function parseUpdate(value: unknown): UpdateInfo | null {
  return isRecord(value) && typeof value.version === 'string' && typeof value.url === 'string'
    ? { version: value.version, url: value.url }
    : null
}
