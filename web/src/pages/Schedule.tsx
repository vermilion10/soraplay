import { useState } from 'react'
import { Link, useSearchParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, titleOf, type Airing, type ListItem } from '../lib/api'
import { byMediaId, STATUS_LABEL, useFullList, useMe } from '../lib/list'
import { Empty, ErrorBox, Loading } from '../components/States'

// Days start at local midnight, weeks on Monday, so the schedule follows the
// viewer's own clock rather than Japan's.
function weekDays(offset: number): Date[] {
  const start = new Date()
  start.setHours(0, 0, 0, 0)
  start.setDate(start.getDate() - ((start.getDay() + 6) % 7) + offset * 7)
  return Array.from({ length: 8 }, (_, i) => {
    const d = new Date(start)
    d.setDate(start.getDate() + i)
    return d
  })
}

const MINE_KEY = 'schedule.mine'

// "On my list" means shows being followed, not ones finished or dropped.
const FOLLOWING = new Set(['CURRENT', 'REPEATING', 'PLANNING', 'PAUSED'])

function readMine(): boolean {
  try {
    return localStorage.getItem(MINE_KEY) === '1'
  } catch {
    return false
  }
}

export default function Schedule() {
  const [params, setParams] = useSearchParams()
  const week = Math.max(-8, Math.min(8, Number(params.get('week')) || 0))
  const days = weekDays(week)
  const from = days[0].getTime() / 1000
  const to = days[7].getTime() / 1000
  const todayIdx = week === 0 ? (new Date().getDay() + 6) % 7 : 0
  const [picked, setPicked] = useState<{ week: number; day: number } | null>(null)
  const day = picked?.week === week ? picked.day : todayIdx

  const me = useMe()
  const loggedIn = !!me.data?.user
  const [mineSetting, setMineSetting] = useState(readMine)
  const mine = loggedIn && mineSetting
  const list = useFullList(loggedIn)
  const onList = byMediaId(list.data)
  const q = useQuery({ queryKey: ['schedule', from, to], queryFn: () => api.schedule(from, to), staleTime: 3_600_000 })

  function setMine(v: boolean) {
    setMineSetting(v)
    try {
      localStorage.setItem(MINE_KEY, v ? '1' : '0')
    } catch {
      // Not remembered; the toggle still works for this visit.
    }
  }

  // Adult titles show up only when they are on the viewer's list.
  const shown = (q.data ?? []).filter((a) => {
    const entry = onList.get(a.media.id)
    if (a.media.isAdult && !entry) return false
    return !mine || (entry && FOLLOWING.has(entry.status))
  })
  const perDay = days.slice(0, 7).map((d, i) =>
    shown.filter((a) => a.airingAt * 1000 >= d.getTime() && a.airingAt * 1000 < days[i + 1].getTime()),
  )
  const range = `${fmtDay(days[0])} – ${fmtDay(days[6])}`

  return (
    <section className="mt-4">
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-2">
        <h1 className="font-display text-3xl font-bold">
          Schedule <span className="text-xl font-semibold text-muted">{range}</span>
        </h1>
        <div className="flex items-center gap-1 text-sm">
          <button type="button" onClick={() => setParams(week - 1 ? { week: String(week - 1) } : {})} className="min-h-11 px-2 text-muted hover:text-ink">
            Previous week
          </button>
          {week !== 0 && (
            <button type="button" onClick={() => setParams({})} className="min-h-11 px-2 text-muted hover:text-ink">
              This week
            </button>
          )}
          <button type="button" onClick={() => setParams(week + 1 ? { week: String(week + 1) } : {})} className="min-h-11 px-2 text-muted hover:text-ink">
            Next week
          </button>
        </div>
      </div>

      {loggedIn && (
        <div role="radiogroup" aria-label="Which shows" className="mt-3 inline-flex rounded-md border border-line p-0.5 text-sm">
          {[
            { v: true, label: 'On my list' },
            { v: false, label: 'Everything' },
          ].map((o) => (
            <button
              key={o.label}
              type="button"
              role="radio"
              aria-checked={mine === o.v}
              onClick={() => setMine(o.v)}
              className={`min-h-10 rounded px-3 ${mine === o.v ? 'bg-line text-ink' : 'text-muted hover:text-ink'}`}
            >
              {o.label}
            </button>
          ))}
        </div>
      )}

      <div role="tablist" aria-label="Day" className="mt-4 grid grid-cols-7 gap-1">
        {days.slice(0, 7).map((d, i) => (
          <button
            key={i}
            type="button"
            role="tab"
            aria-selected={day === i}
            onClick={() => setPicked({ week, day: i })}
            className={`flex min-h-14 flex-col items-center justify-center rounded-md border ${
              day === i ? 'border-ink' : 'border-line text-muted hover:border-muted'
            }`}
          >
            <span className="text-xs">{d.toLocaleDateString(undefined, { weekday: 'short' })}</span>
            <span className="font-display text-xl leading-none font-semibold">{d.getDate()}</span>
            {q.data && <span className="text-[0.7rem] text-muted">{perDay[i].length || '–'}</span>}
          </button>
        ))}
      </div>

      <div role="tabpanel" className="mt-4">
        {q.isPending && <Loading what="the airing schedule" />}
        {q.isError && <ErrorBox title="Could not load the schedule from AniList." error={q.error} onRetry={() => q.refetch()} />}
        {q.data && perDay[day].length === 0 && (
          <Empty>
            {mine
              ? `Nothing you are watching or planning airs on ${fmtWeekday(days[day])}. Pick Everything to see the full schedule.`
              : `AniList lists nothing airing on ${fmtWeekday(days[day])}.`}
          </Empty>
        )}
        {q.data && perDay[day].length > 0 && (
          <ol className="divide-y divide-line">
            {perDay[day].map((a) => (
              <AiringRow key={`${a.media.id}-${a.episode}`} a={a} entry={onList.get(a.media.id)} />
            ))}
          </ol>
        )}
      </div>
    </section>
  )
}

