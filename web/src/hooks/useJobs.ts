import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { useEffect, useRef } from 'react'
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
    refetchInterval: 3000,
  })
}

export function useJobSSE(jobId: string | null) {
  const qc = useQueryClient()
  const eventSourceRef = useRef<EventSource | null>(null)

  useEffect(() => {
    if (!jobId) return

    const es = new EventSource(`/api/v1/jobs/${jobId}/events`)
    eventSourceRef.current = es

    es.onmessage = (event) => {
      try {
        const data: JobDetail = JSON.parse(event.data)
        qc.setQueryData(['job', jobId], data)

        if (['success', 'failed', 'cancelled'].includes(data.status)) {
          es.close()
          qc.invalidateQueries({ queryKey: ['jobs'] })
        }
      } catch {}
    }

    es.onerror = () => {
      es.close()
    }

    return () => {
      es.close()
    }
  }, [jobId, qc])

  return useQuery<JobDetail | null>({
    queryKey: ['job', jobId],
    queryFn: () => {
      if (!jobId) return null
      return api.fetchJob(jobId)
    },
    enabled: !!jobId,
    staleTime: Infinity,
  })
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
