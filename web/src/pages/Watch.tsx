import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import Hls from 'hls.js'
import { api, clock, resumable, titleOf, type Interval, type Position, type SkipTimes } from '../lib/api'
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
            resume={stream.data.resume}
            onExpired={() => stream.refetch()}
          />
        )}
      </div>

      <div className="mt-4 flex flex-wrap items-end gap-x-4 gap-y-2">
        <div className="mr-auto min-w-0">
          {m ? (
            <Link to={`/anime/${id}`} className="flex min-h-11 items-center text-muted hover:text-ink">
              <span className="truncate">{titleOf(m)}</span>
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
        Keys: Space play/pause · ← → seek · F fullscreen · S skip intro or ending{next !== undefined && ' · N next episode'}
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
  resume,
  onExpired,
}: {
  src: string
  animeId: number
  ep: number
  nextEp?: number
  syncProgress: boolean
  resume: Position | null
  onExpired: () => void
}) {
  const video = useRef<HTMLVideoElement>(null)
  const sent = useRef(false)
  const lastSaved = useRef(0)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [resumedAt, setResumedAt] = useState<number | null>(null)
  const [skip, setSkip] = useState<SkipTimes | null>(null)
  const [inPart, setInPart] = useState<'op' | 'ed' | null>(null)
  const qc = useQueryClient()
  const navigate = useNavigate()
  // Held in a ref: the parent passes a new function on every render, and the
  // player must not be rebuilt each time.
  const expired = useRef(onExpired)
  expired.current = onExpired

  useEffect(() => {
    const v = video.current
    if (!v) return
    if (Hls.isSupported()) {
      const hls = new Hls()
      hls.on(Hls.Events.ERROR, (_, d) => {
        if (!d.fatal) return
        if (d.type === Hls.ErrorTypes.NETWORK_ERROR && d.response?.code === 403) {
          expired.current() // signed link ran out; fetch a fresh one
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
  }, [src])

  useEffect(() => {
    const v = video.current
    if (!v) return
    v.play().catch(() => {}) // autoplay can be refused; the controls are there
  }, [src])

  // Saves where playback is, so this episode resumes there on any device.
  function savePosition(keepalive = false) {
    const v = video.current
    if (!v || !v.duration || v.currentTime < 1) return
    lastSaved.current = v.currentTime
    api.savePosition(animeId, ep, v.currentTime, v.duration, keepalive).catch(() => {})
  }
  const saveRef = useRef(savePosition)
  saveRef.current = savePosition

  useEffect(() => {
    const onHide = () => saveRef.current(true)
    window.addEventListener('pagehide', onHide)
    return () => {
      window.removeEventListener('pagehide', onHide)
      saveRef.current(true) // leaving the player, e.g. for the next episode
    }
  }, [])

  function onLoaded() {
    const v = video.current
    if (!v) return
    api.skipTimes(animeId, ep, v.duration).then(setSkip).catch(() => {})
    if (!resumable(resume)) return
    v.currentTime = resume.position
    lastSaved.current = resume.position
    setResumedAt(resume.position)
  }

  function onTime() {
    const v = video.current
    if (!v) return
    if (Math.abs(v.currentTime - lastSaved.current) >= 10) savePosition()
    const within = (iv: Interval | null) => !!iv && v.currentTime >= iv.start && v.currentTime < iv.end - 2
    setInPart(within(skip?.op ?? null) ? 'op' : within(skip?.ed ?? null) ? 'ed' : null)
    if (sent.current || !v.duration || v.currentTime / v.duration < WATCHED_AT) return
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

  // Skipping the ending goes to the next episode when there is one.
  function skipPart() {
    const v = video.current
    if (!v || !inPart || !skip) return
    if (inPart === 'ed' && nextEp !== undefined) {
      navigate(`/watch/${animeId}/${nextEp}`)
      return
    }
    const iv = inPart === 'op' ? skip.op : skip.ed
    if (iv) v.currentTime = iv.end
    setInPart(null)
  }
  const skipRef = useRef(skipPart)
  skipRef.current = skipPart

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const v = video.current
      const t = e.target instanceof Element ? e.target : null
      if (!v || e.defaultPrevented || e.ctrlKey || e.metaKey || e.altKey) return
      if (t?.closest('input, textarea, button, a, dialog')) return
      // A focused <video> already handles Space and the arrows natively;
      // handling them too would undo the toggle or double the seek.
      if (t === v && [' ', 'ArrowLeft', 'ArrowRight'].includes(e.key)) return
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
        case 's':
          skipRef.current()
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
        <div className="relative">
        <video
          ref={video}
          controls
          playsInline
          onLoadedMetadata={onLoaded}
          onTimeUpdate={onTime}
          onPause={() => savePosition()}
          className="aspect-video w-full bg-black"
        />
        {inPart && (
          <button
            type="button"
            onClick={skipPart}
            className="absolute right-3 bottom-16 min-h-11 rounded-md border border-line bg-surface/90 px-4 text-sm font-medium hover:border-muted"
          >
            {inPart === 'op' ? 'Skip intro' : nextEp !== undefined ? 'Next episode' : 'Skip ending'}
          </button>
        )}
        </div>
      )}
      {resumedAt !== null && (
        <div className="flex flex-wrap items-center gap-x-3 px-4 py-1 text-sm text-muted sm:px-3">
          Resumed at {clock(resumedAt)}
          <button
            type="button"
            onClick={() => {
              const v = video.current
              if (v) v.currentTime = 0
              setResumedAt(null)
            }}
            className="min-h-11 text-ink underline"
          >
            Start over
          </button>
        </div>
      )}
      <p role="status" aria-live="polite" className="sr-only sm:not-sr-only sm:px-3 sm:py-1 sm:text-xs sm:text-muted">
        {status}
      </p>
    </div>
  )
}
