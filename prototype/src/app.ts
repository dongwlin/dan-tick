import { type Page, type Room } from './data'
import { state, blankTask } from './state'
import { sidebar, titlebar } from './shell'
import { h, ico, renderIcons } from './ui'
import { dashboard } from './pages/dashboard'
import { tasksPage } from './pages/tasks'
import { roomsPage } from './pages/rooms'
import { historyPage } from './pages/history'
import { settingsPage } from './pages/settings'
import { editorPage, previewPanel } from './pages/editor'

const app = document.querySelector<HTMLDivElement>('#app')!

function showToast(message: string, type: 'success' | 'info' = 'success'): void {
  state.toast = message
  state.toastType = type
  render()
}

function routeTo(path: string): void {
  if (location.hash === path) syncRoute()
  else location.hash = path
}

function syncRoute(): void {
  const route = location.hash.slice(1) || '/'
  state.editingId = null
  if (route === '/') state.page = 'dashboard'
  else if (route === '/tasks') state.page = 'tasks'
  else if (route === '/rooms') state.page = 'rooms'
  else if (route === '/history') state.page = 'history'
  else if (route === '/settings') state.page = 'settings'
  else if (route === '/tasks/new') {
    state.page = 'editor'
    state.draft = blankTask()
  } else {
    const match = route.match(/^\/tasks\/([^/]+)\/edit$/)
    const task = match && state.tasks.find(item => item.id === match[1])
    if (!task) {
      routeTo('#/tasks')
      return
    }
    state.page = 'editor'
    state.editingId = task.id
    state.draft = structuredClone(task)
  }
  state.toast = ''
  render()
  document.querySelector('.main-scroll')?.scrollTo(0, 0)
}

function refreshPreview(): void {
  const panel = document.querySelector('.editor-preview')
  if (!panel || !state.draft) return
  panel.outerHTML = previewPanel(state.draft)
  renderIcons()
}

function render(): void {
  app.innerHTML = `<div class="app-shell">${sidebar()}<div class="app-column">${titlebar()}<main id="main" class="main-scroll" tabindex="-1"><div class="content-wrap">${({ dashboard, tasks: tasksPage, rooms: roomsPage, history: historyPage, settings: settingsPage, editor: editorPage } as Record<Page, () => string>)[state.page]()}</div><footer class="app-footer">dan-tick UI 原型 · 不会发送真实弹幕</footer></main></div></div>${state.toast ? `<div role="status" class="toast">${ico(state.toastType === 'info' ? 'info' : 'circle-check')}${h(state.toast)}<button class="icon-button" type="button" data-action="dismiss-toast" aria-label="关闭提示">${ico('x')}</button></div>` : ''}`
  renderIcons()
}

function saveTask(): void {
  const task = state.draft!
  const editing = Boolean(state.editingId)
  task.messages = task.messages.map(line => line.trim()).filter(Boolean)
  if (!task.name.trim() || !task.messages.length) {
    showToast('请填写任务名称及至少一条弹幕内容')
    return
  }
  if (task.schedule === 'weekly' && !task.weekdays.length) {
    showToast('请至少选择一个执行星期')
    return
  }
  if (task.schedule === 'once' && new Date(`${task.date}T${task.time}:00`) <= new Date()) {
    showToast('一次性任务的执行时间需要晚于现在')
    return
  }
  if (task.kind === 'loop' && (!Number.isInteger(task.interval) || task.interval < 1 || !Number.isInteger(task.maxCount) || task.maxCount < 1 || !Number.isInteger(task.maxDuration) || task.maxDuration < 0)) {
    showToast('请检查循环间隔、最大发送次数和运行时长')
    return
  }
  if (state.editingId) {
    const index = state.tasks.findIndex(item => item.id === state.editingId)
    if (index >= 0) state.tasks[index] = structuredClone(task)
  } else {
    task.id = `t${Date.now()}`
    state.tasks.unshift(structuredClone(task))
  }
  routeTo('#/tasks')
  showToast(editing ? '任务已在原型中更新' : '任务已添加到演示列表')
}

