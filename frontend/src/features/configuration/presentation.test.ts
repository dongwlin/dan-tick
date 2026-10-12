import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import { Kind as TaskKind } from '@bindings/github.com/dongwlin/dan-tick/internal/task/models'
import type { Room, Task } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { kindText, nextRunText, roomText, scheduleText, shortTime } from './presentation'

function task(overrides: Partial<Task> = {}): Task {
  return {
    id: 1,
    roomId: 1,
    name: '每日晚安问候',
    kind: TaskKind.Single,
    messages: ['晚上好'],
    intervalSeconds: null,
    maxCount: null,
    maxDurationSeconds: null,
    schedule: {
      kind: ScheduleKind.Daily,
      timezone: 'Asia/Shanghai',
      localDate: '',
      localTime: '22:00:00',
      weekdays: 0,
    },
    enabled: true,
    nextDueAt: null,
    createdAt: 1,
    updatedAt: 1,
    ...overrides,
  }
}

const room: Room = {
  id: 1,
  platformRoomId: '22625025',
  title: '直播标题',
  anchorName: '主播',
  coverUrl: '',
  remark: '星河放映室',
  taskCount: 1,
}

beforeEach(() => {
  vi.useFakeTimers()
  // Friday 2026-10-09 20:00 in Asia/Shanghai.
  vi.setSystemTime(new Date('2026-10-09T12:00:00Z'))
})

afterEach(() => {
  vi.useRealTimers()
})

it('formats schedule labels in the shape shown by the prototype', () => {
  expect(shortTime('22:00:00')).toBe('22:00')
  expect(
    scheduleText(
      task({
        schedule: {
          kind: ScheduleKind.Daily,
          timezone: 'Asia/Shanghai',
          localDate: '',
          localTime: '09:00:00',
          weekdays: 0,
        },
      }),
    ),
  ).toBe('每天 09:00')
  expect(
    scheduleText(
      task({
        schedule: {
          kind: ScheduleKind.Weekly,
          timezone: 'Asia/Shanghai',
          localDate: '',
          localTime: '21:30:00',
          // Monday, Wednesday, Friday.
          weekdays: 1 | 4 | 16,
        },
      }),
    ),
  ).toBe('每周一、三、五 21:30')
  expect(
    scheduleText(
      task({
        schedule: {
          kind: ScheduleKind.Once,
          timezone: 'Asia/Shanghai',
          localDate: '2026-10-18',
          localTime: '19:00:00',
          weekdays: 0,
        },
      }),
    ),
  ).toBe('2026-10-18 19:00')
})

it('uses the room remark first and hides the next run for disabled tasks', () => {
  expect(roomText(1, [room])).toBe('星河放映室')
  expect(roomText(9, [room])).toBe('直播间 #9')

  expect(nextRunText(task({ nextDueAt: null }))).toBe('—')
  expect(nextRunText(task({ enabled: false, nextDueAt: 1_000_000 }))).toBe('—')
  expect(nextRunText(task({ nextDueAt: Date.UTC(2026, 9, 9, 14, 0) }))).toBe('今天 22:00')
  expect(nextRunText(task({ nextDueAt: Date.UTC(2026, 9, 10, 14, 0) }))).toBe('明天 22:00')
  expect(nextRunText(task({ nextDueAt: Date.UTC(2026, 9, 12, 14, 0) }))).toBe('10/12 22:00')
})

it('labels single and loop sending modes consistently with the prototype', () => {
  expect(kindText(task())).toBe('单次发送 · 1 条消息')
  expect(
    kindText(
      task({
        kind: TaskKind.Loop,
        intervalSeconds: 15,
        maxCount: 20,
        messages: ['一', '二'],
      }),
    ),
  ).toBe('循环 · 每隔 15 秒 · 最多 20 条')
  expect(
    kindText(
      task({
        kind: TaskKind.Loop,
        intervalSeconds: 30,
        maxCount: null,
        messages: ['一'],
      }),
    ),
  ).toBe('循环 · 每隔 30 秒 · 最多 不限 条')
})
