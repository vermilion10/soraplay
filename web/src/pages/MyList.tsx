import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, titleOf, type EntryUpdate, type ListItem, type ListStatus } from '../lib/api'
import { formatLabel, formatScore, STATUS_LABEL, useFullList, useMe } from '../lib/list'
import { EntryEditor } from '../components/EntryEditor'
import { Empty, ErrorBox, Loading } from '../components/States'

// Rewatching sits with Watching, as on AniList.
const TABS: { key: string; label: string; statuses: ListStatus[] }[] = [
  { key: 'watching', label: 'Watching', statuses: ['CURRENT', 'REPEATING'] },
  { key: 'planning', label: 'Planning', statuses: ['PLANNING'] },
  { key: 'completed', label: 'Completed', statuses: ['COMPLETED'] },
  { key: 'paused', label: 'Paused', statuses: ['PAUSED'] },
  { key: 'dropped', label: 'Dropped', statuses: ['DROPPED'] },
]

const SORTS = {
  updated: { label: 'Last updated', fn: (a: ListItem, b: ListItem) => b.updatedAt - a.updatedAt },
  title: { label: 'Title', fn: (a: ListItem, b: ListItem) => titleOf(a.media).localeCompare(titleOf(b.media)) },
  score: { label: 'Your score', fn: (a: ListItem, b: ListItem) => b.score - a.score },
  average: { label: 'Average score', fn: (a: ListItem, b: ListItem) => b.media.averageScore - a.media.averageScore },
}
type SortKey = keyof typeof SORTS

export default function MyList() {
  const me = useMe()
  const loggedIn = !!me.data?.user
  const list = useFullList(loggedIn)
  const [params, setParams] = useSearchParams()
  const tab = TABS.find((t) => t.key === params.get('tab')) ?? TABS[0]
  const sort: SortKey = (params.get('sort') as SortKey) in SORTS ? (params.get('sort') as SortKey) : 'updated'
  const [filter, setFilter] = useState('')
  const [editing, setEditing] = useState<ListItem | null>(null)

  if (me.isPending) return <Loading what="your session" />
  if (!loggedIn)
    return (
      <Empty>
        Log in with AniList to see and edit your list.{' '}
        {me.data?.configured && (
          <a href="/auth/login" className="text-ink underline">
            Log in
          </a>
        )}
      </Empty>
    )

  const all = list.data ?? []
  const count = (t: (typeof TABS)[number]) => all.filter((it) => t.statuses.includes(it.status)).length
  const needle = filter.trim().toLowerCase()
  const rows = all
    .filter((it) => tab.statuses.includes(it.status))
    .filter(
      (it) =>
        !needle ||
        [it.media.title.romaji, it.media.title.english, it.media.title.native, ...(it.media.synonyms ?? [])].some((t) =>
          t?.toLowerCase().includes(needle),
        ),
    )
    .sort(SORTS[sort].fn)
  const setParam = (k: string, v: string, dflt: string) => {
    const p = new URLSearchParams(params)
    if (v === dflt) p.delete(k)
    else p.set(k, v)
    setParams(p)
  }

  return (
    <section className="mt-4">
      <h1 className="font-display text-3xl font-bold">My list</h1>
      <div role="tablist" aria-label="Status" className="mt-3 flex gap-1 overflow-x-auto pb-1">
        {TABS.map((t) => (
          <button
            key={t.key}
            type="button"
            role="tab"
            aria-selected={t === tab}
            onClick={() => setParam('tab', t.key, 'watching')}
            className={`min-h-11 shrink-0 rounded-md border px-3 text-sm ${
              t === tab ? 'border-ink text-ink' : 'border-line text-muted hover:border-muted'
            }`}
          >
            {t.label} {list.data && <span className="text-muted">{count(t)}</span>}
          </button>
        ))}
      </div>

      <div className="mt-3 flex flex-wrap gap-2">
        <label htmlFor="list-filter" className="sr-only">
          Filter by title
        </label>
        <input
          id="list-filter"
          type="search"
          value={filter}
          onChange={(e) => setFilter(e.target.value)}
          placeholder="Filter by title"
          className="min-h-11 min-w-0 flex-1 rounded-md border border-line bg-surface px-3 text-sm placeholder:text-muted sm:max-w-xs"
        />
        <label className="flex items-center gap-2 text-sm text-muted">
          Sort
          <select
            value={sort}
            onChange={(e) => setParam('sort', e.target.value, 'updated')}
            className="min-h-11 rounded-md border border-line bg-surface px-2 text-ink"
          >
            {Object.entries(SORTS).map(([k, s]) => (
              <option key={k} value={k}>
                {s.label}
              </option>
            ))}
          </select>
        </label>
      </div>

      <div role="tabpanel" className="mt-4">
        {list.isPending && <Loading what="your list" />}
        {list.isError && <ErrorBox title="Could not load your list from AniList." error={list.error} onRetry={() => list.refetch()} />}
        {list.data && rows.length === 0 && (
          <Empty>{needle ? `Nothing in ${tab.label} matches "${filter.trim()}".` : `Nothing in ${tab.label}.`}</Empty>
        )}
        {rows.length > 0 && (
          <ul className="divide-y divide-line">
            {rows.map((it) => (
              <Row key={it.media.id} it={it} onEdit={() => setEditing(it)} />
            ))}
          </ul>
        )}
      </div>

      {editing && (
        <EntryEditor media={editing.media} entry={editing} open onClose={() => setEditing(null)} />
      )}
    </section>
  )
}

