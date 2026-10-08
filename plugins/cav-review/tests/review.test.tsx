import { expect, test } from 'claude-code/testing'

// What `cav review export --json` prints for a render with two open notes (one made on an
// older render), one resolved, and send #2 waiting for the agent.
const exported = {
  ok: true,
  video: 'renders/final.mp4',
  reviewFile: 'renders/final.review.json',
  notes: [
    { id: 'c_01', status: 'open' },
    { id: 'c_02', status: 'open', onOlderRender: true },
    { id: 'c_03', status: 'resolved' },
  ],
  sends: [
    { n: 1, at: '2026-10-08T08:00:00Z', comments: ['c_03'], deliveredAt: '2026-10-08T08:00:01Z' },
    { n: 2, at: '2026-10-08T09:00:00Z', comments: ['c_01', 'c_02'] },
  ],
  server: { url: 'http://127.0.0.1:8790/' },
}

const band = {
  component: 'AbovePrompt' as const,
  props: { hasSurvey: false, isWorking: false, maxRows: 10, bodyColumns: 100 },
}

const ran = (stdout: string, exitCode = 0) => ({ value: { exitCode, stdout, stderr: '', isStdoutTruncated: false, isStderrTruncated: false } })

test('a sent review shows in the band, wakes the session once, and follows file changes', async ($, on) => {
  let current: object = exported
  const prompts: string[] = []
  const argvs: string[][] = []
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('process.run', ($, e) => {
    argvs.push([...e.argv])
    return ran(JSON.stringify(current))
  })
  on('prompt.submit', ($, e) => {
    prompts.push(e.text)
    return { text: e.text }
  })
  on('ui.toast', () => ({ value: undefined }))
  on('classic.FileChanged', () => ({}))
  on('ui.render', ($, e) => {
    const { Box } = $.ui.resolve(e)
    return <Box />
  })

  await $.session.start({ cwd: '/project', surface: 'terminal', isInteractive: true })
  expect(argvs[0]).toEqual(['cav', 'review', 'export', '--json', '--dir', 'renders'])
  expect(prompts.length).toBe(1)
  expect(prompts[0]).toContain('cav review wait renders/final.mp4')

  for (const surface of ['terminal', 'desktop'] as const) {
    const ui = await $.ui.mount({ plugin: 'cav-review', surface, ...band })
    expect(await ui.find({ type: 'Text', text: /2 open notes \(1 on an older render\)/ })).toBeDefined()
    expect(await ui.find({ type: 'Text', text: /send #2 waiting: 2 notes/ })).toBeDefined()
    expect(await ui.find({ type: 'Link', text: 'open the page' })).toBeDefined()
    await ui.unmount()
  }

  // The agent read the notes: cav review wait marked the send received.
  current = { ...exported, sends: exported.sends.map(s => ({ ...s, deliveredAt: '2026-10-08T09:00:05Z' })) }
  await $.classic.FileChanged({ file_path: '/project/renders/final.review.json', event: 'change' })
  const ui = await $.ui.mount({ plugin: 'cav-review', surface: 'terminal', ...band })
  expect(await ui.find({ type: 'Text', text: /waiting/ })).toBeUndefined()
  expect(await ui.find({ type: 'Text', text: /2 open notes/ })).toBeDefined()
  expect(prompts.length).toBe(1)
  await ui.unmount()
})

test('without a review, or without cav, the band stays empty', async ($, on) => {
  on('session.start', ($, e) => ({ cwd: e.cwd }))
  on('process.run', () => ran('', 1))
  on('ui.render', ($, e) => {
    const { Box } = $.ui.resolve(e)
    return <Box />
  })
  await $.session.start({ cwd: '/project', surface: 'terminal', isInteractive: true })
  const ui = await $.ui.mount({ plugin: 'cav-review', surface: 'terminal', ...band })
  expect(await ui.find({ type: 'Text', text: /cav review/ })).toBeUndefined()
  await ui.unmount()
})
