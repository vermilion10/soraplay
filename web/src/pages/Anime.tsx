import { useState } from 'react'
import { Link, useParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, plainText, titleOf, type AnimeDetail } from '../lib/api'
import { SourcePicker } from '../components/SourcePicker'
import { Empty, ErrorBox, Loading } from '../components/States'

export default function Anime() {
  const id = Number(useParams().id)
  const q = useQuery({ queryKey: ['anime', id], queryFn: () => api.anime(id), enabled: id > 0 })

  if (!(id > 0)) return <Empty>That is not a valid anime link.</Empty>
  if (q.isPending) return <Loading what="the anime and its episodes" />
  if (q.isError) return <ErrorBox title="Could not load this anime." error={q.error} onRetry={() => q.refetch()} />
  return <Detail data={q.data} />
}

function Detail({ data }: { data: AnimeDetail }) {
  const { media: m, mapping, episodes } = data
  const [picker, setPicker] = useState(false)
  const [more, setMore] = useState(false)
  const progress = m.mediaListEntry?.progress ?? 0
  const available = new Set(episodes.map((e) => e.number))
  const next = episodes.find((e) => e.number > progress)?.number ?? episodes[0]?.number
  const tint = m.coverImage.color || '#3a302a'
  const meta = [
    m.format?.replace('_', ' '),
    m.seasonYear,
    m.episodes ? `${m.episodes} episodes` : null,
    m.status === 'RELEASING' ? 'Airing' : m.status === 'FINISHED' ? 'Finished' : null,
  ].filter(Boolean)
  const desc = m.description ? plainText(m.description) : ''

  return (
    <article>
      {/* The cover color tints the top of the page, so each show looks like its own art. */}
      <div
        className="-mx-4 px-4 pt-4 pb-6 sm:-mx-6 sm:rounded-b-lg sm:px-6"
        style={{ background: `linear-gradient(180deg, ${tint}66, transparent)` }}
      >
        <div className="flex gap-4 sm:gap-6">
          <img
            src={m.coverImage.large}
            alt=""
            className="aspect-[2/3] w-28 shrink-0 self-start rounded-md object-cover sm:w-44"
          />
          <div className="min-w-0">
            <h1 className="font-display text-3xl leading-tight font-bold sm:text-5xl">{titleOf(m)}</h1>
            {m.title.english && m.title.english !== titleOf(m) && (
              <p className="mt-1 text-muted">{m.title.english}</p>
            )}
            <p className="mt-2 text-sm text-muted">{meta.join(' · ')}</p>
            {m.mediaListEntry && (
              <p className="mt-1 text-sm">
                On your list: {progress} / {m.episodes || '?'} watched
              </p>
            )}
            {next !== undefined && (
              <Link
                to={`/watch/${m.id}/${next}`}
                className="mt-4 inline-flex min-h-11 items-baseline gap-2 rounded-md bg-accent px-5 py-2 text-bg hover:brightness-110"
              >
                <span className="text-sm font-medium">Play</span>
                <span className="font-display text-2xl leading-none font-bold">EP {next}</span>
              </Link>
            )}
          </div>
        </div>
      </div>

      {desc && (
        <section className="mt-4 max-w-prose">
          <h2 className="sr-only">Synopsis</h2>
          <p className={`text-sm leading-relaxed whitespace-pre-line text-muted ${more ? '' : 'line-clamp-3'}`}>{desc}</p>
          <button
            type="button"
            onClick={() => setMore(!more)}
            aria-expanded={more}
            className="min-h-11 text-sm text-ink underline"
          >
            {more ? 'Show less' : 'Show more'}
          </button>
        </section>
      )}

      <section className="mt-6">
        <div className="flex flex-wrap items-baseline justify-between gap-2">
          <h2 className="font-display text-2xl font-semibold">Episodes</h2>
          <p className="text-sm text-muted">
            {mapping ? <>Source A: {mapping.title}</> : 'Not linked to Source A'}
            <button
              type="button"
              onClick={() => setPicker(true)}
              className="ml-2 min-h-11 px-1 text-ink underline"
            >
              {mapping ? 'Change' : 'Find it'}
            </button>
          </p>
        </div>

        {data.sourceError && <ErrorBox title="Source A did not respond as expected." error={data.sourceError} />}
        {!mapping && !data.sourceError && (
          <Empty>
            {data.candidates && data.candidates.length > 0
              ? 'No confident match on Source A. Use "Find it" to pick the right show.'
              : 'This anime was not found on Source A. It may not be uploaded yet, or it is listed under another title: use "Find it" to search by hand.'}
          </Empty>
        )}
        {mapping && episodes.length === 0 && !data.sourceError && (
          <Empty>No episodes are up on Source A yet.</Empty>
        )}
        {episodes.length > 0 && (
          <ol className="mt-3 grid grid-cols-[repeat(auto-fill,minmax(3.25rem,1fr))] gap-2">
            {episodes.map((e) => {
              const watched = e.number <= progress
              const isNext = e.number === next
              return (
                <li key={e.number}>
                  <Link
                    to={`/watch/${m.id}/${e.number}`}
                    aria-label={`Episode ${e.number}${watched ? ', watched' : ''}`}
                    className={`flex min-h-11 items-center justify-center rounded-md border font-display text-lg font-semibold ${
                      isNext
                        ? 'border-accent text-accent'
                        : watched
                          ? 'border-transparent bg-surface text-muted'
                          : 'border-line hover:border-muted'
                    }`}
                  >
                    {e.number}
                  </Link>
                </li>
              )
            })}
          </ol>
        )}
        {m.nextAiringEpisode && !available.has(m.nextAiringEpisode.episode) && (
          <p className="mt-3 text-sm text-muted">
            Episode {m.nextAiringEpisode.episode} airs{' '}
            {new Date(m.nextAiringEpisode.airingAt * 1000).toLocaleString(undefined, {
              weekday: 'long',
              hour: '2-digit',
              minute: '2-digit',
            })}
            .
          </p>
        )}
      </section>

      <SourcePicker
        animeId={m.id}
        defaultQuery={m.title.romaji}
        current={mapping}
        open={picker}
        onClose={() => setPicker(false)}
      />
    </article>
  )
}
