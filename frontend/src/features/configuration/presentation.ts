import type { Room, Task } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import { Kind as TaskKind } from '@bindings/github.com/dongwlin/dan-tick/internal/task/models'

// Monday bit 0 through Sunday bit 6, matching schedule.Rule.
const weekdayNames = ['一', '二', '三', '四', '五', '六', '日']

export function shortTime(value: string): string {
  const match = /^(\d{1,2}):(\d{2})/.exec(value.trim())
  if (!match) return value
  return `${match[1]?.padStart(2, '0')}:${match[2]}`
}

export function scheduleText(task: Task): string {
  const time = shortTime(task.schedule.localTime)
  if (task.schedule.kind === ScheduleKind.Once) {
    return `${task.schedule.localDate || '未设置日期'} ${time}`
  }
  if (task.schedule.kind === ScheduleKind.Weekly) {
    const days = weekdayNames.filter((_, index) => (task.schedule.weekdays & (1 << index)) !== 0)
    return days.length ? `每周${days.join('、')} ${time}` : `每周 ${time}`
  }
  return `每天 ${time}`
}

export function roomText(roomId: number, rooms: Room[] | null | undefined): string {
  const room = rooms?.find((candidate) => candidate.id === roomId)
  if (!room) return `直播间 #${roomId}`
  const anchor = room.anchorName?.trim() || '未知主播'
  return `${anchor} · ${room.platformRoomId || roomId}`
}

export function kindText(task: Task): string {
  if (task.kind === TaskKind.Single) return '单次发送 · 1 条消息'
  return `循环 · 每隔 ${task.intervalSeconds ?? 0} 秒 · 最多 ${task.maxCount ?? '不限'} 条`
}

interface CalendarParts {
  date: string
  time: string
}

function calendarParts(value: Date, timeZone: string): CalendarParts {
  const parts = new Intl.DateTimeFormat('en-CA', {
    timeZone,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
  }).formatToParts(value)
  const lookup = new Map(parts.map((part) => [part.type, part.value]))
  return {
    date: `${lookup.get('year')}-${lookup.get('month')}-${lookup.get('day')}`,
    time: `${lookup.get('hour')}:${lookup.get('minute')}`,
  }
}

export function nextRunText(task: Task): string {
  if (!task.enabled || task.nextDueAt === null) return '—'
  const date = new Date(task.nextDueAt)
  if (Number.isNaN(date.getTime())) return '—'
  const timeZone = task.schedule.timezone || 'Asia/Shanghai'
  const target = calendarParts(date, timeZone)
  const now = new Date()
  const today = calendarParts(now, timeZone)
  const tomorrow = calendarParts(new Date(now.getTime() + 24 * 60 * 60 * 1000), timeZone)
  if (target.date === today.date) return `今天 ${target.time}`
  if (target.date === tomorrow.date) return `明天 ${target.time}`
  const monthDay = new Intl.DateTimeFormat('zh-CN', {
    timeZone,
    month: 'numeric',
    day: 'numeric',
  }).format(date)
  return `${monthDay} ${target.time}`
}
