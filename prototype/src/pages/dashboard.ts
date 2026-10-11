import { displayNext, nextRun, type Task } from '../data'
import { state } from '../state'
import { h, ico, logTable, pageHeading, roomName, sectionTitle } from '../ui'

export function dashboard(): string {
  const enabled = state.tasks.filter(task => task.enabled).length
  const running = state.tasks.filter(task => task.state === 'running').length
  const upcoming = state.tasks
    .map(task => ({ task, at: nextRun(task) }))
    .filter((entry): entry is { task: Task; at: Date } => entry.at !== null)
    .sort((a, b) => a.at.getTime() - b.at.getTime())
  const lastFailure = state.logs.find(log => log.status === 'failed' || log.status === 'missed')

  return `${pageHeading(`<button class="button button--primary" data-action="new-task" type="button">${ico('plus')} 新建任务</button>`)}
    <section class="system-summary" aria-label="演示状态">
      <div class="summary-item"><span>调度服务</span><strong class="summary-warning">未接入后端</strong><small>不会自动发送弹幕</small></div>
      <div class="summary-item"><span>B 站账号</span><a href="#/settings" class="summary-link">未配置 ${ico('arrow-up-right')}</a><small>凭证管理尚未接入</small></div>
      <div class="summary-item"><span>已启用任务</span><strong>${enabled} <small>/ ${state.tasks.length}</small></strong><small>仅表示演示计划已启用</small></div>
      <div class="summary-item"><span>模拟运行中</span><strong>${running}</strong><small>${running ? '可前往任务列表停止' : '当前没有模拟执行'}</small></div>
    </section>
    <div class="dashboard-grid">
      <section class="panel schedule-panel">${sectionTitle('即将执行', '', `<a class="subtle-link" href="#/tasks">全部任务 ${ico('chevron-right')}</a>`)}
        ${upcoming.length ? `<div class="upcoming-list scroll-region" tabindex="0" aria-label="即将执行的任务">${upcoming.slice(0, 5).map(({ task }) => `<div class="upcoming-item"><div class="upcoming-body"><strong>${h(task.name)}</strong><small>${h(roomName(task.roomId))} · ${task.kind === 'loop' ? '循环发送' : '单次发送'}</small></div><span class="upcoming-time">${displayNext(task)}</span></div>`).join('')}</div>` : `<div class="empty-state">${ico('calendar-clock')}<p>暂无即将执行的任务</p><button class="button button--secondary" data-action="new-task" type="button">新建任务</button></div>`}
      </section>
      <section class="panel alert-panel">${sectionTitle('最近异常', '', `<a class="subtle-link" href="#/history">执行记录 ${ico('chevron-right')}</a>`)}
        ${lastFailure ? `<div class="alert-details scroll-region"><span class="alert-status">${ico('circle-alert')} ${lastFailure.status === 'failed' ? '发送失败' : '任务错过'}</span><h3>${h(lastFailure.taskName)}</h3><time>${h(lastFailure.date)}</time><p>${h(lastFailure.detail)}</p><button type="button" class="text-link" data-action="log-detail" data-id="${h(lastFailure.id)}">查看记录 ${ico('arrow-up-right')}</button></div>` : `<p class="no-alerts">演示记录中暂无异常。</p>`}
        <div class="alert-footer"><span>已添加直播间</span><a href="#/rooms">${state.rooms.length} 个 ${ico('chevron-right')}</a></div>
      </section>
    </div>
    <section class="panel recent-panel">${sectionTitle('最近执行', '', `<a class="subtle-link" href="#/history">全部记录 ${ico('chevron-right')}</a>`)}${logTable(state.logs.slice(0, 5), true)}</section>`
}
