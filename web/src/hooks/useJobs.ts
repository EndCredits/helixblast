import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef, useState, useCallback } from 'react'
import * as api from '../api/client'
import type { JobDetail } from '../api/client'

export function useHealth() {
  return useQuery({ queryKey: ['health'], queryFn: api.fetchHealth })
}

export function useDatabases() {
  return useQuery({ queryKey: ['databases'], queryFn: api.fetchDatabases })
}

export function useJobs() {
  return useQuery({
    queryKey: ['jobs'],
    queryFn: api.fetchJobs,
    refetchInterval: (query) => {
      const jobs = query.state.data
      if (!jobs || jobs.length === 0) return false
      const hasActive = jobs.some(
        (j) => !['success', 'failed', 'cancelled'].includes(j.status),
      )
      return hasActive ? 3000 : false
    },
  })
}

export function useJobSSE(jobId: string | null) {
  const qc = useQueryClient()
  const retryCountRef = useRef(0)
  const maxRetries = 10
  const eventSourceRef = useRef<EventSource | null>(null)
  const fallbackTimerRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const [sseState, setSseState] = useState<'connecting' | 'connected' | 'reconnecting' | 'disconnected'>('connecting')

  const getBackoff = useCallback((retry: number): number => {
    return Math.min(1000 * Math.pow(2, retry), 30000)
  }, [])

  const stopAll = useCallback(() => {
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

    retryCountRef.current = 0
    setSseState('connecting')

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
            stopAll()
            setSseState('disconnected')
            qc.invalidateQueries({ queryKey: ['jobs'] })
          }
        } catch {}
      }

      es.onerror = () => {
        const readyState = es.readyState
        if (readyState === EventSource.CLOSED) {
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

    const fallbackToPolling = (id: string) => {
      const fetchOnce = async () => {
        try {
          const data = await api.fetchJob(id)
          qc.setQueryData(['job', id], data)
          if (['success', 'failed', 'cancelled'].includes(data.status)) {
            if (fallbackTimerRef.current) {
              clearInterval(fallbackTimerRef.current)
            }
          }
        } catch {
          if (fallbackTimerRef.current) {
            clearInterval(fallbackTimerRef.current)
          }
        }
      }

      fetchOnce()
      fallbackTimerRef.current = setInterval(fetchOnce, 5000)
    }

    connect()

    return () => {
      stopAll()
    }
  }, [jobId, qc, getBackoff, stopAll])

  return {
    ...useQuery<JobDetail | null>({
      queryKey: ['job', jobId],
      queryFn: () => {
        if (!jobId) return null
        return api.fetchJob(jobId)
      },
      enabled: !!jobId,
      staleTime: Infinity,
    }),
    sseState,
  }
}

export function useCreateJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.createJob,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
}

export function useCancelJob() {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: api.cancelJob,
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['jobs'] })
    },
  })
}
