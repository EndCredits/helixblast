import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState, useCallback } from 'react'
import * as api from '../api/client'
import { saveJobFull, loadCachedJob } from '../lib/db'
import type { JobDetail } from '../api/client'

export function useHealth() {
  return useQuery({ queryKey: ['health'], queryFn: api.fetchHealth })
}

export function useDatabases() {
  return useQuery({ queryKey: ['databases'], queryFn: api.fetchDatabases })
}

export function useJobSSE(jobId: string | null, onTerminal?: () => void) {
  const qc = useQueryClient()
  const retryCountRef = useRef(0)
  const doneRef = useRef(false)
  const maxRetries = 10
  const eventSourceRef = useRef<EventSource | null>(null)
  const fallbackTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const [sseState, setSseState] = useState<'connecting' | 'connected' | 'reconnecting' | 'disconnected'>('connecting')

  const getBackoff = useCallback((retry: number): number => {
    return Math.min(1000 * Math.pow(2, retry), 30000)
  }, [])

  const stopAll = useCallback((done = false) => {
    if (done) doneRef.current = true
    if (eventSourceRef.current) {
      eventSourceRef.current.close()
      eventSourceRef.current = null
    }
    if (fallbackTimerRef.current) {
      clearInterval(fallbackTimerRef.current)
      fallbackTimerRef.current = null
    }
  }, [])

  useEffect(() => {
    if (!jobId) return

    const existing = qc.getQueryData(['job', jobId]) as (JobDetail & { _cached?: boolean }) | undefined
    if (existing && ['success', 'failed', 'cancelled'].includes(existing.status)) {
      if (existing.result) { setSseState('disconnected'); return }
      if (existing._cached) { setSseState('disconnected'); return }
    }

    const fallbackToPolling = (id: string) => {
      const fetchOnce = async () => {
        try {
          const cached = await loadCachedJob(id).catch(() => null)
          if (cached && ['success', 'failed', 'cancelled'].includes(cached.status) && cached.result) {
            qc.setQueryData(['job', id], cached as JobDetail)
            if (fallbackTimerRef.current) clearInterval(fallbackTimerRef.current)
            saveJobFull(cached).catch(() => {})
            return
          }
          if (cached) {
            qc.setQueryData(['job', id], cached as JobDetail)
          }
        } catch {
          if (fallbackTimerRef.current) clearInterval(fallbackTimerRef.current)
        }
      }
      fetchOnce()
      fallbackTimerRef.current = setInterval(fetchOnce, 5000)
    }

    const connect = () => {
      stopAll()

      const es = new EventSource(`/api/v1/jobs/${jobId}/events`)
      eventSourceRef.current = es

      es.onopen = () => {
        retryCountRef.current = 0
        setSseState('connected')
      }

      es.onmessage = (event) => {
        try {
          const data: JobDetail = JSON.parse(event.data)
          qc.setQueryData(['job', jobId], data)

          if (['success', 'failed', 'cancelled'].includes(data.status)) {
            stopAll(true)
            setSseState('disconnected')
            saveJobFull(data).catch(() => {})
            onTerminal?.()
          }
        } catch {}
      }

      es.onerror = () => {
        const readyState = es.readyState
        if (readyState === EventSource.CLOSED) {
          if (doneRef.current) {
            setSseState('disconnected')
            return
          }

          const cached = qc.getQueryData(['job', jobId]) as JobDetail | undefined
          if (cached && ['success', 'failed', 'cancelled'].includes(cached.status)) {
            doneRef.current = true
            setSseState('disconnected')
            return
          }

          retryCountRef.current++
          if (retryCountRef.current > maxRetries) {
            stopAll()
            setSseState('disconnected')
            fallbackToPolling(jobId)
            return
          }

          setSseState('reconnecting')
          const delay = getBackoff(retryCountRef.current)
          setTimeout(connect, delay)
        }
      }
    }

    const startStreaming = () => {
      retryCountRef.current = 0
      doneRef.current = false
      setSseState('connecting')
      connect()
    }

    // Check IndexedDB directly — skip SSE if terminal result already cached
    loadCachedJob(jobId).then((cached) => {
      if (cached && ['success', 'failed', 'cancelled'].includes(cached.status) && cached.result) {
        setSseState('disconnected')
        return
      }
      startStreaming()
    }).catch(() => {
      startStreaming()
    })

    return () => {
      stopAll()
    }
  }, [jobId, qc, getBackoff, stopAll, onTerminal])

  return {
    ...useQuery<JobDetail | null>({
      queryKey: ['job', jobId],
      queryFn: async () => {
        if (!jobId) return null
        return loadCachedJob(jobId).catch(() => null)
      },
      enabled: !!jobId,
      staleTime: Infinity,
    }),
    sseState,
  }
}

export function useCreateJob() {
  return useMutation({
    mutationFn: api.createJob,
  })
}

export function useCancelJob() {
  return useMutation({
    mutationFn: api.cancelJob,
  })
}
