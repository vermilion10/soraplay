// Renders the app icons (PNG) referenced by public/manifest.json. Run after changing them:
//   node scripts/make-icons.mjs
import { writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { Resvg } from '@resvg/resvg-js'

const pub = join(import.meta.dirname, '..', 'public')
const bg = '#13110f'
const accent = '#ff5a36'
// Play button in the accent colour on the app's background.
const mark = `<circle cx="32" cy="32" r="18" fill="${accent}"/><path d="M27 23.5v17L41 32z" fill="${bg}"/>`

const any = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="14" fill="${bg}"/>${mark}</svg>`
const maskable = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" fill="${bg}"/><g transform="translate(32 32) scale(.8) translate(-32 -32)">${mark}</g></svg>`

const render = (svg, size, file) =>
  writeFileSync(join(pub, file), new Resvg(svg, { fitTo: { mode: 'width', value: size } }).render().asPng())

writeFileSync(join(pub, 'icon.svg'), any)
render(any, 192, 'icon-192.png')
render(any, 512, 'icon-512.png')
render(maskable, 512, 'icon-maskable-512.png')
render(maskable, 180, 'apple-touch-icon.png')
console.log('icons written')
