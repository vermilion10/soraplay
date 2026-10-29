import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, titleOf, type SavedEpisode } from '../lib/api'
import { Empty, ErrorBox, Loading } from '../components/States'

export default function Saved() {
  const q = useQuery({ queryKey: ['saved'], queryFn: api.saved })

  return (
    <section className="mt-4 max-w-3xl">
      <h1 className="font-display text-3xl font-bold">Saved episodes</h1>
      {q.isPending && <Loading what="saved episodes" />}
      {q.isError && <ErrorBox title="Could not read the saved episodes." error={q.error} onRetry={() => q.refetch()} />}
      {q.data && !q.data.enabled && (
        <Empty>Saving episodes ahead is off on this server. Set PREFETCH_DIR to a folder to turn it on.</Empty>
      )}
      {q.data?.enabled && <Overview {...q.data} />}
    </section>
  )
}

function Overview({ items, usedBytes = 0, maxBytes, ahead }: { items: SavedEpisode[]; usedBytes?: number; maxBytes: number; ahead: number }) {
  const shows = new Map<number, SavedEpisode[]>()
  for (const it of items) shows.set(it.media.id, [...(shows.get(it.media.id) ?? []), it])
  const pct = maxBytes ? Math.min(100, (usedBytes / maxBytes) * 100) : 0

  return (
    <>
      <p className="mt-2 max-w-prose text-sm text-muted">
        The server keeps the next {ahead} unwatched episodes of shows you watched in the last two weeks, so they start
        at once and play even when the source is down. Episodes are deleted once you watch past them. Adult titles are
        never saved.
      </p>
      <div className="mt-5">
        <p className="text-sm">
          {items.length} {items.length === 1 ? 'episode' : 'episodes'}, {size(usedBytes)} of {size(maxBytes)}
        </p>
        <div
          role="meter"
          aria-label="Space used"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={Math.round(pct)}
          className="mt-2 h-2 overflow-hidden rounded-full bg-line"
        >
          <div className="h-full bg-ink" style={{ width: `${pct}%` }} />
        </div>
      </div>

      {items.length === 0 ? (
        <Empty>Nothing is saved right now. The server checks once an hour.</Empty>
      ) : (
        <ul className="mt-6 divide-y divide-line">
          {[...shows.values()].map((eps) => {
            const m = eps[0].media
            return (
              <li key={m.id} className="flex gap-3 py-3">
                <Link to={`/anime/${m.id}`} className="shrink-0">
                  {m.coverImage?.large && <img src={m.coverImage.large} alt="" className="aspect-[2/3] w-12 rounded object-cover" />}
                </Link>
                <div className="min-w-0 flex-1">
                  <Link to={`/anime/${m.id}`} className="leading-snug hover:underline">
                    {m.title ? titleOf(m) : `AniList #${m.id}`}
                  </Link>
                  <ul className="mt-1 flex flex-wrap gap-2">
                    {eps
                      .sort((a, b) => a.episode - b.episode)
                      .map((e) => (
                        <li key={e.episode}>
                          <Link
                            to={`/watch/${m.id}/${e.episode}`}
                            className="inline-flex min-h-11 items-baseline gap-1.5 rounded-md border border-line px-3 py-2 hover:border-muted"
                          >
                            <span className="font-display text-lg leading-none font-semibold">EP {e.episode}</span>
                            <span className="text-xs text-muted">
                              {size(e.bytes)} · saved {ago(e.savedAt)}
                            </span>
                          </Link>
                        </li>
                      ))}
                  </ul>
                </div>
              </li>
            )
          })}
        </ul>
      )}
    </>
  )
}

function size(bytes: number): string {
  if (bytes >= 1 << 30) return `${(bytes / (1 << 30)).toFixed(1)} GB`
  return `${Math.round(bytes / (1 << 20))} MB`
}

function ago(unix: number): string {
  const h = Math.round((Date.now() / 1000 - unix) / 3600)
  if (h < 1) return 'just now'
  if (h < 48) return `${h} h ago`
  return `${Math.round(h / 24)} days ago`
}
