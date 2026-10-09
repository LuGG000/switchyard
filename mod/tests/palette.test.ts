import { expect, test } from 'claude-code/testing'

import { palette } from '../hooks/palette'

test('without options the pane follows the theme', async () => {
  expect(palette({})).toEqual({
    background: undefined,
    text: undefined,
    low: 'success',
    medium: 'warning',
    high: 'error',
    borderActive: 'success',
    border: 'subtle',
  })
})

test('empty and blank options count as unset', async () => {
  expect(palette({ background: '', text: '   ', usageLow: '' })).toEqual(palette({}))
})

test('set options override only their own color', async () => {
  const colors = palette({ background: ' #101010 ', usageHigh: 'magenta', borderActive: '#00ff88' })
  expect(colors.background).toBe('#101010')
  expect(colors.high).toBe('magenta')
  expect(colors.borderActive).toBe('#00ff88')
  expect(colors.low).toBe('success')
  expect(colors.text).toBeUndefined()
})

test('values that are not strings are ignored', async () => {
  expect(palette({ background: 5, text: true, usageLow: ['red'] })).toEqual(palette({}))
})
