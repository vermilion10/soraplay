import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import Hls from 'hls.js'
import type { FragmentLoaderConstructor, LoaderCallbacks, LoaderConfiguration, LoaderContext } from 'hls.js'
import { api, clock, resumable, titleOf, type Interval, type Position, type SkipTimes } from '../lib/api'
import { addCapture, grabFrame, useCaptures } from '../lib/captures'
import { repairPlaylist, toOriginal, toShortened, type Repair } from '../lib/hlsRepair'
import { Empty, ErrorBox, Loading } from '../components/States'
import { EpisodeRating } from '../components/EpisodeRating'

// Share of an episode that counts as watched. Past this point the end credits
// are usually running.
const WATCHED_AT = 0.85

export default function Watch() {
  const params = useParams()
  const id = Number(params.id)
  const ep = Number(params.ep)
  const [search, setSearch] = useSearchParams()
  // A source picked by hand; without one the server falls back by itself.
  const source = search.get('source') ?? undefined
  const watchQuery = source ? `?source=${encodeURIComponent(source)}` : ''
  const anime = useQuery({ queryKey: ['anime', id, source ?? ''], queryFn: () => api.anime(id, source), enabled: id > 0 })
  // Stream links expire after a few hours, so never reuse an old one.
  const stream = useQuery({
    queryKey: ['stream', id, ep, source ?? ''],
    queryFn: () => api.stream(id, ep, source),
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
  // The other source to offer when this one plays badly.
  const other = stream.data?.sources.find((x) => x.id !== stream.data?.source.id)
  const tryOther = other ? () => setSearch({ source: other.id }) : undefined

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
            <div>
              <ErrorBox title={`Episode ${ep} could not be loaded.`} error={stream.error} onRetry={() => stream.refetch()} />
              {source && (
                <button type="button" onClick={() => setSearch({})} className="min-h-11 text-sm underline">
                  Try the usual source instead
                </button>
              )}
            </div>
          </div>
        )}
        {stream.data && (
          <Player
            key={stream.data.streams[0].url}
            src={stream.data.streams[0].url}
            type={stream.data.streams[0].type}
            watchQuery={watchQuery}
            otherSource={other && tryOther ? { name: other.name, pick: tryOther } : undefined}
            animeId={id}
            title={m ? titleOf(m) : ''}
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
            <Link to={`/watch/${id}/${prev}${watchQuery}`} className="inline-flex min-h-11 items-center rounded-md border border-line px-4 hover:border-muted">
              Previous
            </Link>
          )}
          {next !== undefined && (
            <Link to={`/watch/${id}/${next}${watchQuery}`} className="inline-flex min-h-11 items-center rounded-md border border-line px-4 hover:border-muted">
              Next episode
            </Link>
          )}
        </nav>
      </div>
      {stream.data && (
        <p className="mt-2 flex flex-wrap items-center gap-x-2 text-xs text-muted">
          <span>
            Indonesian subtitles (burned in) · {stream.data.source.name}, server {stream.data.streams[0].server}
          </span>
          {other && tryOther && (
            <button type="button" onClick={tryOther} className="min-h-11 underline hover:text-ink">
              Play from {other.name} instead
            </button>
          )}
        </p>
      )}
      <EpisodeRating animeId={id} ep={ep} />
      <p className="mt-4 text-xs text-muted">
        Keys: Space play/pause · ← → seek · F fullscreen · C capture the scene · S skip intro or ending
        {next !== undefined && ' · N next episode'}
      </p>
    </article>
  )
}

