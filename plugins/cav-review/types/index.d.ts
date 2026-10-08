// What the band draws: the newest review file in the renders folder.
export type ReviewSummary = {
  video: string // the render's path, as cav review recorded it
  file: string // the review file
  open: number // open notes
  resolved: number
  olderOpen: number // open notes made on an older render
  pending: { n: number; notes: number } | null // a send no agent has picked up
  url: string | null // the running review page, when cav review serves it
}

declare module 'claude-code' {
  interface PluginState {
    'cav-review': {
      summary: ReviewSummary | null
      hidden: boolean
      // The last send announced, so a send wakes the session once.
      announced: string | null
    }
  }
}
