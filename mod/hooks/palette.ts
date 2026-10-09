import type { PluginOptions } from 'claude-code'

/** The colors of the accounts pane. Unset ones fall back to the person's theme. */
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

/** The theme colors used where an option is empty, so the pane follows the theme. */
const THEME = { low: 'success', medium: 'warning', high: 'error', borderActive: 'success', border: 'subtle' } as const

/** A color option: a theme key, a color name or hex; empty or not a string is unset. */
function color(options: PluginOptions, key: string): string | undefined {
  const value = options[key]
  const text = typeof value === 'string' ? value.trim() : ''

  return text === '' ? undefined : text
}

/** Reads the color options of the manifest's `userConfig`. */
export function palette(options: PluginOptions): Palette {
  return {
    background: color(options, 'background'),
    text: color(options, 'text'),
    low: color(options, 'usageLow') ?? THEME.low,
    medium: color(options, 'usageMedium') ?? THEME.medium,
    high: color(options, 'usageHigh') ?? THEME.high,
    borderActive: color(options, 'borderActive') ?? THEME.borderActive,
    border: color(options, 'border') ?? THEME.border,
  }
}
