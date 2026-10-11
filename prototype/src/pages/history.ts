import { type LogStatus } from '../data'
import { state } from '../state'
import { h, ico, pageHeading, runLabel, logTable } from '../ui'

function pagination(total: number, page: number, pageSize: number): string {
  const totalPages = Math.ceil(total / pageSize)
  if (totalPages <= 1) return ''
  const first = (page - 1) * pageSize + 1
  const last = Math.min(total, page * pageSize)
  const start = Math.min(Math.max(1, page - 2), Math.max(1, totalPages - 4))
  const end = Math.min(totalPages, start + 4)
  const pages = Array.from({ length: end - start + 1 }, (_, index) => start + index)
  return `<nav class="history-pagination" aria-label="执行记录分页">
    <span class="history-page-summary">显示 ${first}–${last} 条，共 ${total} 条</span>
    <div class="history-page-controls">
      <button class="history-page-arrow" type="button" data-action="history-page" data-page="${page - 1}" aria-label="上一页" ${page === 1 ? 'disabled' : ''}>${ico('chevron-left')}</button>
      ${pages.map(number => `<button class="history-page-number" type="button" data-action="history-page" data-page="${number}" aria-label="第 ${number} 页" ${number === page ? 'aria-current="page"' : ''}>${number}</button>`).join('')}
      <button class="history-page-arrow" type="button" data-action="history-page" data-page="${page + 1}" aria-label="下一页" ${page === totalPages ? 'disabled' : ''}>${ico('chevron-right')}</button>
    </div>
  </nav>`
}

export function historyPage(): string {
  const logs = state.logs.filter(log => (state.logStatus === 'all' || log.status === state.logStatus) && (state.logTask === 'all' || log.taskId === state.logTask))
  const pageSize = state.historyPageSize
  const totalPages = Math.max(1, Math.ceil(logs.length / pageSize))
  const page = Math.min(state.historyPage, totalPages)
  const isPaged = totalPages > 1
  const shown = logs.slice((page - 1) * pageSize, page * pageSize)
  const isLastPage = page === totalPages
  const showEndcap = shown.length > 0 && (!isPaged || (isLastPage && shown.length < pageSize))
  return `${pageHeading()}
    <section class="panel history-panel"><div class="list-toolbar"><div class="list-toolbar-heading"><h2>执行历史 <span>${logs.length}</span></h2></div><div class="filter-group"><label class="sr-only" for="log-task">按任务筛选</label><select id="log-task"><option value="all">全部任务</option>${state.tasks.map(task => `<option value="${h(task.id)}" ${state.logTask === task.id ? 'selected' : ''}>${h(task.name)}</option>`).join('')}</select><label class="sr-only" for="log-status">按状态筛选</label><select id="log-status"><option value="all">全部状态</option>${(['success', 'failed', 'stopped', 'missed'] as LogStatus[]).map(status => `<option value="${status}" ${state.logStatus === status ? 'selected' : ''}>${runLabel(status)}</option>`).join('')}</select></div></div><div class="history-table scroll-region" ${showEndcap ? 'data-list-end="history"' : ''} tabindex="0" aria-label="执行记录表格">${logTable(shown)}${showEndcap ? `<div class="list-endcap">${isPaged ? `已到最后一页，共 ${logs.length} 条记录` : `共 ${logs.length} 条记录，已全部显示`}</div>` : ''}</div>${pagination(logs.length, page, pageSize)}</section>`
}
