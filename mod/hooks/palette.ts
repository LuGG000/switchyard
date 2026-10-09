import type { ColorSlot, Colors } from '../types'

/** The colors of the accounts pane, with the defaults where nothing is set. */
export type Palette = {
  /** Background of the whole pane; undefined keeps the engine's. */
  background: string | undefined
  /** Color of the text; undefined keeps the theme's. */
  text: string | undefined
  /** Usage bar colors by level. */
  low: string
  medium: string
  high: string
  borderActive: string
  border: string
}

/**
 * What an empty setting draws. Theme keys follow the person's Claude theme; the
 * active account is plain green. No entry means the engine's own background or
 * the theme's text color.
 */
export const DEFAULT_COLOR: Record<ColorSlot, string | undefined> = {
  background: undefined,
  text: undefined,
  low: 'success',
  medium: 'warning',
  high: 'error',
  active: 'green',
  border: 'subtle',
}

const unset = (value: string | undefined): string | undefined => (value === undefined || value.trim() === '' ? undefined : value.trim())

/** Turns the colors of the switchyard config into the palette the pane draws with. */
export function palette(colors: Colors | null | undefined): Palette {
  return {
    background: unset(colors?.background),
    text: unset(colors?.text),
    low: unset(colors?.low) ?? DEFAULT_COLOR.low ?? '',
    medium: unset(colors?.medium) ?? DEFAULT_COLOR.medium ?? '',
    high: unset(colors?.high) ?? DEFAULT_COLOR.high ?? '',
    borderActive: unset(colors?.border_active) ?? DEFAULT_COLOR.active ?? '',
    border: unset(colors?.border) ?? DEFAULT_COLOR.border ?? '',
  }
}
