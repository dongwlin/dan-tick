import { type LogStatus } from '../data'
import { state } from '../state'
import { h, pageHeading, runLabel, logTable } from '../ui'

export function historyPage(): string {
  const logs = state.logs.filter(log => (state.logStatus === 'all' || log.status === state.logStatus) && (state.logTask === 'all' || log.taskId === state.logTask))
  return `${pageHeading()}
    <section class="panel history-panel"><div class="list-toolbar"><div class="list-toolbar-heading"><h2>执行历史 <span>${logs.length}</span></h2></div><div class="filter-group"><label class="sr-only" for="log-task">按任务筛选</label><select id="log-task"><option value="all">全部任务</option>${state.tasks.map(task => `<option value="${h(task.id)}" ${state.logTask === task.id ? 'selected' : ''}>${h(task.name)}</option>`).join('')}</select><label class="sr-only" for="log-status">按状态筛选</label><select id="log-status"><option value="all">全部状态</option>${(['success', 'failed', 'stopped', 'missed'] as LogStatus[]).map(status => `<option value="${status}" ${state.logStatus === status ? 'selected' : ''}>${runLabel(status)}</option>`).join('')}</select></div></div>${logTable(logs)}</section>`
}
