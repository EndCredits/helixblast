const BASE = '/api/v1'

export interface Database {
  name: string
  type: string
  description: string
  last_updated: string
}

export interface HealthInfo {
  status: string
  version: string
  concurrent_capacity: number
  storage_backend: string
}

export interface JobItem {
  job_id: string
  status: string
  queue_pos: number
  program: string
  database: string
  created_at: string
}

export interface HSP {
  query_start: number
  query_end: number
  subject_start: number
  subject_end: number
  query_seq: string
  subject_seq: string
}

export interface Hit {
  subject_id: string
  identity: number
  coverage: number
  e_value: string
  total_score: number
  alignments: HSP[]
}

export interface JobDetail {
  job_id: string
  status: string
  queue_pos: number
  program: string
  database: string
  created_at: string
  updated_at: string
  result: BlastResult | null
  error?: string
  progress?: string
}

export interface BlastResult {
  job_id: string
  status: string
  database: string
  program: string
  results: Hit[]
}

export async function fetchHealth(): Promise<HealthInfo> {
  const res = await fetch('/health')
  if (!res.ok) throw new Error('Health check failed')
  return res.json()
}

export async function fetchDatabases(): Promise<Database[]> {
  const res = await fetch(`${BASE}/databases`)
  if (!res.ok) throw new Error('Failed to fetch databases')
  return res.json()
}

export async function fetchJobs(): Promise<JobItem[]> {
  const res = await fetch(`${BASE}/jobs`)
  if (!res.ok) throw new Error('Failed to fetch jobs')
  return res.json()
}

export async function fetchJob(id: string): Promise<JobDetail> {
  const res = await fetch(`${BASE}/jobs/${id}`)
  if (!res.ok) throw new Error('Job not found')
  return res.json()
}

export async function createJob(data: {
  fasta: string
  program: string
  db: string
  template?: string
  advanced_params?: Record<string, string>
}): Promise<{ job_id: string; status: string; queue_pos: number }> {
  const res = await fetch(`${BASE}/jobs`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
  if (!res.ok) {
    const err = await res.json()
    throw new Error(err.error || 'Failed to submit job')
  }
  return res.json()
}

export async function cancelJob(id: string): Promise<void> {
  const res = await fetch(`${BASE}/jobs/${id}`, {
    method: 'DELETE',
  })
  if (!res.ok) throw new Error('Failed to cancel job')
}

export interface TranscriptResult {
  transcript_id: string
  database: string
  chromosome: string
  start: number
  end: number
  strand: string
  type: string
  gene_id: string
  sequence: string
  scan_start: number
  scan_end: number
  regions?: {
    exons: { start: number; end: number }[]
    cdss: { start: number; end: number }[]
  }
  related?: {
    transcripts: string[]
    cdss: string[]
    exons: string[]
  }
}

export async function lookupTranscript(
  db: string,
  transcript: string,
): Promise<TranscriptResult> {
  const res = await fetch(
    `${BASE}/transcripts?db=${encodeURIComponent(db)}&transcript=${encodeURIComponent(transcript)}`,
  )
  if (!res.ok) {
    const err = await res.json()
    throw new Error(err.error || 'Transcript lookup failed')
  }
  return res.json()
}
