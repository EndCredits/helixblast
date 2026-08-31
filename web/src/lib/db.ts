const DB = 'helixblast'
const VER = 2
const STORE = 'cache'
const TTL = 24 * 60 * 60 * 1000

interface Entry {
  key: string
  type: string
  data: any
  time: number
}

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const r = indexedDB.open(DB, VER)
    r.onupgradeneeded = () => {
      if (!r.result.objectStoreNames.contains(STORE)) {
        r.result.createObjectStore(STORE, { keyPath: 'key' })
      }
    }
    r.onsuccess = () => resolve(r.result)
    r.onerror = () => reject(r.error)
  })
}

export async function saveJobMeta(job: { job_id: string; [k: string]: any }) {
  const db = await open()
  const e: Entry = { key: `job:${job.job_id}`, type: 'job', data: job, time: Date.now() }
  return new Promise<void>((resolve) => {
    const tx = db.transaction(STORE, 'readwrite')
    const req = tx.objectStore(STORE).add(e)
    tx.oncomplete = () => resolve()
    req.onerror = () => resolve()
  })
}

export async function saveJobFull(job: { job_id: string; [k: string]: any }) {
  const db = await open()
  const e: Entry = { key: `job:${job.job_id}`, type: 'job', data: job, time: Date.now() }
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite')
    tx.objectStore(STORE).put(e)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

export async function loadJobs(): Promise<any[]> {
  const db = await open()
  const cutoff = Date.now() - TTL
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite')
    const req = tx.objectStore(STORE).getAll()
    req.onsuccess = () => {
      const entries = (req.result || []) as Entry[]
      const valid: any[] = []
      for (const e of entries) {
        if (e.type !== 'job') continue
        if (e.time < cutoff) {
          tx.objectStore(STORE).delete(e.key)
        } else {
          valid.push({ ...e.data, _cached: true })
        }
      }
      resolve(valid)
    }
    req.onerror = () => reject(req.error)
  })
}

export async function loadCachedJob(id: string): Promise<any | null> {
  const db = await open()
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readonly')
    const req = tx.objectStore(STORE).get(`job:${id}`)
    req.onsuccess = () => {
      const entry = req.result as Entry | undefined
      if (entry && entry.time > Date.now() - TTL) {
        resolve({ ...entry.data, _cached: true })
      } else {
        resolve(null)
      }
    }
    req.onerror = () => reject(req.error)
  })
}

export async function cacheClear() {
  const db = await open()
  return new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite')
    const store = tx.objectStore(STORE)
    const req = store.getAll()
    req.onsuccess = () => {
      const entries = (req.result || []) as Entry[]
      for (const e of entries) {
        if (e.type !== 'setting') {
          store.delete(e.key)
        }
      }
    }
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}

// Settings are simple key-value pairs persisted in IndexedDB (same store,
// type 'setting'), NOT subject to the 24h TTL that applies to jobs.
// Unlike localStorage, they survive across browsers/devices on the same origin
// and follow the project's IndexedDB-first convention.
// Exception: the theme preference lives in localStorage (web/src/themeMode.tsx)
// because it must be resolved before first paint to avoid a theme flash —
// see architecture → Dark mode.

export async function getSetting(key: string): Promise<any | null> {
  const db = await open()
  return new Promise((resolve) => {
    const tx = db.transaction(STORE, 'readonly')
    const req = tx.objectStore(STORE).get(`setting:${key}`)
    req.onsuccess = () => {
      const entry = req.result as Entry | undefined
      if (entry && entry.type === 'setting') {
        resolve(entry.data)
      } else {
        resolve(null)
      }
    }
    req.onerror = () => resolve(null)
  })
}

export async function setSetting(key: string, data: any): Promise<void> {
  const db = await open()
  const e: Entry = { key: `setting:${key}`, type: 'setting', data, time: Date.now() }
  return new Promise((resolve, reject) => {
    const tx = db.transaction(STORE, 'readwrite')
    tx.objectStore(STORE).put(e)
    tx.oncomplete = () => resolve()
    tx.onerror = () => reject(tx.error)
  })
}
