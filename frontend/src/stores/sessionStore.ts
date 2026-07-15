import { defineStore } from 'pinia'
import { reactive } from 'vue'
import { EventsOn } from '../../wailsjs/runtime'
import type { SessionStatus } from '../types/session'

interface SessionData {
  id: string
  status: SessionStatus
  data: string[]
  // Monotonically increasing count of chunks ever appended to this session.
  // Unlike data.length, it is NOT reset when `data` is trimmed from the front,
  // so it is a stable sequence number for tracking replay position across
  // trims. See getDataFromChunk.
  seq: number
}

// Keep at most MAX_CHUNKS buffered per session; once exceeded, drop the oldest
// down to TRIM_TO. Trimming removes from the front, which is why consumers must
// track position by `seq` (a stable sequence number), not by array index.
const MAX_CHUNKS = 2000
const TRIM_TO = 1000

function pushChunk(s: SessionData, chunk: string) {
  s.data.push(chunk)
  s.seq++
  if (s.data.length > MAX_CHUNKS) {
    s.data.splice(0, s.data.length - TRIM_TO)
  }
}

// Module-level reactive state (shared across all store instances)
const sessionState = reactive<{
  sessions: Map<string, SessionData>
}>({
  sessions: new Map()
})

// Register event listeners once at module level
EventsOn('session:status', (payload: { id: string; status: SessionStatus }) => {
  let s = sessionState.sessions.get(payload.id)
  if (!s) {
    s = { id: payload.id, status: 'connecting', data: [], seq: 0 }
    sessionState.sessions.set(payload.id, s)
  }
  s.status = payload.status
})

EventsOn('session:data', (payload: { id: string; data: string }) => {
  let s = sessionState.sessions.get(payload.id)
  if (!s) {
    s = { id: payload.id, status: 'connecting', data: [], seq: 0 }
    sessionState.sessions.set(payload.id, s)
  }
  pushChunk(s, payload.data)
})

export const useSessionStore = defineStore('session', () => {
  function initSession(id: string) {
    const existing = sessionState.sessions.get(id)
    if (existing) {
      if (existing.status !== 'connected') {
        existing.status = 'connecting'
      }
    } else {
      sessionState.sessions.set(id, { id, status: 'connecting', data: [], seq: 0 })
    }
  }

  function updateStatus(id: string, status: SessionStatus) {
    const s = sessionState.sessions.get(id)
    if (s) {
      s.status = status
    }
  }

  function getStatus(id: string): SessionStatus {
    const s = sessionState.sessions.get(id)
    return s ? s.status : 'disconnected'
  }

  function appendData(id: string, chunk: string) {
    const s = sessionState.sessions.get(id)
    if (s) {
      pushChunk(s, chunk)
    }
  }

  function getData(id: string): string {
    const s = sessionState.sessions.get(id)
    if (!s) return ''
    const raw = s.data.join('')
    // When the buffer is trimmed (2000→1000 chunks), the joined string may
    // start mid-escape-sequence (e.g. DA2, OSC color queries). Those broken
    // fragments lack the \x1b prefix and xterm.js renders them as garbled text.
    // Find the first \n or \x1b to locate a safe restart boundary.
    const nl = raw.indexOf('\n')
    const esc = raw.indexOf('\x1b')
    if (esc >= 0 && esc < 4096 && (nl < 0 || esc < nl)) {
      return raw.slice(esc)
    }
    if (nl > 0 && nl < 4096) {
      return raw.slice(nl + 1)
    }
    return raw
  }

  // Total number of chunks ever received for this session. This is a stable,
  // monotonically increasing sequence number — pass it to getDataFromChunk to
  // resume from a remembered position even after the buffer has been trimmed.
  function getChunkCount(id: string): number {
    const s = sessionState.sessions.get(id)
    return s ? s.seq : 0
  }

  // Return all chunks with sequence number >= startSeq, joined. `startSeq` is a
  // value previously returned by getChunkCount. If the requested position has
  // already been trimmed away, returns everything still buffered (best effort,
  // so the most recent output — including the final prompt — is never dropped).
  function getDataFromChunk(id: string, startSeq: number): string {
    const s = sessionState.sessions.get(id)
    if (!s) return ''
    // Number of chunks already dropped from the front of `data`.
    const base = s.seq - s.data.length
    let idx = startSeq - base
    if (idx >= s.data.length) return ''
    if (idx < 0) idx = 0
    return s.data.slice(idx).join('')
  }

  function removeSession(id: string) {
    sessionState.sessions.delete(id)
  }

  return {
    sessions: sessionState.sessions,
    initSession,
    updateStatus,
    getStatus,
    appendData,
    getData,
    getChunkCount,
    getDataFromChunk,
    removeSession
  }
})
