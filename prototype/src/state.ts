import { initialLogs, initialRooms, initialTasks, type Page, type Task } from './data'

export const state = {
  page: 'dashboard' as Page,
  rooms: structuredClone(initialRooms),
  tasks: structuredClone(initialTasks),
  logs: structuredClone(initialLogs),
  draft: null as Task | null,
  editingId: null as string | null,
  search: '',
  logStatus: 'all',
  logTask: 'all',
  selectedLog: null as string | null,
  roomFormOpen: false,
  roomsView: 'cards' as 'cards' | 'list',
  toast: '',
  toastType: 'success' as 'success' | 'info',
  settings: { autoStart: false, hideOnStart: false, closeToTray: true, scheduling: true, timezone: 'Asia/Shanghai' },
  theme: document.documentElement.dataset.theme === 'light' ? 'light' : 'dark',
}

export function blankTask(): Task {
  return {
    id: '', name: '', roomId: state.rooms[0]?.id ?? '', kind: 'single',
    messages: [''], schedule: 'daily', time: '22:00', date: new Date().toISOString().slice(0, 10),
    weekdays: [1, 3, 5], interval: 15, maxCount: 20, maxDuration: 300,
    enabled: true, state: 'idle',
  }
}
