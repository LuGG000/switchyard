import { expect, mock, test } from 'claude-code/testing'
import type { Engine } from 'claude-code/testing'
import type { On } from 'claude-code'

const PANE = {
  title: 'Accounts',
  isFocused: false,
  bodyColumns: 80,
  placement: 'inline',
  scroll: { offset: 0, bodyRows: 20 },
  view: {},
} as const

const STATUS = JSON.stringify({
  schema: 1,
  version: '0.1.0',
  active: 'main',
  profiles: [
    { name: 'main', active: true, cooldown_until: null, five_hour: { used_percent: 20, resets_at: '', updated_at: '' }, seven_day: null },
    { name: 'work2', active: false, cooldown_until: null, five_hour: null, seven_day: null },
  ],
})

type Run = { exitCode: number; stdout: string; stderr: string }

/** Stands in for the switchyard command: `answer` per argv, every call recorded. */
function fakeCli(on: On, answer: (argv: readonly string[]) => Run | null) {
  const calls: string[][] = []
  on('process.run', (_$, e) => {
    calls.push([...e.argv])
    const run = answer(e.argv)

    return run ? { value: { ...run, isStdoutTruncated: false, isStderrTruncated: false } } : { deny: 'spawn switchyard ENOENT' }
  })

  return calls
}

/** The world beneath the plugin: a clock, a session id and the switchyard command. */
function world(on: On, answer: (argv: readonly string[]) => Run | null) {
  mock.clock(on, { now: Date.parse('2026-10-09T12:00:00Z') })
  on('session.id', () => ({ value: 'session-1' }))
  on('ui.open', () => ({ value: { isPlaced: true } }))

  return fakeCli(on, answer)
}

/** Runs a slash command as typed in the composer. */
function runCommand($: Engine, command: string, args: string) {
  return $.command.run({ command, args, origin: { kind: 'composer' }, presentation: { isFullscreen: false, columns: 80 } })
}

async function openAccounts($: Engine) {
  await runCommand($, 'switchyard', '')
}

test('the pane lists the accounts and offers to continue in the other one', async ($, on) => {
  world(on, () => ({ exitCode: 0, stdout: STATUS, stderr: '' }))
  await openAccounts($)

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface, component: 'Pane', props: PANE, requestId: 'switchyard' })
    expect(await ui.find({ type: 'Text', text: /main/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /20%/ })).toBeDefined()
    expect(await ui.find({ key: 'switch-work2' })).toBeDefined()
    expect(await ui.find({ key: 'switch-main' })).toBeUndefined()
    await ui.unmount()
  }
})

test('pressing a button asks switchyard for a handoff', async ($, on) => {
  const calls = world(on, argv => ({ exitCode: 0, stdout: argv[1] === 'status' ? STATUS : 'Handoff to work2 requested\n', stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  await ui.press({ key: 'fresh-work2' })
  const handoff = calls.find(argv => argv[1] === 'handoff')
  expect(handoff?.slice(0, 3)).toEqual(['switchyard', 'handoff', 'work2'])
  expect(handoff?.at(-1)).toBe('--fresh')
  await ui.unmount()
})

test('a missing switchyard is explained in the pane and nowhere else', async ($, on) => {
  world(on, () => null)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ type: 'Text', text: /not available/ })).toBeDefined()
  expect(await ui.find({ key: 'refresh' })).toBeDefined()
  await ui.unmount()
})

test('an incompatible schema is explained in the pane', async ($, on) => {
  world(on, () => ({ exitCode: 0, stdout: JSON.stringify({ schema: 7, profiles: [] }), stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ type: 'Text', text: /schema 7/ })).toBeDefined()
  await ui.unmount()
})

test('/switch passes the choice on and reports a refusal', async ($, on) => {
  const calls = world(on, argv =>
    argv[1] === 'handoff' ? { exitCode: 1, stdout: '', stderr: 'work2 is already the active profile\n' } : { exitCode: 0, stdout: STATUS, stderr: '' },
  )

  const refused = await runCommand($, 'switchyard', 'switch work2 fresh')
  expect(refused.text).toMatch(/already the active profile/)
  expect(calls.find(argv => argv[1] === 'handoff')?.at(-1)).toBe('--fresh')

  const usage = await runCommand($, 'switchyard', 'switch')
  expect(usage.text).toMatch(/Usage: \/switchyard/)
})

