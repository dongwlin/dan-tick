import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from '@tanstack/vue-query'
import { computed, toValue, type MaybeRefOrGetter } from 'vue'
import {
  AddRoom,
  CreateTask,
  DeleteRoom,
  DeleteTask,
  GetTask,
  ListRoomPage,
  ListRooms,
  ListTaskPage,
  SetTaskEnabled,
  UpdateRoom,
  UpdateTask,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/configurationservice'
import type { TaskInput } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'

export const configKeys = {
  rooms: ['rooms'] as const,
  tasks: ['tasks'] as const,
  detail: (id: number) => ['task', id] as const,
}

export function useRoomPages() {
  return useInfiniteQuery({
    queryKey: configKeys.rooms,
    initialPageParam: null as number | null,
    queryFn: ({ pageParam }) => ListRoomPage({ cursor: pageParam, limit: 12 }),
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  })
}

// The editor needs a complete room choice set, not only the first visible card page.
export function useRoomOptions() {
  return useQuery({
    queryKey: [...configKeys.rooms, 'options'],
    queryFn: async () => (await ListRooms()) ?? [],
  })
}

export function useTaskPages(search: MaybeRefOrGetter<string>, roomID: MaybeRefOrGetter<number>) {
  return useInfiniteQuery({
    queryKey: computed(() => [...configKeys.tasks, toValue(roomID), toValue(search).trim()]),
    initialPageParam: null as number | null,
    queryFn: ({ pageParam }) =>
      ListTaskPage({
        cursor: pageParam,
        limit: 12,
        roomId: toValue(roomID),
        search: toValue(search).trim(),
      }),
    getNextPageParam: (page) => page.nextCursor ?? undefined,
  })
}

export function useTaskDetail(id: MaybeRefOrGetter<number>) {
  return useQuery({
    queryKey: computed(() => configKeys.detail(toValue(id))),
    queryFn: () => GetTask(toValue(id)),
    enabled: computed(() => Number.isSafeInteger(toValue(id)) && toValue(id) > 0),
  })
}

function useConfigurationWrite<T, V>(write: (value: V) => Promise<T>) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: write,
    // A scheduler sync error may be returned after SQLite has committed. In
    // that case the UI must reload authoritative Go state even on rejection.
    onSettled: async () => {
      await Promise.all([
        client.invalidateQueries({ queryKey: configKeys.rooms }),
        client.invalidateQueries({ queryKey: configKeys.tasks }),
        client.invalidateQueries({ queryKey: ['task'] }),
      ])
    },
  })
}

export function useAddRoom() {
  return useConfigurationWrite((value: string) => AddRoom({ value }))
}
export function useUpdateRoom() {
  return useConfigurationWrite(({ id, remark }: { id: number; remark: string }) =>
    UpdateRoom(id, { remark }),
  )
}
export function useDeleteRoom() {
  return useConfigurationWrite((id: number) => DeleteRoom(id))
}
export function useSetTaskEnabled() {
  return useConfigurationWrite(({ id, enabled }: { id: number; enabled: boolean }) =>
    SetTaskEnabled(id, enabled),
  )
}
export function useDeleteTask() {
  return useConfigurationWrite((id: number) => DeleteTask(id))
}
export function useSaveTask(id?: number) {
  const client = useQueryClient()
  return useMutation({
    mutationFn: (input: TaskInput) =>
      id === undefined ? CreateTask(input) : UpdateTask(id, input),
    onSettled: async () => {
      await client.invalidateQueries({ queryKey: configKeys.tasks })
      await client.invalidateQueries({ queryKey: configKeys.rooms })
      await client.invalidateQueries({ queryKey: ['task'] })
    },
    onSuccess: (task) => {
      client.setQueryData(configKeys.detail(task.id), task)
    },
  })
}

// Only messages from desktop.classify are safe to show. A runtime/network
// exception, including one containing arbitrary input, never reaches the UI.
const publicMessages = new Set([
  '操作失败，请重试或查看诊断日志',
  '修改已保存，但定时调度同步失败，请重新启动应用后检查',
  '输入无效，请检查填写内容',
  '该直播间已添加',
  '指定直播间或任务不存在',
  '目标直播间不存在或已删除',
  '存在正在执行的任务，请先停止执行',
  '应用正在关闭，暂时无法更改任务',
  '直播间验证暂不可用，请稍后再试',
  '操作已取消或超时',
  '系统安全凭证存储不可用，请检查系统密钥环',
  '应用正在关闭，暂时无法执行任务',
  '该执行已结束或不在当前进程中运行，请刷新状态',
])
export function bindingError(error: unknown): string {
  if (!error || typeof error !== 'object') return '操作失败，请重试或查看诊断日志'
  const message = 'message' in error ? error.message : undefined
  return typeof message === 'string' && publicMessages.has(message)
    ? message
    : '操作失败，请重试或查看诊断日志'
}
