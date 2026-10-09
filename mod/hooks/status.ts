import type { ProfileStatus, Snapshot } from '../types'

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

  return { kind: 'ok', active: typeof report.active === 'string' ? report.active : '', profiles }
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

/** Splits `/switch` arguments into the profile and the carry choice. */
export function parseSwitchArgs(args: string): { name: string; flag?: '--resume' | '--fresh' } | null {
  const [name, option, ...rest] = args.trim().split(/\s+/)
  if (!name || rest.length > 0) {
    return null
  }
  if (option === undefined) {
    return { name }
  }
  if (option === 'fresh' || option === '--fresh') {
    return { name, flag: '--fresh' }
  }
  if (option === 'resume' || option === '--resume') {
    return { name, flag: '--resume' }
  }

  return null
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
