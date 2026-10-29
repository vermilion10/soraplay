import { lazy, Suspense } from 'react'
import { Link, NavLink, Route, Routes } from 'react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from './lib/api'
import { APP_NAME } from './lib/app'
import Home from './pages/Home'
import Search from './pages/Search'
import Anime from './pages/Anime'
import { Loading, NotFound } from './components/States'

// The player pulls in hls.js, the largest dependency; load it only when watching.
const Watch = lazy(() => import('./pages/Watch'))

export default function App() {
  return (
    <div className="mx-auto flex min-h-dvh max-w-6xl flex-col px-4 sm:px-6">
      <Header />
      <main className="flex-1 pb-12">
        <Routes>
          <Route index element={<Home />} />
          <Route path="search" element={<Search />} />
          <Route path="anime/:id" element={<Anime />} />
          <Route
            path="watch/:id/:ep"
            element={
              <Suspense fallback={<Loading what="the player" />}>
                <Watch />
              </Suspense>
            }
          />
          <Route path="*" element={<NotFound />} />
        </Routes>
      </main>
    </div>
  )
}

function Header() {
  const qc = useQueryClient()
  const me = useQuery({ queryKey: ['me'], queryFn: api.me })
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => qc.invalidateQueries(),
  })
  const navClass = ({ isActive }: { isActive: boolean }) =>
    `inline-flex min-h-11 items-center px-2 text-sm ${isActive ? 'text-ink' : 'text-muted hover:text-ink'}`

  return (
    <header className="flex items-center gap-2 py-3">
      <Link to="/" className="mr-auto inline-flex min-h-11 items-center font-display text-2xl font-bold tracking-wide">
        {APP_NAME}
      </Link>
      <NavLink to="/search" className={navClass}>
        Search
      </NavLink>
      {me.data?.user ? (
        <>
          <span className="hidden items-center gap-2 text-sm text-muted sm:inline-flex">
            <img src={me.data.user.avatar.medium} alt="" className="size-7 rounded-full" />
            {me.data.user.name}
          </span>
          <button
            type="button"
            onClick={() => logout.mutate()}
            disabled={logout.isPending}
            className="min-h-11 px-2 text-sm text-muted hover:text-ink"
          >
            Log out
          </button>
        </>
      ) : me.data?.configured ? (
        <a href="/auth/login" className={navClass({ isActive: false })}>
          Log in
        </a>
      ) : null}
    </header>
  )
}
