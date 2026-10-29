import type { ReactNode } from 'react'
import { Link } from 'react-router'
import { titleOf, type Media } from '../lib/api'

// footer sits under the card, outside its link, for a button of its own.
export type PosterItem = { media: Media; note?: string; footer?: ReactNode }

// A grid of cover images. The cover is the color on the page (DESIGN.md), so
// cards have no frame or shadow of their own.
export function PosterGrid({ items }: { items: PosterItem[] }) {
  return (
    <ul className="grid grid-cols-3 gap-x-3 gap-y-5 sm:grid-cols-4 md:grid-cols-5 lg:grid-cols-6">
      {items.map(({ media, note, footer }) => (
        <li key={media.id} className="min-w-0">
          <Link to={`/anime/${media.id}`} className="group block rounded-md">
            <div
              className="aspect-[2/3] overflow-hidden rounded-md"
              style={{ backgroundColor: media.coverImage.color || 'var(--color-surface)' }}
            >
              <img
                src={media.coverImage.large}
                alt=""
                loading="lazy"
                className="size-full object-cover transition-opacity group-hover:opacity-85"
              />
            </div>
            <p className="mt-1.5 line-clamp-2 text-sm leading-snug">{titleOf(media)}</p>
            {note && <p className="text-xs text-muted">{note}</p>}
          </Link>
          {footer}
        </li>
      ))}
    </ul>
  )
}
