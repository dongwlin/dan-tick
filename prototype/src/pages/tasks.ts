import { state } from '../state'
import { h, ico, pageHeading, roomName, taskRow } from '../ui'

export function tasksPage(): string {
  const search = state.search.trim().toLowerCase()
  const tasks = state.tasks.filter(task => `${task.name} ${roomName(task.roomId)}`.toLowerCase().includes(search))
  return `${pageHeading(`<button class="button button--primary" data-action="new-task" type="button">${ico('plus')} 新建任务</button>`)}
    <section class="panel task-list-panel"><div class="list-toolbar"><div class="list-toolbar-heading"><h2>我的任务 <span>${state.tasks.length}</span></h2></div><label class="searchbox">${ico('search')}<span class="sr-only">搜索任务</span><input id="task-search" type="search" placeholder="搜索任务或直播间..." value="${h(state.search)}" /></label></div>
      <div class="task-list-body scroll-region" data-list-end="tasks" tabindex="0" aria-label="弹幕任务列表">${tasks.length ? `${tasks.map(task => `<div class="task-list-item">${taskRow(task)}<div class="task-list-footer"><span>${ico('clock-3')} ${h(task.kind === 'loop' ? `循环 · 每隔 ${task.interval} 秒 · 最多 ${task.maxCount} 条` : '单次发送 · 1 条消息')}</span><div><label class="switch-label"><input type="checkbox" data-task-toggle="${h(task.id)}" ${task.enabled ? 'checked' : ''}/><span class="switch-ui"></span><span>${task.enabled ? '自动执行开启' : '自动执行关闭'}</span></label><button class="text-danger" type="button" data-action="delete-task" data-id="${h(task.id)}">删除</button></div></div></div>`).join('')}<div class="list-endcap">共 ${tasks.length} 个任务，已全部显示</div>` : `<div class="empty-state">${ico('search')}<p>没有找到匹配的任务</p><button class="button button--secondary" type="button" data-action="clear-search">清除搜索</button></div>`}</div>
    </section>`
}