const COLORS = { background: '', text: '', low: '', medium: '', high: '', border_active: '', border: '' }
const CONFIG = JSON.stringify({ schema: 1, mode: 'ask', carry_context: true, strategy: 'sequential', proactive_threshold: 0, colors: COLORS })

/** A switchyard that answers status, config and `config set`. */
function withConfig(argv: readonly string[]): Run {
  if (argv[1] === 'config') {
    return { exitCode: 0, stdout: argv[2] === 'set' ? `${argv[3]} = ${argv[4]}\n` : CONFIG, stderr: '' }
  }

  return { exitCode: 0, stdout: STATUS, stderr: '' }
}

test('the pane shows the failover settings and marks the current choice', async ($, on) => {
  world(on, withConfig)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect((await ui.find({ key: 'mode-ask' }))?.text).toMatch(/✓/)
  expect((await ui.find({ key: 'mode-auto' }))?.text).not.toMatch(/✓/)
  expect((await ui.find({ key: 'carry-on' }))?.text).toMatch(/✓/)
  await ui.unmount()
})

test('pressing a failover choice changes the setting through switchyard', async ($, on) => {
  const calls = world(on, withConfig)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  await ui.press({ key: 'mode-auto' })
  await ui.press({ key: 'carry-off' })
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'mode', 'auto'])
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'carry_context', 'false'])
  await ui.unmount()
})

test('an older switchyard without config leaves the settings out of the pane', async ($, on) => {
  world(on, argv => (argv[1] === 'config' ? { exitCode: 1, stdout: '', stderr: 'unknown command' } : { exitCode: 0, stdout: STATUS, stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ key: 'mode-auto' })).toBeUndefined()
  expect(await ui.find({ key: 'switch-work2' })).toBeDefined()
  await ui.unmount()
})

test('/switchyard mode shows, sets and refuses', async ($, on) => {
  const calls = world(on, withConfig)

  expect((await runCommand($, 'switchyard', 'mode')).text).toBe('At a limit: ask (asks in the terminal). Conversation: taken along.')
  expect((await runCommand($, 'switchyard', 'mode auto')).text).toBe('mode = auto')
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'mode', 'auto'])
  expect((await runCommand($, 'switchyard', 'mode sometimes')).text).toMatch(/Usage: \/switchyard/)
})

test('the pane follows the theme unless colors are set', async ($, on) => {
  world(on, withConfig)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  const drawn = JSON.stringify(await ui.drawn())
  expect(drawn).not.toContain('backgroundColor')
  expect(drawn).toContain('"success"')
  await ui.unmount()
})

test('the colors of the switchyard config are used by the pane', async ($, on) => {
  const colored = JSON.stringify({ schema: 1, mode: 'ask', carry_context: true, colors: { ...COLORS, background: '#101010', text: 'white', low: 'cyan', border_active: '#00ff88' } })
  world(on, argv => (argv[1] === 'config' ? { exitCode: 0, stdout: colored, stderr: '' } : { exitCode: 0, stdout: STATUS, stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  const drawn = JSON.stringify(await ui.drawn())
  for (const wanted of ['"backgroundColor":"#101010"', '"color":"white"', '"color":"cyan"', '"#00ff88"']) {
    expect(drawn).toContain(wanted)
  }
  await ui.unmount()
})

test('pressing a palette changes the colors through switchyard', async ($, on) => {
  const calls = world(on, withConfig)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  await ui.press({ key: 'to-style' })
  await ui.press({ key: 'colors-dark' })
  await ui.press({ key: 'colors-default' })
  expect(calls).toContainEqual(['switchyard', 'config', 'colors', 'dark'])
  expect(calls).toContainEqual(['switchyard', 'config', 'colors', 'default'])
  await ui.unmount()
})

test('the style page shows the palettes, a preview and the colors in use, and leads back', async ($, on) => {
  const dark = JSON.stringify({ schema: 1, mode: 'ask', carry_context: true, colors: { ...COLORS, background: '#1e1e1e', high: '#f48771' } })
  world(on, argv => (argv[1] === 'config' ? { exitCode: 0, stdout: dark, stderr: '' } : { exitCode: 0, stdout: STATUS, stderr: '' }))
  await runCommand($, 'switchyard', 'style')

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  for (const key of ['colors-default', 'colors-dark', 'colors-light', 'back']) {
    expect(await ui.find({ key })).toBeDefined()
  }
  expect(JSON.stringify(await ui.drawn())).toContain('background: #1e1e1e')
  expect(await ui.find({ type: 'Text', text: /from 90%/ })).toBeDefined()
  expect(await ui.find({ key: 'mode-auto' })).toBeUndefined()

  await ui.press({ key: 'back' })
  expect(await ui.find({ key: 'mode-auto' })).toBeDefined()
  expect(await ui.find({ key: 'to-style' })).toBeDefined()
  await ui.unmount()
})

test('a color can be chosen by button, typed in, or cleared on the style page', async ($, on) => {
  const custom = JSON.stringify({ schema: 1, mode: 'ask', carry_context: true, colors: { ...COLORS, text: '#abcdef' } })
  const calls = world(on, argv => (argv[1] === 'config' && argv[2] === 'set' ? { exitCode: 0, stdout: '', stderr: '' } : argv[1] === 'config' ? { exitCode: 0, stdout: custom, stderr: '' } : { exitCode: 0, stdout: STATUS, stderr: '' }))
  await runCommand($, 'switchyard', 'style')

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ key: 'slot-background' })).toBeDefined()
  expect(await ui.find({ key: 'custom-color' })).toBeDefined()

  await ui.press({ key: 'pick-cyan' })
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'color_background', 'cyan'])

  await ui.press({ key: 'slot-text' })
  await ui.press({ key: 'pick-default' })
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'color_text', ''])

  await ui.input({ key: 'custom-color', text: 'active #00ff88' })
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'color_border_active', '#00ff88'])

  const before = calls.length
  await ui.input({ key: 'custom-color', text: 'two words here' })
  expect(calls.length).toBe(before)
  await ui.unmount()
})

