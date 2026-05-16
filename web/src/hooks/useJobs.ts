import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import * as api from '../api/client'

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
    refetchInterval: 2000,
  })
}

export function useJob(id: string | null) {
  return useQuery({
    queryKey: ['job', id],
    queryFn: () => api.fetchJob(id!),
    enabled: !!id,
    refetchInterval: (query) => {
      const data = query.state.data
      if (!data) return 2000
      if (['success', 'failed', 'cancelled'].includes(data.status)) return false
      return 2000
    },
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
