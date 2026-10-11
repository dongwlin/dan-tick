import {
  createIcons, LayoutDashboard, ListTodo, Radio, History, Settings2,
  Minus, Plus, ChevronRight, ArrowUpRight, CircleCheck, CircleAlert,
  Play, Square, Pencil, Trash2, CalendarClock, Search, Activity, Check,
  Moon, Sun, ExternalLink, UserRound, Bell, Power, Info, Send,
  Shield, SlidersHorizontal, X, ArrowLeft, Clock3, MessageSquareText,
  Copy, RefreshCcw, ChevronDown, LayoutGrid, List,
} from 'lucide'
import { displayNext, scheduleLabel, type Page, type LogStatus, type RunLog, type Task } from './data'
import { state } from './state'

const iconSet = {
  LayoutDashboard, ListTodo, Radio, History, Settings2,
  Minus, Plus, ChevronRight, ArrowUpRight, CircleCheck, CircleAlert,
  Play, Square, Pencil, Trash2, CalendarClock, Search, Activity, Check,
  Moon, Sun, ExternalLink, UserRound, Bell, Power, Info, Send,
  Shield, SlidersHorizontal, X, ArrowLeft, Clock3, MessageSquareText,
  Copy, RefreshCcw, ChevronDown, LayoutGrid, List,
}
export function renderIcons(): void {
  createIcons({ icons: iconSet, attrs: { 'stroke-width': 1.7 } })
}

const titles: Record<Page, { title: string; description: string }> = {
  dashboard: { title: '总览', description: '' },
  tasks: { title: '弹幕任务', description: '' },
  rooms: { title: '直播间', description: '' },
  history: { title: '执行记录', description: '' },
  settings: { title: '设置', description: '' },
  editor: { title: '任务', description: '' },
}

export function h(value: string | number): string {
  return String(value).replace(/[&<>"']/g, character => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[character]!)
}

export function ico(name: string, className = ''): string {
  return `<i data-lucide="${name}" class="${className}" aria-hidden="true"></i>`
}

export function roomName(id: string): string {
  return state.rooms.find(room => room.id === id)?.name ?? `房间 ${id}`
}

export function statusPill(label: string, variant: string): string {
  return `<span class="pill pill--${variant}"><span class="pill-dot"></span>${label}</span>`
}

export function runLabel(status: LogStatus): string {
  return ({ success: '已完成', failed: '已失败', stopped: '已停止', missed: '已错过' })[status]
}

function logPill(status: LogStatus): string {
  return statusPill(runLabel(status), status === 'success' ? 'success' : status === 'failed' ? 'danger' : 'muted')
}

function taskPill(task: Task): string {
  return task.state === 'running' ? statusPill('运行中', 'accent') : task.enabled ? statusPill('已启用', 'success') : statusPill('已停用', 'muted')
}

export function sectionTitle(title: string, sub = '', action = ''): string {
  return `<div class="section-heading"><div><h2>${title}</h2>${sub ? `<p>${sub}</p>` : ''}</div>${action}</div>`
}

export function pageHeading(actions = ''): string {
  const heading = state.page === 'editor' ? { title: state.editingId ? '编辑任务' : '新建任务', description: titles.editor.description } : titles[state.page]
  return `<div class="page-heading"><div><h1>${heading.title}</h1>${heading.description ? `<p>${heading.description}</p>` : ''}</div>${actions ? `<div class="heading-actions">${actions}</div>` : ''}</div>`
}

export function taskRow(task: Task, compact = false): string {
  return `<div class="task-row ${compact ? 'task-row--compact' : ''}">
    <div class="task-primary"><strong>${h(task.name)}</strong><span>${h(roomName(task.roomId))} <span class="sub-separator">·</span> ${h(scheduleLabel(task))}</span></div>
    <div class="task-next"><small>下次执行</small><strong>${displayNext(task)}</strong></div>
    <div class="task-status">${taskPill(task)}</div>
    <div class="task-actions">
      ${task.state === 'running' ? `<button class="icon-button icon-button--danger" type="button" data-action="stop-task" data-id="${h(task.id)}" title="模拟停止" aria-label="停止 ${h(task.name)}">${ico('square')}</button>` : `<button class="icon-button" type="button" data-action="run-task" data-id="${h(task.id)}" title="模拟立即执行" aria-label="立即执行 ${h(task.name)}">${ico('play')}</button>`}
      <button class="icon-button" type="button" data-action="edit-task" data-id="${h(task.id)}" title="编辑任务" aria-label="编辑 ${h(task.name)}">${ico('pencil')}</button>
    </div>
  </div>`
}

export function logTable(logs: RunLog[], compact = false): string {
  return `<div class="table-wrap"><table class="data-table"><thead><tr><th>任务 / 直播间</th><th>执行时间</th><th>发送结果</th><th>状态</th><th><span class="sr-only">操作</span></th></tr></thead><tbody>
    ${logs.length ? logs.map(log => `<tr><td><strong>${h(log.taskName)}</strong><small>${h(roomName(log.roomId))}</small></td><td class="mono">${h(log.date)}</td><td><span class="success-count">${log.sent} 成功</span><span class="muted-count"> / ${log.failed} 失败</span></td><td>${logPill(log.status)}</td><td><button class="icon-button" type="button" data-action="log-detail" data-id="${h(log.id)}" title="查看执行详情" aria-label="查看 ${h(log.taskName)} 的执行详情">${ico('chevron-right')}</button></td></tr>${!compact && state.selectedLog === log.id ? `<tr class="log-detail"><td colspan="5"><div><strong>执行详情</strong><p>${h(log.detail)}</p><span>这是模拟记录，未发生真实网络请求。</span></div></td></tr>` : ''}`).join('') : '<tr><td colspan="5" class="empty-table">没有符合筛选条件的记录。</td></tr>'}
  </tbody></table></div>`
}
