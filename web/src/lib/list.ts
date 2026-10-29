import { useQuery } from '@tanstack/react-query'
import { api, type FuzzyDate, type ListItem, type ListStatus, type ScoreFormat } from './api'

export const STATUS_LABEL: Record<ListStatus, string> = {
  CURRENT: 'Watching',
  REPEATING: 'Rewatching',
  PLANNING: 'Planning',
  COMPLETED: 'Completed',
  PAUSED: 'Paused',
  DROPPED: 'Dropped',
}

export function useMe() {
  return useQuery({ queryKey: ['me'], queryFn: api.me })
}

// The whole AniList list, every status. Shared by My list, Schedule and
// Seasons, which mark shows already on it.
export function useFullList(enabled: boolean) {
  return useQuery({ queryKey: ['list', 'all'], queryFn: api.fullList, enabled, staleTime: 60_000 })
}

export function byMediaId(items: ListItem[] | undefined): Map<number, ListItem> {
  return new Map((items ?? []).map((it) => [it.media.id, it]))
}

// How a score is entered and shown in each AniList score format.
export function scoreSpec(f: ScoreFormat | '') {
  switch (f) {
    case 'POINT_100':
      return { max: 100, step: 1 }
    case 'POINT_10_DECIMAL':
      return { max: 10, step: 0.1 }
    case 'POINT_5':
      return { max: 5, step: 1 }
    case 'POINT_3':
      return { max: 3, step: 1 }
    default:
      return { max: 10, step: 1 }
  }
}

export const POINT_3_LABELS = ['No score', 'Disliked', 'Average', 'Liked']

export function formatScore(score: number, f: ScoreFormat | ''): string {
  if (!score) return ''
  if (f === 'POINT_3') return POINT_3_LABELS[score] ?? ''
  const { max } = scoreSpec(f)
  return `${score}/${max}`
}

// <input type="date"> takes a full date only; partly known dates show empty.
export function dateValue(d: FuzzyDate): string {
  if (!d.year || !d.month || !d.day) return ''
  return `${d.year}-${String(d.month).padStart(2, '0')}-${String(d.day).padStart(2, '0')}`
}

export function fuzzyDate(v: string): FuzzyDate {
  const [year, month, day] = v.split('-').map(Number)
  return v ? { year, month, day } : { year: 0, month: 0, day: 0 }
}

export function formatLabel(f: string | undefined): string {
  if (!f) return ''
  return { TV_SHORT: 'TV short', MOVIE: 'Movie', SPECIAL: 'Special', MUSIC: 'Music' }[f] ?? f
}
