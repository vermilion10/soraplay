import { useEffect, useState } from 'react'

// Scene captures are kept in this browser (IndexedDB) until saved or deleted.
// They never go to the server. When IndexedDB is unavailable (private
// windows on some browsers) they are kept in memory for this visit instead.

export type Capture = {
  id: string
  blob: Blob // PNG of the frame at the video's own resolution
  animeId: number
  title: string
  episode: number
  time: number // seconds into the episode
  capturedAt: number // Unix milliseconds
  width: number
  height: number
}

const DB = 'soraplay'
const STORE = 'captures'
const CHANGED = 'soraplay:captures'

let memory: Capture[] = []
let dbPromise: Promise<IDBDatabase | null> | null = null

function db(): Promise<IDBDatabase | null> {
  dbPromise ??= new Promise((resolve) => {
    try {
      const req = indexedDB.open(DB, 1)
      req.onupgradeneeded = () => req.result.createObjectStore(STORE, { keyPath: 'id' })
      req.onsuccess = () => resolve(req.result)
      req.onerror = () => resolve(null)
    } catch {
      resolve(null)
    }
  })
  return dbPromise
}

function run<T>(mode: IDBTransactionMode, fn: (s: IDBObjectStore) => IDBRequest<T>): Promise<T | null> {
  return db().then(
    (d) =>
      new Promise((resolve) => {
        if (!d) return resolve(null)
        try {
          const req = fn(d.transaction(STORE, mode).objectStore(STORE))
          req.onsuccess = () => resolve(req.result)
          req.onerror = () => resolve(null)
        } catch {
          resolve(null)
        }
      }),
  )
}

export async function addCapture(c: Capture): Promise<void> {
  if ((await run('readwrite', (s) => s.put(c))) === null) memory = [...memory, c]
  window.dispatchEvent(new Event(CHANGED))
}

export async function deleteCapture(id: string): Promise<void> {
  await run('readwrite', (s) => s.delete(id))
  memory = memory.filter((c) => c.id !== id)
  window.dispatchEvent(new Event(CHANGED))
}

export async function listCaptures(): Promise<Capture[]> {
  const stored = ((await run('readonly', (s) => s.getAll())) as Capture[] | null) ?? []
  return [...stored, ...memory].sort((a, b) => b.capturedAt - a.capturedAt)
}

// The captures held in this browser, newest first, kept current.
export function useCaptures(): Capture[] | null {
  const [list, setList] = useState<Capture[] | null>(null)
  useEffect(() => {
    let live = true
    const load = () => listCaptures().then((l) => live && setList(l))
    load()
    window.addEventListener(CHANGED, load)
    return () => {
      live = false
      window.removeEventListener(CHANGED, load)
    }
  }, [])
  return list
}

// Draws the frame on screen into a PNG. Throws when the browser refuses to
// read the video's pixels (a cross-origin stream without CORS).
export function grabFrame(v: HTMLVideoElement): Promise<Blob> {
  const c = document.createElement('canvas')
  c.width = v.videoWidth
  c.height = v.videoHeight
  const ctx = c.getContext('2d')
  if (!ctx || !c.width) return Promise.reject(new Error('The video has no picture yet.'))
  ctx.drawImage(v, 0, 0)
  return new Promise((resolve, reject) => {
    try {
      c.toBlob((b) => (b ? resolve(b) : reject(new Error('The browser could not encode the frame.'))), 'image/png')
    } catch {
      reject(new Error('This stream does not allow captures.'))
    }
  })
}