function AiringRow({ a, entry }: { a: Airing; entry: ListItem | undefined }) {
  const m = a.media
  const aired = a.airingAt * 1000 <= Date.now()
  // The source has it, and it is the viewer's next episode or later.
  const ready = aired && entry?.available != null && a.episode <= entry.available
  const unwatched = entry !== undefined && a.episode > entry.progress
  const time = new Date(a.airingAt * 1000).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false })

  return (
    <li className="flex items-center gap-3 py-3">
      <time
        dateTime={new Date(a.airingAt * 1000).toISOString()}
        className={`w-12 shrink-0 font-display text-xl font-semibold ${aired ? 'text-muted' : ''}`}
      >
        {time}
      </time>
      <Link to={`/anime/${m.id}`} className="shrink-0">
        <img src={m.coverImage.large} alt="" loading="lazy" className="aspect-[2/3] w-11 rounded object-cover" />
      </Link>
      <div className="min-w-0 flex-1">
        <Link to={`/anime/${m.id}`} className="line-clamp-2 leading-snug hover:underline">
          {titleOf(m)}
        </Link>
        <p className="text-xs text-muted">
          {entry ? `${STATUS_LABEL[entry.status]} · ${entry.progress} watched` : m.format?.replace('_', ' ')}
          {!aired && ` · ${untilText(a.airingAt)}`}
        </p>
      </div>
      {ready && unwatched ? (
        <Link
          to={`/watch/${m.id}/${a.episode}`}
          className="inline-flex min-h-11 shrink-0 items-baseline gap-1.5 rounded-md bg-accent px-3 py-2 text-bg hover:brightness-110"
        >
          <span className="text-sm font-medium">Play</span>
          <span className="font-display text-xl leading-none font-bold">EP {a.episode}</span>
        </Link>
      ) : (
        <span className="shrink-0 rounded-md border border-line px-2.5 py-1 font-display text-lg leading-none font-semibold text-muted">
          EP {a.episode}
          {m.episodes ? <span className="text-sm font-normal">/{m.episodes}</span> : null}
        </span>
      )}
    </li>
  )
}

function untilText(unix: number): string {
  const min = Math.round((unix * 1000 - Date.now()) / 60_000)
  if (min < 60) return `in ${min} min`
  const h = Math.round(min / 60)
  return h < 48 ? `in ${h} h` : `in ${Math.round(h / 24)} days`
}

function fmtDay(d: Date) {
  return d.toLocaleDateString(undefined, { day: 'numeric', month: 'short' })
}

function fmtWeekday(d: Date) {
  return d.toLocaleDateString(undefined, { weekday: 'long' })
}
