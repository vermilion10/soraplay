import { useState } from 'react'
import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, type Rating } from '../lib/api'

// Bars for the viewer's own episode ratings, one slot per episode on a fixed
// 0-10 scale, so unrated episodes show as gaps. Hover, focus or tap a bar to
// read it; the list below is the same data as text.
export function RatingsChart({ animeId, episodes }: { animeId: number; episodes: number }) {
  const q = useQuery({ queryKey: ['ratings', animeId], queryFn: () => api.ratings(animeId) })
  const [picked, setPicked] = useState<number | null>(null)
  const rs = q.data ?? []
  if (rs.length === 0) return null

  const byEp = new Map(rs.map((r) => [r.episode, r]))
  const lastRated = Math.max(...rs.map((r) => r.episode))
  // Whole-numbered slots up to the last rated episode, or the whole season when it is short.
  const slots = Math.ceil(episodes > 0 && episodes <= 100 ? Math.max(episodes, lastRated) : lastRated)
  const step = Math.ceil(slots / 12)
  const avg = rs.reduce((s, r) => s + r.score, 0) / rs.length
  const best = rs.reduce((a, b) => (b.score > a.score ? b : a))
  const shown = picked !== null ? byEp.get(picked) : undefined

  return (
    <section className="mt-8 max-w-3xl">
      <h2 className="font-display text-2xl font-semibold">Your episode ratings</h2>
      {/* The readout doubles as the tooltip, so it works the same on touch screens. */}
      <p aria-live="polite" className="mt-1 min-h-11 text-sm text-muted">
        {shown ? (
          <Readout r={shown} animeId={animeId} />
        ) : (
          <>
            Average <span className="text-ink">{avg.toFixed(1)}</span> across {rs.length} rated{' '}
            {rs.length === 1 ? 'episode' : 'episodes'} · highest EP {best.episode} ({best.score})
          </>
        )}
      </p>

      <div className="relative mt-2 overflow-x-auto pb-1">
        <div className="relative pl-6" style={{ minWidth: `${slots * 10 + 24}px` }}>
          {/* Recessive reference lines at 5 and 10 on the one shared scale. */}
          {[10, 5].map((v) => (
            <div key={v} className="pointer-events-none absolute right-0 left-6 border-t border-dashed border-line" style={{ bottom: `calc(${v * 10}% * 0.8 + 1.5rem)` }}>
              <span className="absolute -top-2 -left-6 w-5 text-right text-[0.7rem] text-muted">{v}</span>
            </div>
          ))}
          {/* Positioned so the bars paint over the reference lines. */}
          <ol className="relative flex h-40 items-end gap-0.5" onMouseLeave={() => setPicked(null)}>
            {Array.from({ length: slots }, (_, i) => i + 1).map((ep) => {
              const r = byEp.get(ep)
              const on = picked === ep
              return (
                <li key={ep} className="flex h-full min-w-2 flex-1 flex-col justify-end">
                  <div className="flex h-[80%] items-end">
                    {r ? (
                      <button
                        type="button"
                        aria-label={`Episode ${ep}: ${r.score} out of 10${r.note ? `, ${r.note}` : ''}`}
                        onMouseEnter={() => setPicked(ep)}
                        onFocus={() => setPicked(ep)}
                        onClick={() => setPicked(ep)}
                        // The hit area is the whole column, taller than the bar.
                        className="group flex h-full w-full items-end"
                      >
                        <span
                          className={`block w-full rounded-t ${on ? 'bg-ink' : 'bg-muted group-hover:bg-ink'}`}
                          style={{ height: `${r.score * 10}%` }}
                        />
                      </button>
                    ) : (
                      <span className="block h-0.5 w-full bg-line" title={`Episode ${ep}: not rated`} />
                    )}
                  </div>
                  <span className="mt-1 h-5 text-center text-[0.7rem] leading-5 text-muted">
                    {(ep - 1) % step === 0 ? ep : ''}
                  </span>
                </li>
              )
            })}
          </ol>
        </div>
      </div>

      <details className="mt-3 text-sm">
        <summary className="min-h-11 cursor-pointer py-2 text-muted hover:text-ink">All ratings as a list</summary>
        <table className="mt-1 w-full text-left">
          <thead className="text-xs text-muted">
            <tr>
              <th className="py-1 pr-4 font-normal">Episode</th>
              <th className="py-1 pr-4 font-normal">Score</th>
              <th className="py-1 font-normal">Note</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-line">
            {rs.map((r) => (
              <tr key={r.episode}>
                <td className="py-2 pr-4">
                  <Link to={`/watch/${animeId}/${r.episode}`} className="underline">
                    EP {r.episode}
                  </Link>
                </td>
                <td className="py-2 pr-4 font-display text-lg font-semibold">{r.score}</td>
                <td className="py-2 text-muted">{r.note}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </details>
    </section>
  )
}

function Readout({ r, animeId }: { r: Rating; animeId: number }) {
  return (
    <>
      <span className="font-display text-lg font-semibold text-ink">EP {r.episode}</span> · {r.score}/10
      {r.note && <> · “{r.note}”</>} ·{' '}
      <Link to={`/watch/${animeId}/${r.episode}`} className="underline hover:text-ink">
        Watch it
      </Link>
    </>
  )
}
