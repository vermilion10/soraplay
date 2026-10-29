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
  description: string
  genres: string[] | null
  averageScore: number
  nextAiringEpisode: { episode: number; airingAt: number } | null
  mediaListEntry: ListEntry | null
}

export type ListItem = ListEntry & { media: Media }

export type Viewer = { id: number; name: string; avatar: { medium: string } }

export type Me = { user: Viewer | null; configured: boolean }

export type Mapping = { providerId: string; title: string; epOffset: number; manual: boolean }

export type Candidate = { id: string; title: string; episodes: number; poster: string; score: number }

export type Episode = { number: number; title: string }

export type AnimeDetail = {
  media: Media
  mapping: Mapping | null
  episodes: Episode[]
  candidates?: Candidate[]
  sourceError?: string
}

export type Stream = { url: string; type: string; headers: Record<string, string>; subLang: string; server: string }

export type StreamResult = { episode: number; title: string; streams: Stream[] }

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body === undefined ? undefined : { 'Content-Type': 'application/json' },
    body: body === undefined ? undefined : JSON.stringify(body),
  })
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
