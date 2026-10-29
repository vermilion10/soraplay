import { useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router'
import { useQuery } from '@tanstack/react-query'
import { api } from '../lib/api'
import { PosterGrid } from '../components/PosterGrid'
import { Empty, ErrorBox, Loading } from '../components/States'

export default function Search() {
  const [params, setParams] = useSearchParams()
  const q = params.get('q') ?? ''
  const [text, setText] = useState(q)
  const results = useQuery({ queryKey: ['search', q], queryFn: () => api.search(q), enabled: q !== '' })

  function submit(e: FormEvent) {
    e.preventDefault()
    const t = text.trim()
    setParams(t ? { q: t } : {})
  }

  return (
    <section className="mt-4">
      <h1 className="sr-only">Search</h1>
      <form onSubmit={submit} role="search" className="flex gap-2">
        <label htmlFor="q" className="sr-only">
          Anime title
        </label>
        <input
          id="q"
          type="search"
          value={text}
          onChange={(e) => setText(e.target.value)}
          placeholder="Anime title, in romaji or English"
          autoFocus
          className="min-h-11 min-w-0 flex-1 rounded-md border border-line bg-surface px-3 placeholder:text-muted"
        />
        <button type="submit" className="min-h-11 rounded-md border border-line px-4 hover:border-muted">
          Search
        </button>
      </form>

      <div className="mt-6">
        {q === '' && <p className="text-sm text-muted">Results come from AniList.</p>}
        {results.isFetching && <Loading what={`results for "${q}"`} />}
        {results.isError && (
          <ErrorBox title="Search failed." error={results.error} onRetry={() => results.refetch()} />
        )}
        {results.data && results.data.length === 0 && <Empty>No anime on AniList matches "{q}".</Empty>}
        {results.data && results.data.length > 0 && !results.isFetching && (
          <PosterGrid
            items={results.data.map((media) => ({
              media,
              note: [media.format?.replace('_', ' '), media.seasonYear].filter(Boolean).join(' · '),
            }))}
          />
        )}
      </div>
    </section>
  )
}
