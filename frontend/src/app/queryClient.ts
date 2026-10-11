import { QueryClient } from '@tanstack/vue-query'

export function createAppQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        networkMode: 'always',
        retry: false,
        refetchOnWindowFocus: false,
        staleTime: 10_000,
      },
      mutations: { networkMode: 'always', retry: false },
    },
  })
}
