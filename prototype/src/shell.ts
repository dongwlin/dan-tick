import { type Page } from './data'
import { state } from './state'
import { ico } from './ui'

const navItems: { page: Page; label: string; icon: string; path: string }[] = [
  { page: 'dashboard', label: '总览', icon: 'layout-dashboard', path: '#/' },
  { page: 'tasks', label: '弹幕任务', icon: 'list-todo', path: '#/tasks' },
  { page: 'rooms', label: '直播间', icon: 'radio', path: '#/rooms' },
  { page: 'history', label: '执行记录', icon: 'history', path: '#/history' },
  { page: 'settings', label: '设置', icon: 'settings-2', path: '#/settings' },
]

export function sidebar(): string {
  return `<aside class="sidebar" aria-label="主导航">
    <nav class="sidebar-nav" aria-label="页面">
      ${navItems.map(item => `<a class="nav-link ${state.page === item.page || (state.page === 'editor' && item.page === 'tasks') ? 'nav-link--active' : ''}" href="${item.path}" data-tooltip="${item.label}" aria-label="${item.label}" ${state.page === item.page ? 'aria-current="page"' : ''}>${ico(item.icon)}</a>`).join('')}
    </nav>
    <div class="sidebar-bottom">
      <button class="profile-link" type="button" data-action="go-settings" aria-label="打开 B 站账号设置">
        <span class="avatar" aria-hidden="true">UP</span>
      </button>
    </div>
  </aside>`
}

export function titlebar(): string {
  return `<header class="window-titlebar" aria-label="窗口标题栏">
    <span class="window-brand">dan<span>tick</span></span>
    <div class="window-controls" role="group" aria-label="窗口操作">
      <button class="window-control" type="button" data-action="window-minimize" aria-label="最小化窗口" title="最小化">${ico('minus')}</button>
      <button class="window-control" type="button" data-action="window-maximize" aria-label="最大化窗口" title="最大化">${ico('square')}</button>
      <button class="window-control window-control--close" type="button" data-action="window-close" aria-label="关闭窗口" title="关闭">${ico('x')}</button>
    </div>
  </header>`
}
