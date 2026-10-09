import type { Colors } from '../types'

/** The colors of the accounts pane, with the theme's colors where nothing is set. */
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

/** The theme colors used where a setting is empty, so the pane follows the theme. */
const THEME = { low: 'success', medium: 'warning', high: 'error', borderActive: 'success', border: 'subtle' } as const

const unset = (value: string | undefined): string | undefined => (value === undefined || value.trim() === '' ? undefined : value.trim())

/** Turns the colors of the switchyard config into the palette the pane draws with. */
export function palette(colors: Colors | null | undefined): Palette {
  return {
    background: unset(colors?.background),
    text: unset(colors?.text),
    low: unset(colors?.low) ?? THEME.low,
    medium: unset(colors?.medium) ?? THEME.medium,
    high: unset(colors?.high) ?? THEME.high,
    borderActive: unset(colors?.border_active) ?? THEME.borderActive,
    border: unset(colors?.border) ?? THEME.border,
  }
}
