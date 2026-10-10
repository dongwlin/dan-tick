export type Page = 'dashboard' | 'tasks' | 'rooms' | 'history' | 'settings' | 'editor'
export type TaskKind = 'single' | 'loop'
export type ScheduleKind = 'daily' | 'weekly' | 'once'
export type RunState = 'idle' | 'running'
export type LogStatus = 'success' | 'failed' | 'stopped' | 'missed'

export interface Room {
  id: string
  name: string
  streamer: string
  title: string
  note: string
}

export interface Task {
  id: string
  name: string
  roomId: string
  kind: TaskKind
  messages: string[]
  schedule: ScheduleKind
  time: string
  date: string
  weekdays: number[]
  interval: number
  maxCount: number
  maxDuration: number
  enabled: boolean
  state: RunState
}

export interface RunLog {
  id: string
  taskId: string
  taskName: string
  roomId: string
  date: string
  status: LogStatus
  sent: number
  failed: number
  detail: string
}

export const initialRooms: Room[] = [
  { id: '22625025', name: '星河放映室', streamer: '星河', title: '晚间电台 · 随便聊聊', note: '日常互动' },
  { id: '3044248', name: '小岛的直播间', streamer: '小岛', title: '深夜游戏时间', note: '游戏直播' },
  { id: '21547895', name: '柠檬茶', streamer: '柠檬茶', title: '唱歌 / 点歌 / 闲聊', note: '音乐区' },
]

export const initialTasks: Task[] = [
  { id: 't1', name: '每日晚安问候', roomId: '22625025', kind: 'single', messages: ['晚上好，今天也辛苦啦！'], schedule: 'daily', time: '22:00', date: '', weekdays: [], interval: 15, maxCount: 1, maxDuration: 0, enabled: true, state: 'idle' },
  { id: 't2', name: '夜间弹幕循环', roomId: '3044248', kind: 'loop', messages: ['来了来了！', '今天也要开心～', '支持一下主播！'], schedule: 'weekly', time: '21:30', date: '', weekdays: [1, 3, 5], interval: 15, maxCount: 20, maxDuration: 300, enabled: true, state: 'idle' },
  { id: 't3', name: '周末直播提醒', roomId: '21547895', kind: 'single', messages: ['开播快乐！'], schedule: 'weekly', time: '20:00', date: '', weekdays: [6, 0], interval: 15, maxCount: 1, maxDuration: 0, enabled: true, state: 'idle' },
  { id: 't4', name: '活动开场弹幕', roomId: '22625025', kind: 'loop', messages: ['活动开始！', '大家一起来参与～'], schedule: 'once', time: '19:00', date: '2026-10-18', weekdays: [], interval: 20, maxCount: 10, maxDuration: 180, enabled: false, state: 'idle' },
]

export const initialLogs: RunLog[] = [
  { id: 'l1', taskId: 't1', taskName: '每日晚安问候', roomId: '22625025', date: '2026-10-09 22:00', status: 'success', sent: 1, failed: 0, detail: '按计划完成，成功发送 1 条弹幕。' },
  { id: 'l2', taskId: 't2', taskName: '夜间弹幕循环', roomId: '3044248', date: '2026-10-09 21:30', status: 'success', sent: 20, failed: 0, detail: '达到最大发送次数，执行结束。' },
  { id: 'l3', taskId: 't1', taskName: '每日晚安问候', roomId: '22625025', date: '2026-10-08 22:00', status: 'success', sent: 1, failed: 0, detail: '按计划完成，成功发送 1 条弹幕。' },
  { id: 'l4', taskId: 't2', taskName: '夜间弹幕循环', roomId: '3044248', date: '2026-10-07 21:30', status: 'stopped', sent: 7, failed: 0, detail: '用户手动停止，剩余弹幕未发送。' },
  { id: 'l5', taskId: 't3', taskName: '周末直播提醒', roomId: '21547895', date: '2026-10-03 20:00', status: 'failed', sent: 0, failed: 1, detail: '接口返回登录状态失效，请重新配置凭证。' },
]

export const dayNames = ['日', '一', '二', '三', '四', '五', '六']

export function scheduleLabel(task: Task): string {
  if (task.schedule === 'daily') return `每天 ${task.time}`
  if (task.schedule === 'once') return `${task.date || '未设置日期'} ${task.time}`
  return `每周${task.weekdays.map(day => dayNames[day]).join('、')} ${task.time}`
}

export function nextRun(task: Task): Date | null {
  if (!task.enabled || task.state === 'running') return null
  const [hours, minutes] = task.time.split(':').map(Number)
  const now = new Date()
  if (task.schedule === 'once') {
    const date = new Date(`${task.date}T${task.time}:00`)
    return date > now ? date : null
  }
  for (let i = 0; i <= 7; i++) {
    const date = new Date(now)
    date.setDate(now.getDate() + i)
    date.setHours(hours, minutes, 0, 0)
    if (date > now && (task.schedule === 'daily' || task.weekdays.includes(date.getDay()))) return date
  }
  return null
}

export function displayNext(task: Task): string {
  const date = nextRun(task)
  if (!date) return '—'
  const today = new Date()
  const tomorrow = new Date(today)
  tomorrow.setDate(today.getDate() + 1)
  const day = date.toDateString() === today.toDateString() ? '今天' : date.toDateString() === tomorrow.toDateString() ? '明天' : `${date.getMonth() + 1}/${date.getDate()}`
  return `${day} ${date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit', hour12: false })}`
}
