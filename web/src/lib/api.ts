// Types mirror the Go server's JSON. See internal/anilist and internal/server.

export type Title = { romaji: string; english: string; native: string; userPreferred: string }

export type ListEntry = { progress: number; status: string; updatedAt: number }

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

export type ListItem = ListEntry & { media: Media; resume: Position | null }

export type Viewer = { id: number; name: string; avatar: { medium: string } }

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
  seasonal: () => request<{ season: string; year: number; media: Media[] }>('GET', '/api/seasonal'),
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
  progress: (id: number, episode: number) =>
    request<{ queued: boolean }>('POST', `/api/anime/${id}/progress`, { episode }),
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
