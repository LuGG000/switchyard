import { expect, test } from 'claude-code/testing'

import { palette } from '../hooks/palette'
import type { Colors } from '../types'

const EMPTY: Colors = { background: '', text: '', low: '', medium: '', high: '', border_active: '', border: '' }

test('without colors the pane follows the theme', async () => {
  const theme = {
    background: undefined,
    text: undefined,
    low: 'success',
    medium: 'warning',
    high: 'error',
    borderActive: 'green',
    border: 'subtle',
  }
  expect(palette(null)).toEqual(theme)
  expect(palette(undefined)).toEqual(theme)
  expect(palette(EMPTY)).toEqual(theme)
})

test('blank colors count as unset', async () => {
  expect(palette({ ...EMPTY, background: '   ', text: '' })).toEqual(palette(null))
})

test('set colors override only their own entry', async () => {
  const colors = palette({ ...EMPTY, background: ' #101010 ', high: 'magenta', border_active: '#00ff88' })
  expect(colors.background).toBe('#101010')
  expect(colors.high).toBe('magenta')
  expect(colors.borderActive).toBe('#00ff88')
  expect(colors.low).toBe('success')
  expect(colors.text).toBeUndefined()
})
