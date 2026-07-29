// Mirror of backend/diag/types.go. Fields are JSON-serialised as the Go
// `diag.Entry` / `diag.Summary` / `diag.OpStat` structs.
export type DiagLevel = 'DEBUG' | 'INFO' | 'WARN' | 'ERROR'

export interface DiagCaller {
  file: string
  line: number
}

export interface DiagEntry {
  ts: string
  level: DiagLevel
  tag: string
  msg: string
  fields?: Record<string, unknown>
  caller?: DiagCaller
  goroutine?: string
  dedup_count: number
  dropped: number
}

export interface DiagOpStat {
  name: string
  count: number
  p50Ms: number
  p95Ms: number
  p99Ms: number
  lastErr?: string
}

export interface DiagSummary {
  levels: Record<DiagLevel, number>
  ops: DiagOpStat[]
  droppedTotal: number
  dedupTotal: number
}
