import { atom, read, update } from 'claude-code'
import type { EngineInterface, Register } from 'claude-code'

import type { ReviewSummary } from '../types'

// cav review keeps its notes in <renders>/<video>.review.json. This plugin asks Claude Code
// to watch that folder and reads the newest file when it changes: a band above the prompt
// shows the open notes and links to the page, a toast says when the reviewer presses "Send to
// agent", and (option `wake`) a turn starts that reads the notes with `cav review wait`.

const summary = atom({ plugin: 'cav-review', key: 'summary' } as const, null)
const hidden = atom({ plugin: 'cav-review', key: 'hidden' } as const, false)
const announced = atom({ plugin: 'cav-review', key: 'announced' } as const, null)

// What `cav review export --json` prints (the fields the band uses).
type Export = {
  video: string
  reviewFile: string
  notes?: { status: string; onOlderRender?: boolean }[] | null
  sends?: { n: number; comments: string[]; deliveredAt?: string }[] | null
  server?: { url: string } | null
}

function summarize(x: Export): ReviewSummary {
  const notes = x.notes ?? []
  const open = notes.filter(n => n.status === 'open')
  const pending = (x.sends ?? []).find(s => !s.deliveredAt)
  return {
    video: x.video,
    file: x.reviewFile,
    open: open.length,
    resolved: notes.length - open.length,
    olderOpen: open.filter(n => n.onOlderRender).length,
    pending: pending ? { n: pending.n, notes: pending.comments.length } : null,
    // Surfaces link only https: and http://localhost; the review server answers both names.
    url: x.server?.url ? x.server.url.replace('//127.0.0.1:', '//localhost:') : null,
  }
}

function wakePrompt(s: ReviewSummary): string {
  return (
    `The reviewer pressed "Send to agent" in cav review: ${s.pending?.notes ?? 0} note(s) on ${s.video}. ` +
    `Run \`cav review wait ${s.video} --timeout 10s\` to read them (it prints each note's frame, text, drawing and snapshot path, ` +
    `and marks the send received), look at each snapshot, fix the notes, re-render to the same file, then resolve each with ` +
    `\`cav review resolve <id> --note "what changed"\`.`
  )
}

// Reads the newest review file and updates the band. Nothing polls: this runs when the
// session starts, when Claude Code reports a change to a watched file, and around turns.
async function refresh($: EngineInterface, folder: string, wake: boolean) {
  // cav finds the newest review file and reads it, so the format lives in one place.
  const r = await $.process.run(['cav', 'review', 'export', '--json', '--dir', folder], { timeoutMs: 15000 })
  let x: Export | null = null
  if (r.exitCode === 0) {
    try {
      x = JSON.parse(r.stdout)
    } catch {
      return
    }
  }
  if (x === null || !x.reviewFile) {
    if ((await read($, summary)) !== null) await update($, summary, () => null)
    return
  }
  const s = summarize(x)
  const before = await read($, summary)
  if (JSON.stringify(before) !== JSON.stringify(s)) await update($, summary, () => s)

  if (s.pending) {
    const key = `${s.file}#${s.pending.n}`
    if ((await read($, announced)) !== key) {
      await update($, announced, () => key)
      await update($, hidden, () => false)
      $.ui.toast(`cav review: the reviewer sent ${s.pending.notes} note(s) on ${s.video}`, { timeoutMs: 8000 })
      if (wake) await $.prompt.submit({ text: wakePrompt(s) })
    }
  }
}

export const register: Register = (on, options) => {
  const folder = typeof options.folder === 'string' && options.folder !== '' ? options.folder : 'renders'
  const wake = options.wake !== false

  // Ask Claude Code to watch the renders folder and the review files in it.
  on('classic.SessionStart', async ($, e, next) => {
    const result = await next(e)
    const base = `${e.cwd}/${folder}`
    const paths = [base]
    try {
      for (const f of await $.fs.list(folder)) {
        if (f.kind === 'file' && f.name.endsWith('.review.json')) paths.push(`${base}/${f.name}`)
      }
    } catch {
      // the folder appears later; watching it catches that
    }
    return { ...result, watchPaths: [...(result.watchPaths ?? []), ...paths] }
  }).catch(($, e, next) => next(e))

  on('classic.FileChanged', async ($, e, next) => {
    if (e.file_path.endsWith('.review.json') || e.file_path.endsWith(`/${folder}`)) await refresh($, folder, wake)
    return next(e)
  }).catch(($, e, next) => next(e))

  on('session.start', async ($, e, next) => {
    await refresh($, folder, wake)
    return next(e)
  })

  // A failure here must never hold up the person's prompt.
  on('prompt.submit', async ($, e, next) => {
    refresh($, folder, wake).catch(() => {})
    return next(e)
  }).catch(($, e, next) => next(e))

  on('turn.complete', async ($, e, next) => {
    refresh($, folder, wake).catch(() => {})
    return next(e)
  }).catch(($, e, next) => next(e))

  on('ui.render', { component: 'AbovePrompt' }, async ($, e, next) => {
    const s = await read($, summary)
    if (s === null || e.props.hasSurvey || (await read($, hidden)) || (s.open === 0 && !s.pending)) {
      return next(e)
    }
    const { Box, Button, Link, Text } = $.ui.resolve(e)
    const name = s.video.split('/').pop()
    return (
      <Box flexDirection="row" gap={1} flexWrap="wrap">
        <Text bold>cav review</Text>
        <Text>
          {name}: {s.open} open note{s.open === 1 ? '' : 's'}
          {s.olderOpen > 0 ? ` (${s.olderOpen} on an older render)` : ''}
        </Text>
        {s.pending ? (
          <Text color="yellow">
            send #{s.pending.n} waiting: {s.pending.notes} note{s.pending.notes === 1 ? '' : 's'}
          </Text>
        ) : null}
        {s.url ? <Link href={s.url} label="open the page" /> : null}
        {s.pending ? (
          <Button key="handle" label="Handle notes" onPress={() => $.prompt.submit({ text: wakePrompt(s) })} />
        ) : null}
        <Button key="hide" label="Hide" onPress={() => update($, hidden, () => true)} />
      </Box>
    )
  })
}
