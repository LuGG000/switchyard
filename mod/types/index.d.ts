export type Usage = {
  used_percent: number
  resets_at: string
  updated_at: string
}

export type ProfileStatus = {
  name: string
  active: boolean
  cooldown_until: string | null
  five_hour: Usage | null
  seven_day: Usage | null
}

/** The pane colors from `switchyard config --json`; empty means the Claude theme's color. */
export type Colors = {
  background: string
  text: string
  low: string
  medium: string
  high: string
  border_active: string
  border: string
}

/** The failover settings, from `switchyard config --json`. */
export type Settings = {
  mode: 'auto' | 'ask'
  carry_context: boolean
  colors: Colors
}

/** What the mod knows about switchyard, from `switchyard status --json` and `config --json`. */
export type Snapshot =
  | { kind: 'ok'; active: string; profiles: ProfileStatus[]; settings: Settings | null }
  /** The command is missing from PATH or failed. */
  | { kind: 'unavailable'; reason: string }
  /** The command reports a schema this mod does not read. */
  | { kind: 'incompatible'; schema: number | null }

/** The color the style page edits. */
export type ColorSlot = 'background' | 'text' | 'low' | 'medium' | 'high' | 'active' | 'border'

/** The pages of the /switchyard pane. */
export type Page = 'main' | 'style'

declare module 'claude-code' {
  interface PluginState {
    'switchyard-mod': { snapshot: Snapshot | null; page: Page; slot: ColorSlot }
  }
}
