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
    { name: 'zweit', active: false, cooldown_until: null, five_hour: null, seven_day: null },
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
  on('ui.status', () => ({ value: undefined }))
  on('ui.open', () => ({ value: { isPlaced: true } }))

  return fakeCli(on, answer)
}

/** Runs a slash command as typed in the composer. */
function runCommand($: Engine, command: string, args: string) {
  return $.command.run({ command, args, origin: { kind: 'composer' }, presentation: { isFullscreen: false, columns: 80 } })
}

async function openAccounts($: Engine) {
  await runCommand($, 'accounts', '')
}

test('the pane lists the accounts and offers to continue in the other one', async ($, on) => {
  world(on, () => ({ exitCode: 0, stdout: STATUS, stderr: '' }))
  await openAccounts($)

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface, component: 'Pane', props: PANE, requestId: 'accounts' })
    expect(await ui.find({ type: 'Text', text: /main/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /5h 20%/ })).toBeDefined()
    expect(await ui.find({ key: 'switch-zweit' })).toBeDefined()
    expect(await ui.find({ key: 'switch-main' })).toBeUndefined()
    await ui.unmount()
  }
})

test('pressing a button asks switchyard for a handoff', async ($, on) => {
  const calls = world(on, argv => ({ exitCode: 0, stdout: argv[1] === 'status' ? STATUS : 'Handoff to zweit requested\n', stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'accounts' })
  await ui.press({ key: 'fresh-zweit' })
  const handoff = calls.find(argv => argv[1] === 'handoff')
  expect(handoff?.slice(0, 3)).toEqual(['switchyard', 'handoff', 'zweit'])
  expect(handoff?.at(-1)).toBe('--fresh')
  await ui.unmount()
})

test('a missing switchyard is explained in the pane and nowhere else', async ($, on) => {
  world(on, () => null)
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'accounts' })
  expect(await ui.find({ type: 'Text', text: /not available/ })).toBeDefined()
  expect(await ui.find({ key: 'refresh' })).toBeDefined()
  await ui.unmount()
})

test('an incompatible schema is explained in the pane', async ($, on) => {
  world(on, () => ({ exitCode: 0, stdout: JSON.stringify({ schema: 7, profiles: [] }), stderr: '' }))
  await openAccounts($)

  const ui = await $.ui.mount({ plugin: 'switchyard-mod', surface: 'terminal', component: 'Pane', props: PANE, requestId: 'accounts' })
  expect(await ui.find({ type: 'Text', text: /schema 7/ })).toBeDefined()
  await ui.unmount()
})

test('/switch passes the choice on and reports a refusal', async ($, on) => {
  const calls = world(on, argv =>
    argv[1] === 'handoff' ? { exitCode: 1, stdout: '', stderr: 'zweit is already the active profile\n' } : { exitCode: 0, stdout: STATUS, stderr: '' },
  )

  const refused = await runCommand($, 'switch', 'zweit fresh')
  expect(refused.text).toMatch(/already the active profile/)
  expect(calls.find(argv => argv[1] === 'handoff')?.at(-1)).toBe('--fresh')

  const usage = await runCommand($, 'switch', '')
  expect(usage.text).toMatch(/Usage: \/switch/)
})
