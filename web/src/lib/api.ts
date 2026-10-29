// Types mirror the Go server's JSON. See internal/anilist and internal/server.

export type Title = { romaji: string; english: string; native: string; userPreferred: string }

// Zero parts are unknown.
export type FuzzyDate = { year: number; month: number; day: number }

export type ListStatus = 'CURRENT' | 'PLANNING' | 'COMPLETED' | 'PAUSED' | 'DROPPED' | 'REPEATING'

export type ListEntry = {
  id: number
  progress: number
  status: ListStatus
  updatedAt: number
  score: number // in the viewer's score format
  repeat: number
  startedAt: FuzzyDate
  completedAt: FuzzyDate
}

// Fields left out are not changed. A date of all zeros clears it.
export type EntryUpdate = Partial<Pick<ListEntry, 'status' | 'progress' | 'score' | 'repeat' | 'startedAt' | 'completedAt'>>

export type Media = {
  id: number
  title: Title
  synonyms: string[] | null
  format: string
  status: string
  episodes: number
  duration: number
  season: string
  seasonYear: number
  coverImage: { large: string; color: string }
  bannerImage: string
  isAdult: boolean
  description: string
  genres: string[] | null
  averageScore: number
  nextAiringEpisode: { episode: number; airingAt: number } | null
  mediaListEntry: ListEntry | null
}

export type Position = { episode: number; position: number; duration: number; updatedAt: number }

// available: newest episode on the source at the server's last check, or null.
// arrivedAt: when it appeared (Unix seconds), null if it was there at the first check.
export type ListItem = ListEntry & {
  media: Media
  resume: Position | null
  available: number | null
  arrivedAt: number | null
}

export type ScoreFormat = 'POINT_100' | 'POINT_10_DECIMAL' | 'POINT_10' | 'POINT_5' | 'POINT_3'

export type Viewer = {
  id: number
  name: string
  avatar: { medium: string }
  mediaListOptions: { scoreFormat: ScoreFormat | '' }
}

export type Me = { user: Viewer | null; configured: boolean }

export type Mapping = { providerId: string; title: string; epOffset: number; manual: boolean }

export type Candidate = { id: string; title: string; episodes: number; poster: string; score: number }

export type Source = { id: string; name: string }

export type Episode = { number: number; title: string }

export type AnimeDetail = {
  media: Media
  source: Source
  mapping: Mapping | null
  episodes: Episode[]
  candidates?: Candidate[]
  sourceError?: string
}

export type Stream = { url: string; type: string; headers: Record<string, string>; subLang: string; server: string }

export type Interval = { start: number; end: number }

export type SkipTimes = { op: Interval | null; ed: Interval | null }

export type Airing = { episode: number; airingAt: number; media: Media }

export type Season = 'WINTER' | 'SPRING' | 'SUMMER' | 'FALL'

export type SeasonPage = { season: Season; year: number; media: Media[]; hasNext: boolean }

export type SeasonParams = { season: Season; year: number; sort: string; format: string; page: number }

export type Relation = { type: string; media: Media }

export type Extras = { source: string; studios: string[]; relations: Relation[]; recommendations: Media[] }

export type SavedEpisode = { media: Media; episode: number; bytes: number; savedAt: number; usedAt: number }

export type SavedInfo = {
  enabled: boolean
  maxBytes: number
  ahead: number
  usedBytes?: number
  items: SavedEpisode[]
}

export type StreamResult = { episode: number; title: string; streams: Stream[]; resume: Position | null }

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown, keepalive = false): Promise<T> {
  const res = await fetch(path, {
    method,
    keepalive,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
  if (res.status === 204) return undefined as T
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new ApiError(res.status, data.error ?? res.statusText)
  return data as T
}

export const api = {
  me: () => request<Me>('GET', '/api/me'),
  logout: () => request<{ ok: boolean }>('POST', '/api/logout', {}),
  list: () => request<ListItem[]>('GET', '/api/list'),
  fullList: () => request<ListItem[]>('GET', `/api/list?status=${ALL_STATUSES.join(',')}`),
  seasonal: (p?: Partial<SeasonParams>) =>
    request<SeasonPage>('GET', `/api/seasonal?${new URLSearchParams(clean(p ?? {}))}`),
  schedule: (from: number, to: number) => request<Airing[]>('GET', `/api/schedule?from=${from}&to=${to}`),
  extras: (id: number) => request<Extras>('GET', `/api/anime/${id}/extras`),
  saveEntry: (id: number, u: EntryUpdate) => request<ListEntry>('PUT', `/api/anime/${id}/entry`, u),
  deleteEntry: (id: number) => request<void>('DELETE', `/api/anime/${id}/entry`, {}),
  saved: () => request<SavedInfo>('GET', '/api/saved'),
  search: (q: string) => request<Media[]>('GET', `/api/search?q=${encodeURIComponent(q)}`),
  anime: (id: number) => request<AnimeDetail>('GET', `/api/anime/${id}`),
  candidates: (id: number, q: string) =>
    request<Candidate[]>('GET', `/api/anime/${id}/candidates?q=${encodeURIComponent(q)}`),
  saveMapping: (id: number, providerId: string, epOffset: number) =>
    request<Mapping>('PUT', `/api/anime/${id}/mapping`, { providerId, epOffset }),
  stream: (id: number, ep: number) => request<StreamResult>('GET', `/api/anime/${id}/episodes/${ep}/stream`),
  // keepalive lets the last save finish while the page is closing.
  savePosition: (id: number, ep: number, position: number, duration: number, keepalive = false) =>
    request<void>('PUT', `/api/anime/${id}/episodes/${ep}/position`, { position, duration }, keepalive),
  skipTimes: (id: number, ep: number, length: number) =>
    request<SkipTimes>('GET', `/api/anime/${id}/episodes/${ep}/skip?length=${Math.round(length)}`),
  progress: (id: number, episode: number) =>
    request<{ queued: boolean }>('POST', `/api/anime/${id}/progress`, { episode }),
}

export const ALL_STATUSES: ListStatus[] = ['CURRENT', 'REPEATING', 'PLANNING', 'COMPLETED', 'PAUSED', 'DROPPED']

function clean(p: Record<string, unknown>): Record<string, string> {
  const out: Record<string, string> = {}
  for (const [k, v] of Object.entries(p)) if (v !== undefined && v !== '') out[k] = String(v)
  return out
}

export function titleOf(m: Media): string {
  return m.title.userPreferred || m.title.romaji || m.title.english
}

// The next episode to watch, or null when the list entry is finished.
export function nextEpisode(m: Media, progress: number): number | null {
  if (m.episodes > 0 && progress >= m.episodes) return null
  return progress + 1
}

// AniList descriptions are plain text with some HTML tags left in.
export function plainText(s: string): string {
  return s
    .replace(/<br\s*\/?>/gi, '\n')
    .replace(/<[^>]+>/g, '')
    .replace(/\n{3,}/g, '\n\n')
    .trim()
}

// Whether a saved position is worth resuming: past the first seconds and
// before the end credits.
export function resumable(p: Position | null | undefined): p is Position {
  return !!p && p.position > 20 && p.position < p.duration * 0.92
}

// 754 -> "12:34", 3754 -> "1:02:34"
export function clock(seconds: number): string {
  const s = Math.floor(seconds)
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const ss = String(s % 60).padStart(2, '0')
  return h > 0 ? `${h}:${String(m).padStart(2, '0')}:${ss}` : `${m}:${ss}`
}
