import { useSearchParams } from 'react-router'
import { useInfiniteQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { api, type Media, type Season } from '../lib/api'
import { byMediaId, formatLabel, STATUS_LABEL, useFullList, useMe } from '../lib/list'
import { PosterGrid } from '../components/PosterGrid'
import { Empty, ErrorBox, Loading } from '../components/States'

const ORDER: Season[] = ['WINTER', 'SPRING', 'SUMMER', 'FALL']

const FORMATS = [
  { v: '', label: 'All' },
  { v: 'TV', label: 'TV' },
  { v: 'MOVIE', label: 'Movies' },
  { v: 'ONA', label: 'ONA' },
  { v: 'OVA,SPECIAL', label: 'OVA & specials' },
]

const SORTS = [
  { v: 'POPULARITY_DESC', label: 'Most popular' },
  { v: 'SCORE_DESC', label: 'Highest score' },
  { v: 'TRENDING_DESC', label: 'Trending now' },
  { v: 'START_DATE', label: 'Start date' },
]

export function currentSeason(): { season: Season; year: number } {
  const d = new Date()
  return { season: ORDER[Math.floor(d.getMonth() / 3)], year: d.getFullYear() }
}

function shift(season: Season, year: number, by: number) {
  const i = ORDER.indexOf(season) + by
  return { season: ORDER[(i + 4) % 4], year: year + Math.floor(i / 4) }
}

export function seasonName(season: Season, year: number) {
  return `${season.charAt(0)}${season.slice(1).toLowerCase()} ${year}`
}

export default function Seasons() {
  const [params, setParams] = useSearchParams()
  const now = currentSeason()
  const season = (ORDER.includes(params.get('season') as Season) ? params.get('season') : now.season) as Season
  const year = Number(params.get('year')) || now.year
  const sort = SORTS.some((s) => s.v === params.get('sort')) ? params.get('sort')! : SORTS[0].v
  const format = FORMATS.some((f) => f.v === params.get('format')) ? params.get('format')! : ''

  const me = useMe()
  const loggedIn = !!me.data?.user
  const list = useFullList(loggedIn)
  const onList = byMediaId(list.data)

  const q = useInfiniteQuery({
    queryKey: ['seasonal', season, year, sort, format],
    queryFn: ({ pageParam }) => api.seasonal({ season, year, sort, format, page: pageParam }),
    initialPageParam: 1,
    getNextPageParam: (last, all) => (last.hasNext && all.length < 20 ? all.length + 1 : undefined),
    staleTime: 3_600_000,
  })

  function go(next: Partial<Record<'season' | 'year' | 'sort' | 'format', string>>) {
    const p: Record<string, string> = { season, year: String(year), sort, format, ...next }
    if (p.season === now.season && p.year === String(now.year)) {
      delete p.season
      delete p.year
    }
    if (p.sort === SORTS[0].v) delete p.sort
    if (!p.format) delete p.format
    setParams(p)
  }
  const prev = shift(season, year, -1)
  const next = shift(season, year, 1)
  const media = q.data?.pages.flatMap((p) => p.media) ?? []

  return (
    <section className="mt-4">
      <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
        <button
          type="button"
          onClick={() => go({ season: prev.season, year: String(prev.year) })}
          className="min-h-11 px-1 text-sm text-muted hover:text-ink"
        >
          ‹ {seasonName(prev.season, prev.year)}
        </button>
        <h1 className="mx-2 font-display text-3xl font-bold">{seasonName(season, year)}</h1>
        <button
          type="button"
          onClick={() => go({ season: next.season, year: String(next.year) })}
          className="min-h-11 px-1 text-sm text-muted hover:text-ink"
        >
          {seasonName(next.season, next.year)} ›
        </button>
        {(season !== now.season || year !== now.year) && (
          <button type="button" onClick={() => go({ season: now.season, year: String(now.year) })} className="min-h-11 px-1 text-sm underline">
            This season
          </button>
        )}
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-2">
        <div role="radiogroup" aria-label="Format" className="flex flex-wrap gap-1">
          {FORMATS.map((f) => (
            <button
              key={f.v}
              type="button"
              role="radio"
              aria-checked={format === f.v}
              onClick={() => go({ format: f.v })}
              className={`min-h-10 rounded-md border px-3 text-sm ${
                format === f.v ? 'border-ink text-ink' : 'border-line text-muted hover:border-muted'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>
        <label className="ml-auto flex items-center gap-2 text-sm text-muted">
          Sort
          <select
            value={sort}
            onChange={(e) => go({ sort: e.target.value })}
            className="min-h-10 rounded-md border border-line bg-surface px-2 text-ink"
          >
            {SORTS.map((s) => (
              <option key={s.v} value={s.v}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div className="mt-5">
        {q.isPending && <Loading what={`${seasonName(season, year)} from AniList`} />}
        {q.isError && <ErrorBox title="Could not load this season from AniList." error={q.error} onRetry={() => q.refetch()} />}
        {q.data && media.length === 0 && <Empty>AniList lists nothing for this season and format.</Empty>}
        {media.length > 0 && (
          <PosterGrid
            items={media.map((m) => {
              const entry = onList.get(m.id)
              return {
                media: m,
                note: entry ? `On your list: ${STATUS_LABEL[entry.status]}` : seasonNote(m),
                footer: loggedIn && !entry && list.isSuccess ? <AddToPlanning id={m.id} /> : undefined,
              }
            })}
          />
        )}
        {q.hasNextPage && (
          <button
            type="button"
            onClick={() => q.fetchNextPage()}
            disabled={q.isFetchingNextPage}
            className="mt-6 min-h-11 rounded-md border border-line px-5 text-sm hover:border-muted"
          >
            {q.isFetchingNextPage ? 'Loading…' : 'Show more'}
          </button>
        )}
      </div>
    </section>
  )
}

function seasonNote(m: Media): string {
  return [formatLabel(m.format), m.episodes ? `${m.episodes} eps` : null, m.averageScore ? `${m.averageScore}%` : null]
    .filter(Boolean)
    .join(' · ')
}

export function AddToPlanning({ id }: { id: number }) {
  const qc = useQueryClient()
  const add = useMutation({
    mutationFn: () => api.saveEntry(id, { status: 'PLANNING' }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['list'] }),
  })
  return (
    <button
      type="button"
      onClick={() => add.mutate()}
      disabled={add.isPending}
      className="min-h-11 text-xs text-muted underline hover:text-ink disabled:no-underline"
    >
      {add.isPending ? 'Adding…' : add.isError ? 'Failed, try again' : 'Add to Planning'}
    </button>
  )
}