test('every page has a Close button that closes the pane', async ($, on) => {
  const closed: string[] = []
  on('ui.close', (_$, e) => {
    closed.push(e.id)

    return { value: undefined }
  })
  world(on, withConfig)

  for (const page of ['', 'style']) {
    await runCommand($, 'switchyard', page)
    const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
    await ui.press({ key: 'close' })
    await ui.unmount()
  }
  expect(closed).toEqual(['switchyard', 'switchyard'])
})

test('the style page marks the current value of the chosen color and follows the choice', async ($, on) => {
  const colored = JSON.stringify({ schema: 1, mode: 'ask', carry_context: true, colors: { ...COLORS, text: 'red' } })
  world(on, argv => (argv[1] === 'config' ? { exitCode: 0, stdout: colored, stderr: '' } : { exitCode: 0, stdout: STATUS, stderr: '' }))
  await runCommand($, 'switchyard', 'style')

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect((await ui.find({ key: 'pick-default' }))?.text).toMatch(/✓/)
  await ui.press({ key: 'slot-text' })
  expect((await ui.find({ key: 'pick-red' }))?.text).toMatch(/✓/)
  expect((await ui.find({ key: 'pick-default' }))?.text).not.toMatch(/✓/)
  await ui.unmount()
})

test('a hex code typed on the style page applies to the chosen color', async ($, on) => {
  const calls = world(on, argv => (argv[1] === 'config' && argv[2] === 'set' ? { exitCode: 0, stdout: '', stderr: '' } : argv[1] === 'config' ? { exitCode: 0, stdout: CONFIG, stderr: '' } : { exitCode: 0, stdout: STATUS, stderr: '' }))
  await runCommand($, 'switchyard', 'style')

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  await ui.press({ key: 'slot-active' })
  await ui.input({ key: 'custom-color', text: '#00ff88' })
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'color_border_active', '#00ff88'])

  await ui.press({ key: 'slot-medium' })
  await ui.input({ key: 'custom-color', text: 'ffaa00' })
  expect(calls).toContainEqual(['switchyard', 'config', 'set', 'color_medium', '#ffaa00'])
  await ui.unmount()
})

test('the active account is green by default', async ($, on) => {
  world(on, withConfig)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(JSON.stringify(await ui.drawn())).toContain('"borderColor":"green"')
  await ui.unmount()
})

const WITH_UPDATE = JSON.stringify({ ...JSON.parse(STATUS), update: { version: '0.2.0', url: 'https://example.test/v0.2.0' } })

