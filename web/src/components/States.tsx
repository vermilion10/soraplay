import type { ReactNode } from 'react'
import { Link } from 'react-router'

export function Loading({ what }: { what: string }) {
  return (
    <p role="status" className="py-10 text-sm text-muted">
      Loading {what}…
    </p>
  )
}

export function ErrorBox({ title, error, onRetry }: { title: string; error: unknown; onRetry?: () => void }) {
  const msg = error instanceof Error ? error.message : String(error)
  return (
    <div role="alert" className="my-6 rounded-md border border-line bg-surface p-4">
      <p className="font-medium">{title}</p>
      <p className="mt-1 break-words text-sm text-muted">{msg}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          className="mt-3 min-h-11 rounded-md border border-line px-4 text-sm hover:border-muted"
        >
          Try again
        </button>
      )}
    </div>
  )
}

export function Empty({ children }: { children: ReactNode }) {
  return <div className="my-6 rounded-md border border-dashed border-line p-6 text-sm text-muted">{children}</div>
}

export function NotFound() {
  return (
    <Empty>
      This page does not exist.{' '}
      <Link to="/" className="text-ink underline">
        Back to the home page
      </Link>
    </Empty>
  )
}