function Player({
  src,
  type,
  watchQuery,
  otherSource,
  animeId,
  title,
  ep,
  nextEp,
  syncProgress,
  resume,
  onExpired,
}: {
  src: string
  type: string // "hls" or "mp4"
  watchQuery: string // keeps the picked source on episode links
  otherSource?: { name: string; pick: () => void }
  animeId: number
  title: string
  ep: number
  nextEp?: number
  syncProgress: boolean
  resume: Position | null
  onExpired: () => void
}) {
  const video = useRef<HTMLVideoElement>(null)
  const frame = useRef<HTMLDivElement>(null)
  const sent = useRef(false)
  const lastSaved = useRef(0)
  const [status, setStatus] = useState('')
  const [error, setError] = useState('')
  const [resumedAt, setResumedAt] = useState<number | null>(null)
  const [skip, setSkip] = useState<SkipTimes | null>(null)
  const [inPart, setInPart] = useState<'op' | 'ed' | null>(null)
  const [shot, setShot] = useState<{ ok: boolean; text: string } | null>(null)
  // Set when broken parts were cut from the playlist; at is where playback was.
  const [repair, setRepair] = useState<(Repair & { at: number }) | null>(null)
  const [checking, setChecking] = useState(false)
  const removed = repair?.removed ?? []
  const cutTotal = removed.reduce((n, r) => n + r.end - r.start, 0)
  // Positions, skip times and captures use the episode's own timeline.
  const episodeTime = (t: number) => toOriginal(t, removed)
  const [paused, setPaused] = useState(true)
  const [awake, setAwake] = useState(false)
  const [fullscreen, setFullscreen] = useState(false)
  const held = useCaptures()?.length ?? 0
  const qc = useQueryClient()
  const navigate = useNavigate()
  // Held in a ref: the parent passes a new function on every render, and the
  // player must not be rebuilt each time.
  const expired = useRef(onExpired)
  expired.current = onExpired

  useEffect(() => {
    const v = video.current
    if (!v) return
    if (type === 'hls' && Hls.isSupported()) {
      // A segment that is not video (sources sometimes serve an error page in
      // its place) means parts of the episode are broken at the source. The
      // playlist is then rebuilt without them and playback carries on from
      // the same moment; see lib/hlsRepair.
      let broken = false
      const onBroken = () => {
        if (broken) return
        broken = true
        hls.stopLoad()
        if (repair) {
          setError('Parts of this episode are broken at the source, and it would not play even without them.')
          return
        }
        const at = v.currentTime
        setChecking(true)
        repairPlaylist(src)
          .then((r) => {
            if (r.removed.length === 0) {
              URL.revokeObjectURL(r.url)
              setError('The source sent a broken part of this episode.')
            } else {
              setRepair({ ...r, at })
            }
          })
          .catch((e: Error) => setError(`The source sent a broken part, and checking it failed: ${e.message}`))
          .finally(() => setChecking(false))
      }

      const Base = Hls.DefaultConfig.loader
      class CheckedLoader extends Base {
        load(context: LoaderContext, config: LoaderConfiguration, callbacks: LoaderCallbacks<LoaderContext>) {
          super.load(context, config, {
            ...callbacks,
            onSuccess: (res, stats, ctx, details) => {
              const data = res.data
              // Video segments are binary; '<' starts an HTML error page.
              if (data instanceof ArrayBuffer && data.byteLength > 0 && new Uint8Array(data, 0, 1)[0] === 0x3c) {
                onBroken()
                return
              }
              callbacks.onSuccess(res, stats, ctx, details)
            },
          })
        }
      }

      // The base loader class is typed for any request; this one only sees fragments.
      const hls = new Hls({ fLoader: CheckedLoader as unknown as FragmentLoaderConstructor })
      hls.on(Hls.Events.ERROR, (_, d) => {
        if (!d.fatal || broken) return
        if (d.details === Hls.ErrorDetails.FRAG_PARSING_ERROR) {
          onBroken()
        } else if (d.type === Hls.ErrorTypes.NETWORK_ERROR && d.response?.code === 403) {
          expired.current() // signed link ran out; fetch a fresh one
        } else {
          setError(`Playback failed (${d.details}).`)
          hls.destroy()
        }
      })
      hls.loadSource(repair?.url ?? src)
      hls.attachMedia(v)
      return () => hls.destroy()
    }
    v.src = src // MP4 files, and HLS on Safari and iOS, play natively
  }, [src, type, repair])

  useEffect(() => () => {
    if (repair) URL.revokeObjectURL(repair.url)
  }, [repair])

  useEffect(() => {
    const v = video.current
    if (!v) return
    v.play().catch(() => {}) // autoplay can be refused; the controls are there
  }, [src, repair])

  // Saves where playback is, so this episode resumes there on any device.
  function savePosition(keepalive = false) {
    const v = video.current
    if (!v || !v.duration || v.currentTime < 1) return
    const t = episodeTime(v.currentTime)
    lastSaved.current = t
    api.savePosition(animeId, ep, t, v.duration + cutTotal, keepalive).catch(() => {})
  }
  const saveRef = useRef(savePosition)
  saveRef.current = savePosition

  useEffect(() => {
    if (!shot) return
    const t = setTimeout(() => setShot(null), 3500)
    return () => clearTimeout(t)
  }, [shot])

  useEffect(() => {
    if (!awake) return
    const t = setTimeout(() => setAwake(false), 2500)
    return () => clearTimeout(t)
  }, [awake])

  useEffect(() => {
    const on = () => setFullscreen(!!document.fullscreenElement)
    document.addEventListener('fullscreenchange', on)
    return () => document.removeEventListener('fullscreenchange', on)
  }, [])

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
    api.skipTimes(animeId, ep, v.duration + cutTotal).then(setSkip).catch(() => {})
    if (repair) {
      // The rebuilt playlist carries on where the broken one stopped.
      v.currentTime = toShortened(repair.at, removed)
      return
    }
    if (!resumable(resume)) return
    v.currentTime = resume.position
    lastSaved.current = resume.position
    setResumedAt(resume.position)
  }

  function onTime() {
    const v = video.current
    if (!v) return
    const now = episodeTime(v.currentTime)
    if (Math.abs(now - lastSaved.current) >= 10) savePosition()
    const within = (iv: Interval | null) => !!iv && now >= iv.start && now < iv.end - 2
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

  // Direct MP4 links expire after about an hour; a failure after playback
  // started fetches a fresh link, which resumes at the saved position.
  function onVideoError() {
    const v = video.current
    if (!v || type !== 'mp4') return
    if (v.currentTime > 0) {
      savePosition(true)
      expired.current()
    } else {
      setError('This server would not play the video.')
    }
  }

  // Captures the frame on screen; it is kept in this browser until saved.
  async function capture() {
    const v = video.current
    if (!v) return
    const time = episodeTime(v.currentTime)
    try {
      const blob = await grabFrame(v)
      await addCapture({
        id: crypto.randomUUID(),
        blob,
        animeId,
        title,
        episode: ep,
        time,
        capturedAt: Date.now(),
        width: v.videoWidth,
        height: v.videoHeight,
      })
      setShot({ ok: true, text: `Captured ${clock(time)}` })
    } catch (e) {
      setShot({ ok: false, text: e instanceof Error ? e.message : 'Capture failed.' })
    }
  }
  const captureRef = useRef(capture)
  captureRef.current = capture

  // Fullscreen covers the player frame rather than the bare video, so the
  // capture and skip buttons stay on screen. iPhones only allow the video
  // itself to go fullscreen.
  function toggleFullscreen() {
    const f = frame.current
    if (document.fullscreenElement) {
      document.exitFullscreen().catch(() => {})
      return
    }
    if (f?.requestFullscreen) {
      f.requestFullscreen()
        .then(() => (screen.orientation as ScreenOrientation & { lock?: (o: string) => Promise<void> }).lock?.('landscape'))
        .catch(() => {})
    } else {
      ;(video.current as (HTMLVideoElement & { webkitEnterFullscreen?: () => void }) | null)?.webkitEnterFullscreen?.()
    }
  }
  const fullscreenRef = useRef(toggleFullscreen)
  fullscreenRef.current = toggleFullscreen

  // Skipping the ending goes to the next episode when there is one.
  function skipPart() {
    const v = video.current
    if (!v || !inPart || !skip) return
    if (inPart === 'ed' && nextEp !== undefined) {
      navigate(`/watch/${animeId}/${nextEp}${watchQuery}`)
      return
    }
    const iv = inPart === 'op' ? skip.op : skip.ed
    if (iv) v.currentTime = toShortened(iv.end, removed)
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
          fullscreenRef.current()
          break
        case 'c':
          captureRef.current()
          break
        case 's':
          skipRef.current()
          break
        case 'n':
          if (nextEp !== undefined) navigate(`/watch/${animeId}/${nextEp}${watchQuery}`)
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
          <div>
            <ErrorBox title="The video stopped." error={error} onRetry={onExpired} />
            {otherSource && (
              <button type="button" onClick={otherSource.pick} className="min-h-11 text-sm underline">
                Play from {otherSource.name} instead
              </button>
            )}
          </div>
        </div>
      ) : (
        <div
          ref={frame}
          className="player relative"
          onPointerMove={() => setAwake(true)}
          onPointerDown={() => setAwake(true)}
        >
          <video
            ref={video}
            controls
            playsInline
            // Lets a natively played cross-origin stream be read for captures.
            // Direct MP4 hosts send no CORS headers, and asking for it would
            // stop them playing at all, so captures are not offered there.
            crossOrigin={type === 'hls' ? 'anonymous' : undefined}
            onError={onVideoError}
            // The player's own fullscreen button below replaces this one.
            controlsList="nofullscreen"
            onLoadedMetadata={onLoaded}
            onTimeUpdate={onTime}
            onPlay={() => setPaused(false)}
            onPause={() => {
              setPaused(true)
              savePosition()
            }}
            className="aspect-video w-full bg-black"
          />
          {/* Shown while paused or for a moment after the pointer moves, like the native controls. */}
          <div
            className={`absolute top-2 right-2 flex gap-1 transition-opacity ${
              paused || awake || shot ? 'opacity-100' : 'pointer-events-none opacity-0'
            }`}
          >
            <button
              type="button"
              onClick={capture}
              className="min-h-11 rounded-md bg-black/70 px-3 text-sm font-medium text-ink hover:bg-black/90"
            >
              Capture
            </button>
            <button
              type="button"
              onClick={toggleFullscreen}
              className="min-h-11 rounded-md bg-black/70 px-3 text-sm font-medium text-ink hover:bg-black/90"
            >
              {fullscreen ? 'Exit fullscreen' : 'Fullscreen'}
            </button>
          </div>
          {shot && (
            <p role="status" className="absolute top-16 right-2 max-w-[80%] rounded-md bg-black/80 px-3 py-2 text-sm text-ink">
              {shot.text}
              {shot.ok && !fullscreen && (
                <>
                  {' · '}
                  <Link to="/captures" className="underline">
                    {held} held, view them
                  </Link>
                </>
              )}
              {shot.ok && fullscreen && ` · ${held} held`}
            </p>
          )}
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
      {checking && (
        <p className="px-4 py-1 text-sm text-muted sm:px-3">The source sent a broken part; checking the rest of this episode…</p>
      )}
      {removed.length > 0 && (
        <p className="px-4 py-1 text-sm text-muted sm:px-3">
          Parts of this episode are broken at the source and are left out:{' '}
          {removed.map((g) => `${clock(g.start)}–${clock(g.end)}`).join(', ')}.
          {otherSource && (
            <>
              {' '}
              <button type="button" onClick={otherSource.pick} className="min-h-11 text-ink underline">
                Play the whole episode from {otherSource.name}
              </button>
            </>
          )}
        </p>
      )}
      <p role="status" aria-live="polite" className="sr-only sm:not-sr-only sm:px-3 sm:py-1 sm:text-xs sm:text-muted">
        {status}
      </p>
    </div>
  )
}
