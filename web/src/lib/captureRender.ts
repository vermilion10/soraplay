import { APP_NAME } from './app'
import { clock } from './api'
import type { Capture } from './captures'

export type OverlayStyle = 'none' | 'bar' | 'tag'

// What to draw over a capture when saving it.
export type Overlay = {
  style: OverlayStyle // scene only, a caption bar along one edge, or a small corner tag
  // Bottom by default; top keeps burned-in subtitles clear.
  position: 'bottom' | 'top'
  title: boolean
  episode: boolean
  time: boolean
  date: boolean
  app: boolean
}

export const DEFAULT_OVERLAY: Overlay = { style: 'bar', position: 'bottom', title: true, episode: true, time: true, date: false, app: false }

export type ImageType = 'image/png' | 'image/jpeg'

function details(c: Capture, o: Overlay): string[] {
  return [
    o.episode ? `EP ${c.episode}` : '',
    o.time ? clock(c.time) : '',
    o.date ? new Date(c.capturedAt).toLocaleDateString(undefined, { day: 'numeric', month: 'short', year: 'numeric' }) : '',
  ].filter(Boolean)
}

function hasInfo(c: Capture, o: Overlay): boolean {
  return o.style !== 'none' && ((o.title && !!c.title) || details(c, o).length > 0 || o.app)
}

function fit(ctx: CanvasRenderingContext2D, text: string, max: number): string {
  if (ctx.measureText(text).width <= max) return text
  let t = text
  while (t.length > 1 && ctx.measureText(t + '…').width > max) t = t.slice(0, -1)
  return t.trimEnd() + '…'
}

// Renders the capture with its overlay at the video's own resolution.
export async function renderCapture(c: Capture, o: Overlay, type: ImageType): Promise<Blob> {
  if (!hasInfo(c, o) && type === 'image/png') return c.blob
  const img = await createImageBitmap(c.blob)
  const canvas = document.createElement('canvas')
  canvas.width = img.width
  canvas.height = img.height
  const ctx = canvas.getContext('2d')!
  ctx.drawImage(img, 0, 0)
  img.close()
  if (hasInfo(c, o)) {
    await document.fonts.load('700 40px "Barlow Condensed"').catch(() => {})
    if (o.style === 'bar') drawBar(ctx, c, o)
    else drawTag(ctx, c, o)
  }
  return new Promise((resolve, reject) =>
    canvas.toBlob((b) => (b ? resolve(b) : reject(new Error('Could not encode the image.'))), type, 0.92),
  )
}

const INK = '#f2ece4'
const SANS = 'ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, sans-serif'

// A darkened band along the bottom with the title large in the display face,
// like the title card on the anime page.
function drawBar(ctx: CanvasRenderingContext2D, c: Capture, o: Overlay) {
  const { width: W, height: H } = ctx.canvas
  const u = H / 100
  const pad = Math.round(W * 0.035)
  const top = o.position === 'top'
  const band = H * 0.38
  const g = top ? ctx.createLinearGradient(0, 0, 0, band) : ctx.createLinearGradient(0, H, 0, H - band)
  g.addColorStop(0, 'rgba(0,0,0,0.8)')
  g.addColorStop(1, 'rgba(0,0,0,0)')
  ctx.fillStyle = g
  ctx.fillRect(0, top ? 0 : H - band, W, band)

  ctx.textBaseline = 'alphabetic'
  ctx.fillStyle = INK
  const line = details(c, o).join('  ·  ')
  const hasTitle = o.title && !!c.title
  const titleSize = 6.4 * u
  const lineSize = 3.2 * u
  // Title above the details line, whichever edge the band is on.
  const titleY = top ? pad + titleSize * 0.8 : H - pad - (line ? 4.8 * u : 0)
  const lineY = top ? (hasTitle ? titleY + 4.8 * u : pad + lineSize) : H - pad
  if (line) {
    ctx.font = `500 ${lineSize}px ${SANS}`
    ctx.globalAlpha = 0.88
    ctx.fillText(line, pad, lineY)
    ctx.globalAlpha = 1
  }
  if (hasTitle) {
    ctx.font = `700 ${titleSize}px "Barlow Condensed", ${SANS}`
    ctx.fillText(fit(ctx, c.title, W - 2 * pad - (o.app ? 14 * u : 0)), pad, titleY)
  }
  if (o.app) {
    ctx.font = `700 ${3.4 * u}px "Barlow Condensed", ${SANS}`
    ctx.textAlign = 'right'
    ctx.globalAlpha = 0.7
    ctx.fillText(APP_NAME, W - pad, top ? pad + 3.4 * u : H - pad)
    ctx.globalAlpha = 1
    ctx.textAlign = 'left'
  }
}

// One line in a small dark box in the bottom-left corner, leaving the scene clear.
function drawTag(ctx: CanvasRenderingContext2D, c: Capture, o: Overlay) {
  const { width: W, height: H } = ctx.canvas
  const u = H / 100
  const margin = 2.4 * u
  const padX = 1.4 * u
  const size = 2.8 * u
  ctx.font = `600 ${size}px ${SANS}`
  const rest = [...details(c, o), o.app ? APP_NAME : ''].filter(Boolean).join(' · ')
  let title = o.title ? c.title : ''
  const sep = title && rest ? ' · ' : ''
  title = title ? fit(ctx, title, W * 0.62 - ctx.measureText(sep + rest).width) : ''
  const text = title + sep + rest
  const w = ctx.measureText(text).width + 2 * padX
  const h = size * 1.9
  // At the top it goes right: sources put their watermark in the top-left.
  const x = o.position === 'top' ? W - margin - w : margin
  const y = o.position === 'top' ? margin : H - margin - h
  ctx.fillStyle = 'rgba(19,17,15,0.8)'
  ctx.beginPath()
  ctx.roundRect(x, y, w, h, 0.8 * u)
  ctx.fill()
  ctx.fillStyle = INK
  ctx.textBaseline = 'middle'
  ctx.fillText(text, x + padX, y + h / 2)
}

// "Grand-Blue-Season-3_EP06_12m34s.png"
export function fileName(c: Capture, type: ImageType): string {
  const slug = (c.title || `anime-${c.animeId}`)
    .normalize('NFKD')
    .replace(/[^\w]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 60)
  const t = Math.floor(c.time)
  const ep = String(c.episode).padStart(2, '0')
  const stamp = `${Math.floor(t / 60)}m${String(t % 60).padStart(2, '0')}s`
  return `${slug}_EP${ep}_${stamp}.${type === 'image/png' ? 'png' : 'jpg'}`
}