export function startApp(): void {
  app.addEventListener('click', event => {
    const actionEl = (event.target as HTMLElement).closest<HTMLElement>('[data-action]')
    if (!actionEl) return
    const { action, id } = actionEl.dataset
    switch (action) {
      case 'toggle-sidebar': state.sidebarExpanded = !state.sidebarExpanded; render(); break
      case 'window-minimize':
      case 'window-maximize':
      case 'window-close':
        showToast('浏览器原型仅展示窗口控件，实际窗口操作需由 Wails 接入。', 'info')
        break
      case 'toggle-theme':
        state.theme = state.theme === 'dark' ? 'light' : 'dark'
        document.documentElement.dataset.theme = state.theme
        localStorage.setItem('dan-tick-prototype-theme', state.theme)
        render()
        break
      case 'go-settings': routeTo('#/settings'); break
      case 'go-tasks': routeTo('#/tasks'); break
      case 'new-task': routeTo('#/tasks/new'); break
      case 'edit-task': routeTo(`#/tasks/${id}/edit`); break
      case 'run-task': {
        const task = state.tasks.find(item => item.id === id)
        if (!task) return
        task.state = 'running'
        showToast(`「${task.name}」已进入模拟运行状态；不会发送弹幕`)
        break
      }
      case 'stop-task': {
        const task = state.tasks.find(item => item.id === id)
        if (!task) return
        task.state = 'idle'
        state.logs.unshift({ id: `l${Date.now()}`, taskId: task.id, taskName: task.name, roomId: task.roomId, date: new Date().toLocaleString('sv-SE').slice(0, 16), status: 'stopped', sent: 0, failed: 0, detail: '用户停止了模拟执行；没有发送真实弹幕。' })
        showToast(`「${task.name}」的模拟执行已停止`)
        break
      }
      case 'delete-task': {
        const task = state.tasks.find(item => item.id === id)
        if (task && confirm(`删除演示任务「${task.name}」？`)) {
          state.tasks = state.tasks.filter(item => item.id !== id)
          showToast('演示任务已删除')
        }
        break
      }
      case 'delete-room': {
        const room = state.rooms.find(item => item.id === id)
        if (!room) return
        const count = state.tasks.filter(task => task.roomId === room.id).length
        if (confirm(`删除直播间「${room.name}」？这会同时删除 ${count} 个关联演示任务。`)) {
          state.rooms = state.rooms.filter(item => item.id !== id)
          state.tasks = state.tasks.filter(task => task.roomId !== id)
          showToast('直播间及关联演示任务已删除')
        }
        break
      }
      case 'toggle-room-form': state.roomFormOpen = !state.roomFormOpen; render(); break
      case 'set-rooms-view':
        if (actionEl.dataset.view === 'cards' || actionEl.dataset.view === 'list') {
          state.roomsView = actionEl.dataset.view
          render()
        }
        break
      case 'clear-search': state.search = ''; render(); break
      case 'log-detail':
        state.selectedLog = state.page === 'dashboard' ? id ?? null : state.selectedLog === id ? null : id ?? null
        if (state.page === 'dashboard') routeTo('#/history')
        else render()
        break
      case 'dismiss-toast': state.toast = ''; render(); break
    }
  })

  app.addEventListener('input', event => {
    const target = event.target as HTMLInputElement | HTMLTextAreaElement
    if (target.id === 'task-search') {
      state.search = target.value
      const selection = target.selectionStart
      render()
      const input = document.querySelector<HTMLInputElement>('#task-search')
      input?.focus()
      input?.setSelectionRange(selection, selection)
    }
    if (target.dataset.field && state.draft) {
      const field = target.dataset.field
      if (field === 'messages') state.draft.messages = target.value.split('\n')
      else if (field === 'interval' || field === 'maxCount' || field === 'maxDuration') state.draft[field] = Number(target.value)
      else if (field === 'name' || field === 'roomId' || field === 'time' || field === 'date') state.draft[field] = target.value
      refreshPreview()
    }
  })

  app.addEventListener('change', event => {
    const target = event.target as HTMLInputElement | HTMLSelectElement
    if (target.dataset.taskToggle) {
      const task = state.tasks.find(item => item.id === target.dataset.taskToggle)
      if (task && target instanceof HTMLInputElement) {
        if (!target.checked && task.state === 'running' && !confirm('该任务正在模拟运行。停用计划并停止本次执行吗？')) {
          target.checked = true
          return
        }
        task.enabled = target.checked
        if (!task.enabled && task.state === 'running') task.state = 'idle'
        showToast(`「${task.name}」已${task.enabled ? '启用' : '停用'}自动执行（演示）`)
      }
    } else if (target.dataset.setting) {
      if (target instanceof HTMLInputElement) {
        const key = target.dataset.setting as 'autoStart' | 'hideOnStart' | 'closeToTray' | 'scheduling'
        state.settings[key] = target.checked
        showToast('演示设置已更新；不会影响操作系统')
      }
    } else if (target.id === 'log-status') { state.logStatus = target.value; render() }
    else if (target.id === 'log-task') { state.logTask = target.value; render() }
    else if (target.dataset.weekday && state.draft && target instanceof HTMLInputElement) {
      const day = Number(target.dataset.weekday)
      state.draft.weekdays = target.checked ? [...state.draft.weekdays, day].sort((a, b) => a - b) : state.draft.weekdays.filter(item => item !== day)
      render()
    } else if (target.dataset.field && state.draft) {
      if (target.dataset.field === 'enabled' && target instanceof HTMLInputElement) state.draft.enabled = target.checked
      if (target.dataset.field === 'kind' && (target.value === 'single' || target.value === 'loop')) state.draft.kind = target.value
      if (target.dataset.field === 'schedule' && (target.value === 'daily' || target.value === 'weekly' || target.value === 'once')) state.draft.schedule = target.value
      if (target.dataset.field === 'kind' || target.dataset.field === 'schedule') render()
    }
  })

  app.addEventListener('submit', event => {
    event.preventDefault()
    const form = event.target as HTMLFormElement
    if (form.id === 'task-form') saveTask()
    if (form.dataset.form === 'room') {
      const data = new FormData(form)
      const id = String(data.get('roomId') ?? '').trim()
      if (state.rooms.some(room => room.id === id)) {
        showToast('这个直播间 ID 已在演示列表中')
        return
      }
      const room: Room = { id, name: String(data.get('roomName') ?? '').trim(), streamer: String(data.get('streamer') ?? '').trim(), title: String(data.get('title') ?? '').trim(), note: '自定义' }
      state.rooms.push(room)
      state.roomFormOpen = false
      showToast('直播间已添加到演示列表')
    }
  })

  window.addEventListener('hashchange', syncRoute)
  syncRoute()
}
