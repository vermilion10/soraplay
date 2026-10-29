import { Link } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api, clock, nextEpisode, resumable, titleOf, type ListItem, type Me } from '../lib/api'
import { PosterGrid } from '../components/PosterGrid'
import { Empty, ErrorBox, Loading } from '../components/States'

export default function Home() {
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  if (me.isPending) return <Loading what="your session" />
  if (me.isError) return <ErrorBox title="Could not reach the server." error={me.error} onRetry={() => me.refetch()} />

  return (
    <>
      {me.data.user ? <Watching /> : <LoggedOut me={me.data} />}
      <Seasonal />
    </>
  )
}

function LoggedOut({ me }: { me: Me }) {
  return (
    <section className="my-6 rounded-md bg-surface p-5">
      <h1 className="font-display text-3xl font-bold">Your AniList, ready to play</h1>
      {me.configured ? (
        <>
          <p className="mt-2 max-w-prose text-muted">
            Log in to see your Watching list here. Episodes you finish are marked as watched on AniList.
          </p>
          <a
            href="/auth/login"
            className="mt-4 inline-flex min-h-11 items-center rounded-md bg-accent px-5 font-medium text-bg hover:brightness-110"
          >
            Log in with AniList
          </a>
        </>
      ) : (
        <p className="mt-2 max-w-prose text-muted">
          AniList login is not set up on this server. Set <code className="text-ink">ANILIST_CLIENT_ID</code> and{' '}
          <code className="text-ink">ANILIST_CLIENT_SECRET</code>, then restart it. You can still search and watch.
        </p>
      )}
    </section>
  )
}

function Watching() {
  const list = useQuery({ queryKey: ['list'], queryFn: api.list })
  if (list.isPending) return <Loading what="your Watching list" />
  if (list.isError)
    return <ErrorBox title="Could not load your Watching list." error={list.error} onRetry={() => list.refetch()} />

  const [first, ...rest] = list.data
  // New means it arrived this week and is unwatched; an older backlog is not news.
  const hasNew = (it: ListItem) =>
    it.available !== null &&
    it.available > it.progress &&
    it.arrivedAt !== null &&
    Date.now() / 1000 - it.arrivedAt < NEW_FOR
  const fresh = rest.filter(hasNew)
  const others = rest.filter((it) => !hasNew(it))
  if (!first)
    return (
      <Empty>
        Nothing on your Watching list yet. <Link to="/search" className="text-ink underline">Search for a show</Link>,
        play the first episode, and it is added to your list when you finish it.
      </Empty>
    )

  return (
    <section className="my-4">
      <h2 className="sr-only">Continue watching</h2>
      <ContinueCard item={first} />
      {fresh.length > 0 && (
        <div className="mt-8">
          <h2 className="mb-3 font-display text-xl font-semibold">New episodes</h2>
          <PosterGrid items={fresh.map((it) => ({ media: it.media, note: newNote(it) }))} />
        </div>
      )}
      {others.length > 0 && (
        <div className="mt-8">
          <h2 className="mb-3 font-display text-xl font-semibold text-muted">Also watching</h2>
          <PosterGrid items={others.map((it) => ({ media: it.media, note: progressNote(it) }))} />
        </div>
      )}
    </section>
  )
}

const NEW_FOR = 7 * 24 * 3600 // seconds an episode counts as new

function newNote(it: ListItem): string {
  const n = Math.floor(it.available! - it.progress)
  return n === 1 ? `EP ${it.available} is up` : `${n} new, up to EP ${it.available}`
}

function progressNote(it: ListItem): string {
  const total = it.media.episodes || '?'
  return `${it.progress} / ${total} watched`
}

// The focal point of the home screen: the show watched most recently, with
// its next episode set large (the "ticket stub" motif in DESIGN.md).
function ContinueCard({ item }: { item: ListItem }) {
  const m = item.media
  const next = nextEpisode(m, item.progress)
  // An episode past the AniList progress that was left halfway, maybe on another device.
  const resume = resumable(item.resume) && item.resume.episode > item.progress ? item.resume : null
  const notOutYet = next !== null && m.nextAiringEpisode !== null && next >= m.nextAiringEpisode.episode
  // Aired, but the source has not uploaded it yet (as of the server's last check).
  const notUpYet = !notOutYet && next !== null && item.available !== null && next > item.available
  const tint = m.coverImage.color || '#3a302a'

  return (
    <article
      className="relative overflow-hidden rounded-lg"
      style={{ background: `linear-gradient(110deg, ${tint}55, var(--color-surface) 60%)` }}
    >
      <div className="flex gap-4 p-4 sm:gap-6 sm:p-6">
        <Link to={`/anime/${m.id}`} className="w-24 shrink-0 sm:w-36">
          <img src={m.coverImage.large} alt="" className="aspect-[2/3] w-full rounded-md object-cover" />
        </Link>
        <div className="flex min-w-0 flex-col">
          <p className="text-sm text-muted">Continue watching</p>
          <Link to={`/anime/${m.id}`} className="mt-1 font-display text-2xl leading-tight font-bold sm:text-4xl">
            {titleOf(m)}
          </Link>
          <p className="mt-1 text-sm text-muted">{progressNote(item)}</p>
          <div className="mt-auto pt-4">
            {resume ? (
              <Link
                to={`/watch/${m.id}/${resume.episode}`}
                className="inline-flex min-h-11 items-baseline gap-2 rounded-md bg-accent px-5 py-2 text-bg hover:brightness-110"
              >
                <span className="text-sm font-medium">Resume</span>
                <span className="font-display text-2xl leading-none font-bold">EP {resume.episode}</span>
                <span className="text-sm">at {clock(resume.position)}</span>
              </Link>
            ) : next === null ? (
              <p className="text-sm">All episodes watched.</p>
            ) : notUpYet ? (
              <p className="text-sm">Episode {next} has aired but is not up on the source yet.</p>
            ) : notOutYet ? (
              <p className="text-sm">
                Episode {next} airs {airsIn(m.nextAiringEpisode!.airingAt)}.
              </p>
            ) : (
              <Link
                to={`/watch/${m.id}/${next}`}
                className="inline-flex min-h-11 items-baseline gap-2 rounded-md bg-accent px-5 py-2 text-bg hover:brightness-110"
              >
                <span className="text-sm font-medium">Play</span>
                <span className="font-display text-2xl leading-none font-bold">EP {next}</span>
              </Link>
            )}
          </div>
        </div>
      </div>
    </article>
  )
}

function airsIn(unix: number): string {
  const hours = Math.round((unix * 1000 - Date.now()) / 3_600_000)
  if (hours < 1) return 'within the hour'
  if (hours < 48) return `in ${hours} hours`
  return `in ${Math.round(hours / 24)} days`
}

function Seasonal() {
  const q = useQuery({ queryKey: ['seasonal'], queryFn: api.seasonal, staleTime: 3_600_000 })
  return (
    <section className="mt-10">
      <h2 className="mb-3 font-display text-xl font-semibold text-muted">
        {q.data ? `${cap(q.data.season)} ${q.data.year}` : 'This season'}
      </h2>
      {q.isPending && <Loading what="this season's anime" />}
      {q.isError && <ErrorBox title="Could not load this season from AniList." error={q.error} onRetry={() => q.refetch()} />}
      {q.data && q.data.media.length === 0 && <Empty>AniList has no anime listed for this season yet.</Empty>}
      {q.data && q.data.media.length > 0 && <PosterGrid items={q.data.media.map((media) => ({ media }))} />}
    </section>
  )
}

function cap(s: string) {
  return s.charAt(0) + s.slice(1).toLowerCase()
}
