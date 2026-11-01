import { useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router'
import { clock } from '../lib/api'
import { deleteCapture, useCaptures, type Capture } from '../lib/captures'
import { DEFAULT_OVERLAY, fileName, renderCapture, type ImageType, type Overlay, type OverlayStyle } from '../lib/captureRender'
import { Empty, Loading } from '../components/States'

const OVERLAY_KEY = 'captures.overlay'

function readOverlay(): Overlay {
  try {
    return { ...DEFAULT_OVERLAY, ...JSON.parse(localStorage.getItem(OVERLAY_KEY) ?? '{}') }
  } catch {
    return DEFAULT_OVERLAY
  }
}

const STYLES: { v: OverlayStyle; label: string }[] = [
  { v: 'none', label: 'Scene only' },
  { v: 'bar', label: 'Caption bar' },
  { v: 'tag', label: 'Corner tag' },
]

const FIELDS: { k: 'title' | 'episode' | 'time' | 'date' | 'app'; label: string }[] = [
  { k: 'title', label: 'Anime title' },
  { k: 'episode', label: 'Episode' },
  { k: 'time', label: 'Time in the episode' },
  { k: 'date', label: 'Date captured' },
  { k: 'app', label: 'SoraPlay name' },
]

export default function Captures() {
  const list = useCaptures()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const selected = list?.find((c) => c.id === selectedId) ?? list?.[0]

  // One object URL per capture for the thumbnails, released when they go.
  const thumbs = useMemo(() => new Map((list ?? []).map((c) => [c.id, URL.createObjectURL(c.blob)])), [list])
  useEffect(() => () => thumbs.forEach((u) => URL.revokeObjectURL(u)), [thumbs])

  if (list === null) return <Loading what="your captures" />

  return (
    <section className="mt-4">
      <h1 className="font-display text-3xl font-bold">Captures</h1>
      <p className="mt-1 max-w-prose text-sm text-muted">
        Scenes you capture in the player (the Capture button, or C on a keyboard) wait here, in this browser only,
        until you save or delete them.
      </p>
      {list.length === 0 || !selected ? (
        <Empty>
          No captures yet. Pause on a scene you like and press Capture.{' '}
          <Link to="/" className="text-ink underline">
            Back to your shows
          </Link>
        </Empty>
      ) : (
        <>
          <Editor key={selected.id} c={selected} />
          <h2 className="mt-10 mb-3 font-display text-xl font-semibold text-muted">
            Held in this browser <span className="font-sans text-sm font-normal">{list.length}</span>
          </h2>
          <ul className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
            {list.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => setSelectedId(c.id)}
                  aria-pressed={c.id === selected.id}
                  className={`block w-full rounded-md border-2 text-left ${
                    c.id === selected.id ? 'border-ink' : 'border-transparent hover:border-line'
                  }`}
                >
                  <img src={thumbs.get(c.id)} alt="" className="aspect-video w-full rounded object-cover" />
                  <span className="block truncate px-1 pt-1 text-sm">{c.title || `AniList #${c.animeId}`}</span>
                  <span className="block px-1 pb-1 text-xs text-muted">
                    EP {c.episode} · {clock(c.time)}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </>
      )}
    </section>
  )
}

function Editor({ c }: { c: Capture }) {
  const [o, setO] = useState(readOverlay)
  const [type, setType] = useState<ImageType>('image/png')
  const [preview, setPreview] = useState<{ url: string; blob: Blob } | null>(null)
  const [msg, setMsg] = useState('')
  const [confirmDelete, setConfirmDelete] = useState(false)

  function update(next: Overlay) {
    setO(next)
    try {
      localStorage.setItem(OVERLAY_KEY, JSON.stringify(next))
    } catch {
      // Not remembered for next time; this edit still applies.
    }
  }

  useEffect(() => {
    let live = true
    let url = ''
    renderCapture(c, o, type)
      .then((blob) => {
        if (!live) return
        url = URL.createObjectURL(blob)
        setPreview({ url, blob })
      })
      .catch((e: Error) => live && setMsg(e.message))
    return () => {
      live = false
      if (url) URL.revokeObjectURL(url)
    }
  }, [c, o, type])

  const name = fileName(c, type)
  const file = preview ? new File([preview.blob], name, { type }) : null
  const canShare = !!file && typeof navigator.canShare === 'function' && navigator.canShare({ files: [file] })

  function save() {
    if (!preview) return
    const a = document.createElement('a')
    a.href = preview.url
    a.download = name
    a.click()
    setMsg(`Saved as ${name}`)
  }

  async function share() {
    if (!file) return
    try {
      await navigator.share({ files: [file], title: c.title })
    } catch (e) {
      if ((e as Error).name !== 'AbortError') setMsg(`Could not share: ${(e as Error).message}`)
    }
  }

  return (
    <div className="mt-5 grid gap-5 lg:grid-cols-[1fr_18rem]">
      <figure>
        <div className="aspect-video overflow-hidden rounded-md bg-black">
          {preview && <img src={preview.url} alt={`Capture from ${c.title} episode ${c.episode} at ${clock(c.time)}`} className="size-full object-contain" />}
        </div>
        <figcaption className="mt-2 text-sm text-muted">
          <Link to={`/anime/${c.animeId}`} className="text-ink hover:underline">
            {c.title || `AniList #${c.animeId}`}
          </Link>{' '}
          · EP {c.episode} at {clock(c.time)} · {c.width}×{c.height}
        </figcaption>
      </figure>

      <div>
        <fieldset>
          <legend className="text-sm text-muted">Info on the image</legend>
          <div className="mt-2 flex flex-wrap gap-1">
            {STYLES.map((s) => (
              <label
                key={s.v}
                className={`flex min-h-11 cursor-pointer items-center rounded-md border px-3 text-sm has-focus-visible:outline-2 has-focus-visible:outline-accent ${
                  o.style === s.v ? 'border-ink text-ink' : 'border-line text-muted hover:border-muted'
                }`}
              >
                <input
                  type="radio"
                  name="overlay-style"
                  value={s.v}
                  checked={o.style === s.v}
                  onChange={() => update({ ...o, style: s.v })}
                  className="sr-only"
                />
                {s.label}
              </label>
            ))}
          </div>
        </fieldset>

        <fieldset disabled={o.style === 'none'} className="mt-4 disabled:opacity-50">
          <legend className="text-sm text-muted">Where</legend>
          <div className="mt-2 flex gap-1">
            {(['bottom', 'top'] as const).map((p) => (
              <label
                key={p}
                className={`flex min-h-11 cursor-pointer items-center rounded-md border px-3 text-sm has-focus-visible:outline-2 has-focus-visible:outline-accent ${
                  o.position === p ? 'border-ink text-ink' : 'border-line text-muted hover:border-muted'
                }`}
              >
                <input
                  type="radio"
                  name="overlay-position"
                  value={p}
                  checked={o.position === p}
                  onChange={() => update({ ...o, position: p })}
                  className="sr-only"
                />
                {p === 'bottom' ? 'Bottom' : 'Top, clear of subtitles'}
              </label>
            ))}
          </div>
        </fieldset>

        <fieldset disabled={o.style === 'none'} className="mt-4 disabled:opacity-50">
          <legend className="text-sm text-muted">Show</legend>
          {FIELDS.map((f) => (
            <label key={f.k} className="flex min-h-11 items-center gap-3 text-sm">
              <input
                type="checkbox"
                checked={o[f.k]}
                onChange={(e) => update({ ...o, [f.k]: e.target.checked })}
                className="size-5 accent-ink"
              />
              {f.label}
            </label>
          ))}
        </fieldset>

        <label className="mt-3 flex items-center gap-2 text-sm text-muted">
          Format
          <select
            value={type}
            onChange={(e) => setType(e.target.value as ImageType)}
            className="min-h-11 rounded-md border border-line bg-surface px-2 text-ink"
          >
            <option value="image/png">PNG, full quality</option>
            <option value="image/jpeg">JPEG, smaller file</option>
          </select>
        </label>

        <div className="mt-5 flex flex-wrap gap-2">
          <button
            type="button"
            onClick={save}
            disabled={!preview}
            className="min-h-11 rounded-md bg-ink px-5 font-medium text-bg hover:brightness-95 disabled:opacity-60"
          >
            Save image
          </button>
          {canShare && (
            <button type="button" onClick={share} className="min-h-11 rounded-md border border-line px-4 hover:border-muted">
              Share
            </button>
          )}
          <button
            type="button"
            onClick={() => (confirmDelete ? deleteCapture(c.id) : setConfirmDelete(true))}
            className="min-h-11 px-2 text-sm text-muted underline hover:text-ink"
          >
            {confirmDelete ? 'Delete it?' : 'Delete'}
          </button>
        </div>
        <p role="status" className="mt-2 text-sm break-words text-muted">
          {msg}
        </p>
      </div>
    </div>
  )
}
