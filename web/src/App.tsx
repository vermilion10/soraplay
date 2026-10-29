import { lazy, Suspense, useState, type FormEvent } from 'react'
import { Link, NavLink, Route, Routes, useNavigate } from 'react-router'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from './lib/api'
import { APP_NAME } from './lib/app'
import { useMe } from './lib/list'
import Home from './pages/Home'
import Search from './pages/Search'
import Anime from './pages/Anime'
import Schedule from './pages/Schedule'
import Seasons from './pages/Seasons'
import MyList from './pages/MyList'
import Saved from './pages/Saved'
import { Loading, NotFound } from './components/States'

// The player pulls in hls.js, the largest dependency; load it only when watching.
const Watch = lazy(() => import('./pages/Watch'))

const SECTIONS = [
  { to: '/', label: 'Home' },
  { to: '/schedule', label: 'Schedule' },
  { to: '/seasons', label: 'Seasons' },
  { to: '/list', label: 'My list' },
]

export default function App() {
  return (
    <div className="mx-auto flex min-h-dvh max-w-6xl flex-col px-4 sm:px-6">
      <Header />
      {/* On phones the bottom tab bar covers the last 4rem. */}
      <main className="flex-1 pb-28 sm:pb-12">
        <Routes>
          <Route index element={<Home />} />
          <Route path="schedule" element={<Schedule />} />
          <Route path="seasons" element={<Seasons />} />
          <Route path="list" element={<MyList />} />
          <Route path="saved" element={<Saved />} />
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
      <TabBar />
    </div>
  )
}

function Header() {
  const navClass = ({ isActive }: { isActive: boolean }) =>
    `inline-flex min-h-11 items-center px-2 font-display text-lg font-semibold ${
      isActive ? 'text-ink' : 'text-muted hover:text-ink'
    }`

  return (
    <header className="flex items-center gap-1 py-3 sm:gap-3">
      <Link to="/" className="mr-auto inline-flex min-h-11 items-center font-display text-2xl font-bold tracking-wide sm:mr-4">
        {APP_NAME}
      </Link>
      <nav aria-label="Sections" className="hidden sm:flex">
        {SECTIONS.map((s) => (
          <NavLink key={s.to} to={s.to} end={s.to === '/'} className={navClass}>
            {s.label}
          </NavLink>
        ))}
      </nav>
      <SearchBox />
      <NavLink to="/search" className={(a) => `${navClass(a)} sm:hidden`}>
        Search
      </NavLink>
      <Account />
    </header>
  )
}

// A search field in the bar on wider screens; phones get the Search link.
function SearchBox() {
  const navigate = useNavigate()
  const [text, setText] = useState('')
  function submit(e: FormEvent) {
    e.preventDefault()
    const t = text.trim()
    if (t) navigate(`/search?q=${encodeURIComponent(t)}`)
  }
  return (
    <form onSubmit={submit} role="search" className="ml-auto hidden md:block">
      <label htmlFor="bar-search" className="sr-only">
        Search anime
      </label>
      <input
        id="bar-search"
        type="search"
        value={text}
        onChange={(e) => setText(e.target.value)}
        placeholder="Search anime"
        className="min-h-10 w-52 rounded-md border border-line bg-surface px-3 text-sm placeholder:text-muted lg:w-64"
      />
    </form>
  )
}

function Account() {
  const qc = useQueryClient()
  const me = useMe()
  const logout = useMutation({
    mutationFn: api.logout,
    onSuccess: () => qc.invalidateQueries(),
  })
  const user = me.data?.user

  if (!user) {
    return me.data?.configured ? (
      <a href="/auth/login" className="inline-flex min-h-11 items-center px-2 text-sm text-muted hover:text-ink">
        Log in
      </a>
    ) : null
  }

  const item = 'flex min-h-11 w-full items-center px-4 text-left text-sm hover:bg-line'
  return (
    <>
      <button
        type="button"
        popoverTarget="account-menu"
        aria-label={`Account: ${user.name}`}
        className="inline-flex min-h-11 min-w-11 items-center justify-center"
      >
        <img src={user.avatar.medium} alt="" className="size-8 rounded-full" />
      </button>
      {/* Native popover: closes on Escape, outside clicks and navigation links below. */}
      <div
        id="account-menu"
        popover="auto"
        className="account-menu w-56 rounded-md border border-line bg-surface py-1 text-ink"
      >
        <p className="px-4 py-2 text-sm text-muted">{user.name}</p>
        <Link to="/saved" className={item} onClick={hideMenu}>
          Saved episodes
        </Link>
        <a href={`https://anilist.co/user/${encodeURIComponent(user.name)}`} target="_blank" rel="noreferrer" className={item}>
          Profile on AniList
        </a>
        <button
          type="button"
          onClick={() => {
            hideMenu()
            logout.mutate()
          }}
          disabled={logout.isPending}
          className={`${item} text-muted`}
        >
          Log out
        </button>
      </div>
    </>
  )
}

function hideMenu() {
  document.getElementById('account-menu')?.hidePopover()
}

// Phone navigation, in thumb reach. Text labels only: the four words are
// shorter to read than icons would be to learn.
function TabBar() {
  return (
    <nav
      aria-label="Sections"
      className="fixed inset-x-0 bottom-0 z-10 grid grid-cols-4 border-t border-line bg-bg/95 pb-[env(safe-area-inset-bottom)] backdrop-blur sm:hidden"
    >
      {SECTIONS.map((s) => (
        <NavLink
          key={s.to}
          to={s.to}
          end={s.to === '/'}
          className={({ isActive }) =>
            `flex min-h-14 items-center justify-center border-t-2 font-display text-lg font-semibold ${
              isActive ? 'border-ink text-ink' : 'border-transparent text-muted'
            }`
          }
        >
          {s.label}
        </NavLink>
      ))}
    </nav>
  )
}
