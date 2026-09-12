import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../contexts/AuthContext'
import { ApiError } from '../services/api'

export function useResource<T>(key: readonly unknown[], loader: () => Promise<T>, interval?: number) {
  const { user } = useAuth()
  return useQuery({ queryKey: [user?.id, ...key], queryFn: loader, enabled: !!user, refetchInterval: interval,
    retry: (count, error) => count < 1 && !(error instanceof ApiError && error.status < 500) })
}
export function useAction() {
  const cache = useQueryClient()
  const { user } = useAuth()
  return useMutation({ mutationFn: (action: () => Promise<unknown>) => action(),
    onSuccess: () => cache.invalidateQueries({ queryKey: [user?.id] }) })
}
