import type { Interval } from './api'

// Some sources serve an error page in place of a few video segments. hls.js
// cannot play past such holes in a VOD, so the playlist is rebuilt without
// them: every segment is checked (its first byte only), broken ones are left
// out, and a discontinuity marks each cut so the player handles the jump in
// timestamps. Times are then shifted by what was cut; these helpers convert
// between the episode's own timeline and the shortened one.

export type Repair = {
  url: string // blob: URL of the rebuilt playlist
  removed: Interval[] // cut parts, in the episode's own times, in order
}

// A time in the shortened playlist -> the same moment in the episode.
export function toOriginal(t: number, removed: Interval[]): number {
  let o = t
  for (const r of removed) {
    if (r.start <= o) o += r.end - r.start
    else break
  }
  return o
}

// A time in the episode -> the shortened playlist. Inside a cut part, it is
// where the cut is.
export function toShortened(t: number, removed: Interval[]): number {
  let shift = 0
  for (const r of removed) {
    if (t >= r.end) shift += r.end - r.start
    else if (t >= r.start) return r.start - shift
    else break
  }
  return t - shift
}

type Segment = { tags: string[]; uri: string; start: number; end: number }

export async function repairPlaylist(src: string): Promise<Repair> {
  let base = new URL(src, location.href)
  let text = await fetchText(base.href)
  if (text.includes('#EXT-X-STREAM-INF')) {
    // A master playlist: the first variant, as these sources offer one.
    const variant = text.split(/\r?\n/).find((l) => l.trim() && !l.startsWith('#'))
    if (!variant) throw new Error('The playlist has no variants.')
    base = new URL(variant.trim(), base)
    text = await fetchText(base.href)
  }

  const header: string[] = []
  const segments: Segment[] = []
  let tags: string[] = []
  let t = 0
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim()
    if (!line || line === '#EXT-X-ENDLIST') continue
    if (line.startsWith('#')) {
      // Wall-clock dates would contradict the cuts; they are not needed for VOD.
      if (line.startsWith('#EXT-X-PROGRAM-DATE-TIME')) continue
      if (segments.length === 0 && tags.length === 0 && !line.startsWith('#EXTINF') && !line.startsWith('#EXT-X-DISCONTINUITY')) {
        header.push(line)
      } else {
        tags.push(line)
      }
      continue
    }
    const dur = Number(tags.find((x) => x.startsWith('#EXTINF:'))?.slice(8).split(',')[0]) || 0
    segments.push({ tags, uri: new URL(line, base).href, start: t, end: t + dur })
    t += dur
    tags = []
  }

  const ok = await mapLimit(segments, 8, (s) => isVideo(s.uri))
  const out = [...header]
  const removed: Interval[] = []
  let cut = false
  segments.forEach((s, i) => {
    if (!ok[i]) {
      const last = removed[removed.length - 1]
      if (last && Math.abs(last.end - s.start) < 0.01) last.end = s.end
      else removed.push({ start: s.start, end: s.end })
      cut = true
      return
    }
    if (cut && out.length > header.length) out.push('#EXT-X-DISCONTINUITY')
    cut = false
    out.push(...s.tags, s.uri)
  })
  out.push('#EXT-X-ENDLIST')
  const url = URL.createObjectURL(new Blob([out.join('\n') + '\n'], { type: 'application/vnd.apple.mpegurl' }))
  return { url, removed }
}

async function fetchText(u: string): Promise<string> {
  const res = await fetch(u)
  if (!res.ok) throw new Error(`The source answered ${res.status} for the playlist.`)
  return res.text()
}

// Reads one byte: video is binary, an error page starts with '<'. When the
// check itself fails (offline for a moment), the segment is kept.
async function isVideo(u: string): Promise<boolean> {
  try {
    const res = await fetch(u, { headers: { Range: 'bytes=0-0' } })
    if (res.status === 403 || res.status === 404 || res.status === 410) return false
    const reader = res.body?.getReader()
    if (!reader) return true
    const { value } = await reader.read()
    reader.cancel().catch(() => {})
    return !value || value.length === 0 || value[0] !== 0x3c
  } catch {
    return true
  }
}

async function mapLimit<T, R>(items: T[], limit: number, fn: (x: T) => Promise<R>): Promise<R[]> {
  const out: R[] = new Array(items.length)
  let next = 0
  const worker = async () => {
    while (next < items.length) {
      const i = next++
      out[i] = await fn(items[i])
    }
  }
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker))
  return out
}
