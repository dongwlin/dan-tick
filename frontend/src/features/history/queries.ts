import { useQuery } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  GetExecutionHistory,
  ListExecutionHistory,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/historyservice'

export const historyKeys = {
  all: ['history'] as const,
  page: (page: number, size: number) => ['history', 'page', page, size] as const,
  detail: (id: number) => ['history', 'detail', id] as const,
}

export function useExecutionHistoryPages(
  page: MaybeRefOrGetter<number>,
  pageSize: MaybeRefOrGetter<number>,
) {
  return useQuery({
    queryKey: computed(() => historyKeys.page(toValue(page), toValue(pageSize))),
    queryFn: () => ListExecutionHistory(toValue(page), toValue(pageSize)),
  })
}

export function useExecutionHistoryDetail(id: MaybeRefOrGetter<number>) {
  return useQuery({
    queryKey: computed(() => historyKeys.detail(toValue(id))),
    queryFn: () => GetExecutionHistory(toValue(id)),
    enabled: computed(() => Number.isSafeInteger(toValue(id)) && toValue(id) > 0),
  })
}
