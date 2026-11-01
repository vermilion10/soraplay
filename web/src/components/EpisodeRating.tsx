import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Rating } from '../lib/api'

const SCORES = [1, 2, 3, 4, 5, 6, 7, 8, 9, 10]

// Rates one episode from 1 to 10, with an optional note. Saved on the
// server, so it shows on every device and on the anime page's chart.
export function EpisodeRating({ animeId, ep }: { animeId: number; ep: number }) {
  const qc = useQueryClient()
  const key = ['ratings', animeId]
  const q = useQuery({ queryKey: key, queryFn: () => api.ratings(animeId) })
  const current = q.data?.find((r) => r.episode === ep) ?? null
  const [noteOpen, setNoteOpen] = useState(false)
  const [draft, setDraft] = useState<string | null>(null)
  const note = draft ?? current?.note ?? ''

  // Applied on screen at once; the server's answer replaces it.
  const put = (list: Rating[] | undefined, r: Rating | null) =>
    [...(list ?? []).filter((x) => x.episode !== ep), ...(r ? [r] : [])].sort((a, b) => a.episode - b.episode)
  const save = useMutation({
    mutationFn: (v: { score: number; note: string }) => api.saveRating(animeId, ep, v.score, v.note),
    onMutate: (v) => qc.setQueryData<Rating[]>(key, (l) => put(l, { episode: ep, ...v, updatedAt: Date.now() / 1000 })),
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  })
  const clear = useMutation({
    mutationFn: () => api.deleteRating(animeId, ep),
    onMutate: () => qc.setQueryData<Rating[]>(key, (l) => put(l, null)),
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  })

  function saveNote(e: FormEvent) {
    e.preventDefault()
    if (!current) return
    save.mutate({ score: current.score, note: note.trim() }, { onSuccess: () => setDraft(null) })
    setNoteOpen(false)
  }

  const error = save.error ?? clear.error

  return (
    <section aria-labelledby="rate-title" className="mt-5 rounded-md border border-line p-3 sm:p-4">
      <div className="flex flex-wrap items-baseline justify-between gap-x-3">
        <h2 id="rate-title" className="font-display text-lg font-semibold">
          Your rating for EP {ep}
        </h2>
        <p className="text-xs text-muted">Kept in SoraPlay; AniList scores whole anime only.</p>
      </div>
      <div role="radiogroup" aria-labelledby="rate-title" className="mt-2 grid grid-cols-5 gap-1 sm:grid-cols-10">
        {SCORES.map((n) => {
          const on = current?.score === n
          return (
            <button
              key={n}
              type="button"
              role="radio"
              aria-checked={on}
              aria-label={`${n} out of 10`}
              onClick={() => save.mutate({ score: n, note: current?.note ?? '' })}
              className={`min-h-11 rounded-md border font-display text-lg font-semibold ${
                on ? 'border-ink bg-ink text-bg' : 'border-line text-muted hover:border-muted hover:text-ink'
              }`}
            >
              {n}
            </button>
          )
        })}
      </div>

      {current && (
        <div className="mt-2 text-sm">
          {noteOpen ? (
            <form onSubmit={saveNote} className="flex flex-wrap gap-2">
              <label htmlFor="rate-note" className="sr-only">
                Note for episode {ep}
              </label>
              <input
                id="rate-note"
                value={note}
                onChange={(e) => setDraft(e.target.value)}
                maxLength={280}
                autoFocus
                placeholder="What stood out, in a line"
                className="min-h-11 min-w-0 flex-1 rounded-md border border-line bg-surface px-3 placeholder:text-muted"
              />
              <button type="submit" className="min-h-11 rounded-md border border-line px-4 hover:border-muted">
                Save note
              </button>
            </form>
          ) : (
            <p className="flex flex-wrap items-center gap-x-3 text-muted">
              {current.note && <span className="text-ink">“{current.note}”</span>}
              <button type="button" onClick={() => setNoteOpen(true)} className="min-h-11 underline hover:text-ink">
                {current.note ? 'Edit note' : 'Add a note'}
              </button>
              <button type="button" onClick={() => clear.mutate()} className="min-h-11 underline hover:text-ink">
                Clear rating
              </button>
            </p>
          )}
        </div>
      )}
      {error && (
        <p role="alert" className="mt-2 text-sm">
          Could not save the rating: {error instanceof Error ? error.message : String(error)}
        </p>
      )}
    </section>
  )
}
