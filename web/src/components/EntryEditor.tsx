import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, titleOf, type EntryUpdate, type ListEntry, type ListStatus, type Media } from '../lib/api'
import { dateValue, fuzzyDate, POINT_3_LABELS, scoreSpec, STATUS_LABEL, useMe } from '../lib/list'

const STATUSES: ListStatus[] = ['CURRENT', 'PLANNING', 'COMPLETED', 'REPEATING', 'PAUSED', 'DROPPED']

type Props = { media: Media; entry: ListEntry | null; open: boolean; onClose: () => void }

// Edits the AniList entry of one anime. Only the fields that changed are
// sent, so a date AniList knows only partly is not wiped by saving.
export function EntryEditor({ media, entry, open, onClose }: Props) {
  const ref = useRef<HTMLDialogElement>(null)
  useEffect(() => {
    const d = ref.current
    if (!d) return
    if (open && !d.open) d.showModal()
    if (!open && d.open) d.close()
  }, [open])

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby="entry-title"
      className="m-auto w-[min(32rem,calc(100vw-2rem))] rounded-lg border border-line bg-surface p-0 text-ink"
    >
      {open && <Form media={media} entry={entry} onClose={onClose} />}
    </dialog>
  )
}

function Form({ media, entry, onClose }: Omit<Props, 'open'>) {
  const qc = useQueryClient()
  const me = useMe()
  const fmt = me.data?.user?.mediaListOptions.scoreFormat ?? ''
  const spec = scoreSpec(fmt)
  const total = media.episodes || 0

  const initial = {
    status: entry?.status ?? 'PLANNING',
    progress: String(entry?.progress ?? 0),
    score: String(entry?.score ?? 0),
    repeat: String(entry?.repeat ?? 0),
    startedAt: entry ? dateValue(entry.startedAt) : '',
    completedAt: entry ? dateValue(entry.completedAt) : '',
  }
  const [f, setF] = useState(initial)
  const [confirmRemove, setConfirmRemove] = useState(false)
  const set = (k: keyof typeof f) => (v: string) => setF((p) => ({ ...p, [k]: v }))

  function changes(): EntryUpdate {
    const u: EntryUpdate = {}
    // A new entry sends everything; an existing one only what changed.
    const changed = (k: keyof typeof f) => !entry || f[k] !== initial[k]
    if (changed('status')) u.status = f.status as ListStatus
    if (changed('progress')) u.progress = Number(f.progress)
    if (changed('score') && (entry || Number(f.score) > 0)) u.score = Number(f.score)
    if (changed('repeat') && (entry || Number(f.repeat) > 0)) u.repeat = Number(f.repeat)
    if (f.startedAt !== initial.startedAt) u.startedAt = fuzzyDate(f.startedAt)
    if (f.completedAt !== initial.completedAt) u.completedAt = fuzzyDate(f.completedAt)
    return u
  }

  const done = () => {
    qc.invalidateQueries({ queryKey: ['list'] })
    qc.invalidateQueries({ queryKey: ['anime', media.id] })
    onClose()
  }
  const save = useMutation({ mutationFn: () => api.saveEntry(media.id, changes()), onSuccess: done })
  const remove = useMutation({ mutationFn: () => api.deleteEntry(media.id), onSuccess: done })

  function submit(e: FormEvent) {
    e.preventDefault()
    if (Object.keys(changes()).length === 0) return onClose()
    save.mutate()
  }

  // Marking it completed fills in the episode count, as AniList itself does.
  function pickStatus(s: string) {
    setF((p) => ({ ...p, status: s as ListStatus, progress: s === 'COMPLETED' && total ? String(total) : p.progress }))
  }

  const field = 'mt-1 block min-h-11 w-full rounded-md border border-line bg-bg px-3'
  const error = save.error ?? remove.error

  return (
    <form onSubmit={submit} className="p-5">
      <h2 id="entry-title" className="font-display text-2xl leading-tight font-bold">
        {titleOf(media)}
      </h2>
      <p className="text-sm text-muted">{entry ? 'Edit your AniList entry' : 'Add to your AniList list'}</p>

      <div className="mt-4 grid grid-cols-2 gap-3">
        <label className="col-span-2 text-sm text-muted sm:col-span-1">
          Status
          <select value={f.status} onChange={(e) => pickStatus(e.target.value)} className={`${field} text-ink`}>
            {STATUSES.map((s) => (
              <option key={s} value={s}>
                {STATUS_LABEL[s]}
              </option>
            ))}
          </select>
        </label>
        <label className="col-span-2 text-sm text-muted sm:col-span-1">
          Episodes watched{total ? ` (of ${total})` : ''}
          <input
            type="number"
            inputMode="numeric"
            min={0}
            max={total || undefined}
            value={f.progress}
            onChange={(e) => set('progress')(e.target.value)}
            className={`${field} text-ink`}
          />
        </label>
        <label className="text-sm text-muted">
          Score{fmt === 'POINT_3' ? '' : ` (0–${spec.max})`}
          {fmt === 'POINT_3' ? (
            <select value={f.score} onChange={(e) => set('score')(e.target.value)} className={`${field} text-ink`}>
              {POINT_3_LABELS.map((l, i) => (
                <option key={l} value={i}>
                  {l}
                </option>
              ))}
            </select>
          ) : (
            <input
              type="number"
              inputMode="decimal"
              min={0}
              max={spec.max}
              step={spec.step}
              value={f.score}
              onChange={(e) => set('score')(e.target.value)}
              className={`${field} text-ink`}
            />
          )}
        </label>
        <label className="text-sm text-muted">
          Rewatches
          <input
            type="number"
            inputMode="numeric"
            min={0}
            value={f.repeat}
            onChange={(e) => set('repeat')(e.target.value)}
            className={`${field} text-ink`}
          />
        </label>
        <label className="text-sm text-muted">
          Started
          <input type="date" value={f.startedAt} onChange={(e) => set('startedAt')(e.target.value)} className={`${field} text-ink`} />
        </label>
        <label className="text-sm text-muted">
          Finished
          <input type="date" value={f.completedAt} onChange={(e) => set('completedAt')(e.target.value)} className={`${field} text-ink`} />
        </label>
      </div>

      {error && (
        <p role="alert" className="mt-3 text-sm">
          Could not save: {error instanceof Error ? error.message : String(error)}
        </p>
      )}

      <div className="mt-5 flex flex-wrap items-center gap-2">
        <button
          type="submit"
          disabled={save.isPending}
          className="min-h-11 rounded-md bg-ink px-5 font-medium text-bg hover:brightness-95 disabled:opacity-60"
        >
          {save.isPending ? 'Saving…' : 'Save'}
        </button>
        <button type="button" onClick={onClose} className="min-h-11 rounded-md border border-line px-4 hover:border-muted">
          Cancel
        </button>
        {entry && (
          <button
            type="button"
            onClick={() => (confirmRemove ? remove.mutate() : setConfirmRemove(true))}
            disabled={remove.isPending}
            className="ml-auto min-h-11 px-2 text-sm text-muted underline hover:text-ink"
          >
            {remove.isPending ? 'Removing…' : confirmRemove ? 'Remove from AniList?' : 'Remove from list'}
          </button>
        )}
      </div>
    </form>
  )
}