test('a newer release is offered in the pane and installed in the background', async ($, on) => {
  let installed = false
  const calls = world(on, argv => {
    if (argv[1] === 'update') {
      installed = true

      return { exitCode: 0, stdout: 'Updated switchyard 0.1.0 to 0.2.0.\n', stderr: '' }
    }

    return { exitCode: 0, stdout: installed ? STATUS : WITH_UPDATE, stderr: '' }
  })
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ type: 'Text', text: /Update available: switchyard 0\.2\.0/ })).toBeDefined()
  await ui.press({ key: 'update-install' })
  expect(calls.find(argv => argv[1] === 'update')).toEqual(['switchyard', 'update', '--install'])
  expect(await ui.find({ key: 'update-install' })).toBeUndefined()
  await ui.unmount()
})

test('a newer release is mentioned once with a toast', async ($, on) => {
  const toasts: string[] = []
  on('ui.toast', (_, e) => {
    toasts.push(e.text)

    return { value: undefined }
  })
  world(on, () => ({ exitCode: 0, stdout: WITH_UPDATE, stderr: '' }))
  await openAccounts($)
  await openAccounts($)

  expect(toasts.filter(text => /switchyard 0\.2\.0 is available/.test(text)).length).toBe(1)
})

test('without a newer release the pane has no update button', async ($, on) => {
  world(on, () => ({ exitCode: 0, stdout: STATUS, stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ key: 'update-install' })).toBeUndefined()
  await ui.unmount()
})

test('/switchyard update installs the release, or says it is up to date', async ($, on) => {
  const calls = world(on, argv => ({ exitCode: 0, stdout: argv[1] === 'status' ? WITH_UPDATE : 'ok\n', stderr: '' }))
  const result = await runCommand($, 'switchyard', 'update')
  expect(calls.some(argv => argv[1] === 'update' && argv[2] === '--install')).toBe(true)
  expect(JSON.stringify(result)).toMatch(/0\.2\.0/)
})

const QUESTION = JSON.stringify({
  schema: 1,
  pending: { profile: 'main', reason: 'rate_limit', options: ['work2'], carry: true, expires_at: '2026-10-09T12:02:00Z' },
})

/** A world in which main hit its limit: `decision --json` shows the question until it is answered. */
function limitWorld(on: On) {
  let answered = false

  return fakeCli(on, argv => {
    if (argv[1] === 'decision' && argv[2] === 'answer') {
      answered = true

      return { exitCode: 0, stdout: 'Answered\n', stderr: '' }
    }
    if (argv[1] === 'decision') {
      return { exitCode: 0, stdout: answered ? JSON.stringify({ schema: 1, pending: null }) : QUESTION, stderr: '' }
    }

    return { exitCode: 0, stdout: STATUS, stderr: '' }
  })
}

test('a limit that waits shows buttons, and a button answers it', async ($, on) => {
  mock.clock(on, { now: Date.parse('2026-10-09T12:00:00Z') })
  on('session.id', () => ({ value: 'session-1' }))
  on('ui.open', () => ({ value: { isPlaced: true } }))
  const calls = limitWorld(on)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ type: 'Text', text: /main reached its limit/ })).toBeDefined()
  expect(await ui.find({ type: 'Text', text: /120 s left/ })).toBeDefined()
  await ui.press({ key: 'decide-fresh-work2' })
  expect(calls.find(argv => argv[2] === 'answer')).toEqual(['switchyard', 'decision', 'answer', 'switch', 'work2', '--fresh'])
  expect(await ui.find({ key: 'decide-work2' })).toBeUndefined()
  await ui.unmount()
})

for (const [name, key, answer] of [
  ['continue', 'decide-work2', ['switch', 'work2', '--resume']],
  ['stay', 'decide-stay', ['stay']],
] as const) {
  test(`the ${name} button sends its own answer`, async ($, on) => {
    mock.clock(on, { now: Date.parse('2026-10-09T12:00:00Z') })
    on('session.id', () => ({ value: 'session-1' }))
    on('ui.open', () => ({ value: { isPlaced: true } }))
    const calls = limitWorld(on)
    await openAccounts($)

    const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
    await ui.press({ key })
    expect(calls.find(argv => argv[2] === 'answer')?.slice(3)).toEqual([...answer])
    await ui.unmount()
  })
}

test('without a waiting limit the pane shows no question', async ($, on) => {
  world(on, () => ({ exitCode: 0, stdout: STATUS, stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'switchyard' })
  expect(await ui.find({ key: 'decide-stay' })).toBeUndefined()
  await ui.unmount()
})
