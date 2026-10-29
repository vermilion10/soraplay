import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api, type Candidate, type Mapping } from '../lib/api'
import { ErrorBox, Loading } from './States'

// A dialog for linking an AniList entry to a show on the source site by hand,
// for when the automatic match is wrong or found nothing. Native <dialog>
// gives focus trapping and Escape to close.
export function SourcePicker({
  animeId,
  defaultQuery,
  current,
  sourceName,
  open,
  onClose,
}: {
  animeId: number
  defaultQuery: string
  current: Mapping | null
  sourceName: string
  open: boolean
  onClose: () => void
}) {
  const ref = useRef<HTMLDialogElement>(null)
  const [text, setText] = useState(defaultQuery)
  const [q, setQ] = useState(defaultQuery)
  const [offset, setOffset] = useState(String(current?.epOffset ?? 0))
  const qc = useQueryClient()

  useEffect(() => {
    const d = ref.current
    if (!d) return
    if (open && !d.open) d.showModal()
    if (!open && d.open) d.close()
  }, [open])

  const results = useQuery({
    queryKey: ['candidates', animeId, q],
    queryFn: () => api.candidates(animeId, q),
    enabled: open && q !== '',
  })
  const save = useMutation({
    mutationFn: (c: Candidate) => api.saveMapping(animeId, c.id, Number(offset) || 0),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['anime', animeId] })
      onClose()
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    setQ(text.trim())
  }

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      aria-labelledby="picker-title"
      className="m-auto w-[min(40rem,calc(100vw-2rem))] rounded-lg border border-line bg-surface p-0 text-ink"
    >
      <div className="flex items-center justify-between gap-4 border-b border-line px-4 py-2">
        <h2 id="picker-title" className="font-display text-xl font-semibold">
          Choose the show on {sourceName}
        </h2>
        <button type="button" onClick={onClose} className="min-h-11 px-2 text-sm text-muted hover:text-ink">
          Close
        </button>
      </div>
      <div className="max-h-[70dvh] overflow-y-auto p-4">
        <form onSubmit={submit} className="flex gap-2">
          <label htmlFor="picker-q" className="sr-only">
            Search {sourceName}
          </label>
          <input
            id="picker-q"
            type="search"
            value={text}
            onChange={(e) => setText(e.target.value)}
            className="min-h-11 min-w-0 flex-1 rounded-md border border-line bg-bg px-3"
          />
          <button type="submit" className="min-h-11 rounded-md border border-line px-4 hover:border-muted">
            Search
          </button>
        </form>
        <label className="mt-3 flex flex-wrap items-center gap-2 text-sm text-muted">
          Episode offset
          <input
            type="number"
            step="1"
            value={offset}
            onChange={(e) => setOffset(e.target.value)}
            className="min-h-11 w-20 rounded-md border border-line bg-bg px-2 text-ink"
          />
          <span className="basis-full sm:basis-auto">
            Added to the AniList episode number to get the site's number, for sites that number a split season
            continuously.
          </span>
        </label>

        <div className="mt-4">
          {results.isFetching && <Loading what="results" />}
          {results.isError && <ErrorBox title="Search failed." error={results.error} />}
          {save.isError && <ErrorBox title="Could not save that choice." error={save.error} />}
          {results.data && results.data.length === 0 && !results.isFetching && (
            <p className="text-sm text-muted">Nothing on {sourceName} matches "{q}". Try a shorter or different title.</p>
          )}
          <ul className="flex flex-col gap-1">
            {results.data?.map((c) => (
              <li key={c.id}>
                <button
                  type="button"
                  onClick={() => save.mutate(c)}
                  disabled={save.isPending}
                  className="flex min-h-11 w-full items-center gap-3 rounded-md p-2 text-left hover:bg-bg"
                >
                  {c.poster && <img src={c.poster} alt="" className="h-14 w-10 shrink-0 rounded object-cover" />}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">{c.title}</span>
                    <span className="text-xs text-muted">
                      {c.episodes ? `${c.episodes} episodes` : 'episode count unknown'}
                      {current?.providerId === c.id && ' · current choice'}
                    </span>
                  </span>
                </button>
              </li>
            ))}
          </ul>
        </div>
      </div>
    </dialog>
  )
}