function Row({ it, onEdit }: { it: ListItem; onEdit: () => void }) {
  const me = useMe()
  const fmt = me.data?.user?.mediaListOptions.scoreFormat ?? ''
  const m = it.media
  const watching = it.status === 'CURRENT' || it.status === 'REPEATING'
  const total = m.episodes || 0
  const meta = [it.status === 'REPEATING' && STATUS_LABEL.REPEATING, formatLabel(m.format), m.seasonYear, formatScore(it.score, fmt) && `Score ${formatScore(it.score, fmt)}`]
    .filter(Boolean)
    .join(' · ')

  return (
    <li className="flex items-center gap-3 py-3">
      <Link to={`/anime/${m.id}`} className="shrink-0">
        <img src={m.coverImage.large} alt="" loading="lazy" className="aspect-[2/3] w-12 rounded object-cover" />
      </Link>
      <div className="min-w-0 flex-1">
        <Link to={`/anime/${m.id}`} className="line-clamp-2 leading-snug hover:underline">
          {titleOf(m)}
        </Link>
        <p className="text-xs text-muted">{meta}</p>
      </div>
      <p className="shrink-0 text-right font-display text-lg leading-none font-semibold">
        {it.progress}
        <span className="text-sm font-normal text-muted">/{total || '?'}</span>
      </p>
      {watching && <PlusOne it={it} />}
      <button
        type="button"
        onClick={onEdit}
        aria-label={`Edit ${titleOf(m)}`}
        className="min-h-11 shrink-0 px-2 text-sm text-muted underline hover:text-ink"
      >
        Edit
      </button>
    </li>
  )
}

// One more episode watched, finishing the entry on the last one.
function PlusOne({ it }: { it: ListItem }) {
  const qc = useQueryClient()
  const total = it.media.episodes || 0
  const next = it.progress + 1
  const save = useMutation({
    mutationFn: () => {
      const u: EntryUpdate = { progress: next }
      if (total && next >= total) {
        u.status = 'COMPLETED'
        if (it.status === 'REPEATING') u.repeat = it.repeat + 1
      }
      return api.saveEntry(it.media.id, u)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['list'] })
      qc.invalidateQueries({ queryKey: ['anime', it.media.id] })
    },
  })
  if (total && it.progress >= total) return null
  return (
    <button
      type="button"
      onClick={() => save.mutate()}
      disabled={save.isPending}
      aria-label={`Mark episode ${next} of ${titleOf(it.media)} watched`}
      title={save.isError ? 'Could not save, try again' : `Mark episode ${next} watched`}
      className="min-h-11 min-w-11 shrink-0 rounded-md border border-line font-display text-lg font-semibold hover:border-muted disabled:opacity-60"
    >
      {save.isError ? '!' : '+1'}
    </button>
  )
}
