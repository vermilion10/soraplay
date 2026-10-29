import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import Hls from 'hls.js'
import { api, titleOf } from '../lib/api'
import { Empty, ErrorBox, Loading } from '../components/States'

// Share of an episode that counts as watched. Past this point the end credits
// are usually running.
const WATCHED_AT = 0.85

export default function Watch() {
  const params = useParams()
  const id = Number(params.id)
  const ep = Number(params.ep)
  const anime = useQuery({ queryKey: ['anime', id], queryFn: () => api.anime(id), enabled: id > 0 })
  // Stream links expire after a few hours, so never reuse an old one.
  const stream = useQuery({
    queryKey: ['stream', id, ep],
    queryFn: () => api.stream(id, ep),
    enabled: id > 0 && ep > 0,
    staleTime: 0,
    gcTime: 0,
  })
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })

  if (!(id > 0 && ep > 0)) return <Empty>That is not a valid episode link.</Empty>

  const m = anime.data?.media
  const eps = anime.data?.episodes ?? []
  const idx = eps.findIndex((e) => e.number === ep)
  const prev = idx > 0 ? eps[idx - 1].number : undefined
  const next = idx >= 0 && idx < eps.length - 1 ? eps[idx + 1].number : undefined

  return (
    <article className="mt-2">
      <div className="-mx-4 bg-black sm:mx-0 sm:overflow-hidden sm:rounded-lg">
        {stream.isPending && (
          <div className="flex aspect-video items-center justify-center">
            <Loading what={`episode ${ep}`} />
          </div>
        )}
        {stream.isError && (
          <div className="flex aspect-video items-center justify-center p-4">
            <ErrorBox title={`Episode ${ep} could not be loaded.`} error={stream.error} onRetry={() => stream.refetch()} />
          </div>
        )}
        {stream.data && (
          <Player
            key={stream.data.streams[0].url}
            src={stream.data.streams[0].url}
            animeId={id}
            ep={ep}
            nextEp={next}
            syncProgress={!!me.data?.user}
            onExpired={() => stream.refetch()}
          />
        )}
      </div>

      <div className="mt-4 flex flex-wrap items-end gap-x-4 gap-y-2">
        <div className="mr-auto min-w-0">
          {m ? (
            <Link to={`/anime/${id}`} className="block truncate text-muted hover:text-ink">
              {titleOf(m)}
            </Link>
          ) : (
            <span className="text-muted">{anime.isError ? 'Unknown anime' : 'Loading…'}</span>
          )}
          <h1 className="font-display text-4xl leading-none font-bold">EP {ep}</h1>
        </div>
        <nav aria-label="Episodes" className="flex gap-2">
          {prev !== undefined && (
            <Link to={`/watch/${id}/${prev}`} className="inline-flex min-h-11 items-center rounded-md border border-line px-4 hover:border-muted">
              Previous
            </Link>
          )}
          {next !== undefined && (
            <Link to={`/watch/${id}/${next}`} className="inline-flex min-h-11 items-center rounded-md border border-line px-4 hover:border-muted">
              Next episode
            </Link>
          )}
        </nav>
      </div>
      {stream.data && (
        <p className="mt-2 text-xs text-muted">
          Indonesian subtitles (burned in) · server {stream.data.streams[0].server}
        </p>
      )}
      <p className="mt-4 text-xs text-muted">
        Keys: Space play/pause · ← → 10 seconds · F fullscreen{next !== undefined && ' · N next episode'}
      </p>
    </article>
  )
}

function Player({
  src,
  animeId,
  ep,
  nextEp,
  syncProgress,
  onExpired,
}: {
  src: string
  animeId: number
  ep: number
  nextEp?: number
  syncProgress: boolean
  onExpired: () => void
}) {
  const video = useRef<HTMLVideoElement>(null)
  const sent = useRef(false)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const qc = useQueryClient()
  const navigate = useNavigate()

  useEffect(() => {
    const v = video.current
    if (!v) return
    if (Hls.isSupported()) {
      const hls = new Hls()
      hls.on(Hls.Events.ERROR, (_, d) => {
        if (!d.fatal) return
        if (d.type === Hls.ErrorTypes.NETWORK_ERROR && d.response?.code === 403) {
          onExpired() // signed link ran out; fetch a fresh one
        } else {
          setError(`Playback failed (${d.details}).`)
          hls.destroy()
        }
      })
      hls.loadSource(src)
      hls.attachMedia(v)
      return () => hls.destroy()
    }
    v.src = src // Safari and iOS play HLS natively
  }, [src, onExpired])

  useEffect(() => {
    const v = video.current
    if (!v) return
    v.play().catch(() => {}) // autoplay can be refused; the controls are there
  }, [src])

  function onTime() {
    const v = video.current
    if (!v || sent.current || !v.duration || v.currentTime / v.duration < WATCHED_AT) return
    sent.current = true
    if (!syncProgress) {
      setStatus('Log in with AniList to mark episodes as watched.')
      return
    }
    api
      .progress(animeId, ep)
      .then(() => {
        setStatus(`Episode ${ep} marked as watched on AniList.`)
        // The server sends it in the background; refresh lists once it has.
        setTimeout(() => {
          qc.invalidateQueries({ queryKey: ['list'] })
          qc.invalidateQueries({ queryKey: ['anime', animeId] })
        }, 3000)
      })
      .catch((e: Error) => {
        sent.current = false
        setStatus(`Could not save progress: ${e.message}`)
      })
  }

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const v = video.current
      const t = e.target as HTMLElement
      if (!v || e.ctrlKey || e.metaKey || e.altKey || t.closest('input, textarea, button, a, dialog')) return
      switch (e.key) {
        case ' ':
        case 'k':
          e.preventDefault()
          if (v.paused) v.play()
          else v.pause()
          break
        case 'ArrowLeft':
          e.preventDefault()
          v.currentTime = Math.max(0, v.currentTime - 10)
          break
        case 'ArrowRight':
          e.preventDefault()
          v.currentTime = Math.min(v.duration || Infinity, v.currentTime + 10)
          break
        case 'f':
          if (document.fullscreenElement) document.exitFullscreen()
          else v.requestFullscreen?.()
          break
        case 'n':
          if (nextEp !== undefined) navigate(`/watch/${animeId}/${nextEp}`)
          break
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [animeId, nextEp, navigate])

  return (
    <div>
      {error ? (
        <div className="flex aspect-video items-center justify-center p-4">
          <ErrorBox title="The video stopped." error={error} onRetry={onExpired} />
        </div>
      ) : (
        <video
          ref={video}
          controls
          playsInline
          onTimeUpdate={onTime}
          className="aspect-video w-full bg-black"
        />
      )}
      <p role="status" aria-live="polite" className="sr-only sm:not-sr-only sm:px-3 sm:py-1 sm:text-xs sm:text-muted">
        {status}
      </p>
    </div>
  )
}
