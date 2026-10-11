import { dayNames, scheduleLabel, type Task } from '../data'
import { state } from '../state'
import { h, ico, pageHeading, sectionTitle } from '../ui'

export function editorPage(): string {
  const task = state.draft!
  return `<div class="editor-back"><a href="#/tasks">${ico('arrow-left')} 返回任务列表</a></div>${pageHeading()}
    <form id="task-form" class="editor-layout"><div class="editor-main scroll-region" tabindex="0" aria-label="任务配置表单">
      <section class="panel editor-panel">${sectionTitle('基本信息', '先决定把什么内容发到哪里')}<div class="form-grid"><label class="field field--wide">任务名称 <input name="name" data-field="name" required maxlength="48" placeholder="例如：每日晚安问候" value="${h(task.name)}" /></label><label class="field">目标直播间 <select name="roomId" data-field="roomId" required>${state.rooms.map(room => `<option value="${h(room.id)}" ${task.roomId === room.id ? 'selected' : ''}>${h(room.name)} · ${h(room.id)}</option>`).join('')}</select></label><label class="field">任务类型 <select name="kind" data-field="kind"><option value="single" ${task.kind === 'single' ? 'selected' : ''}>单次发送</option><option value="loop" ${task.kind === 'loop' ? 'selected' : ''}>循环发送</option></select></label></div><label class="checkbox-line"><input type="checkbox" data-field="enabled" ${task.enabled ? 'checked' : ''} /><span>启用这个任务的自动执行</span></label></section>
      <section class="panel editor-panel">${sectionTitle('弹幕内容', task.kind === 'loop' ? '每行一条，按照顺序依次发送，到末尾后循环' : '只会发送第一条弹幕')}<label class="field field--wide">发送队列 <textarea name="messages" data-field="messages" required rows="5" placeholder="在这里输入弹幕内容...">${h(task.messages.join('\n'))}</textarea></label><div class="field-help">${ico('info')} 每行一条，空行自动忽略；原型不校验 B 站接口的实际字符限制。</div>
        ${task.kind === 'loop' ? `<div class="form-grid loop-config"><label class="field">发送间隔（秒） <input type="number" name="interval" data-field="interval" min="1" step="1" value="${task.interval}" required /></label><label class="field">最大发送次数 <input type="number" name="maxCount" data-field="maxCount" min="1" step="1" value="${task.maxCount}" required /></label><label class="field">最长运行时长（秒） <input type="number" name="maxDuration" data-field="maxDuration" min="0" step="1" value="${task.maxDuration}" /></label></div>` : ''}</section>
      <section class="panel editor-panel">${sectionTitle('执行计划', '指定触发方式和时间')}<div class="form-grid"><label class="field">调度方式 <select name="schedule" data-field="schedule"><option value="daily" ${task.schedule === 'daily' ? 'selected' : ''}>每天</option><option value="weekly" ${task.schedule === 'weekly' ? 'selected' : ''}>每周</option><option value="once" ${task.schedule === 'once' ? 'selected' : ''}>一次性</option></select></label><label class="field">执行时间 <input type="time" name="time" data-field="time" value="${h(task.time)}" required /></label>
        ${task.schedule === 'once' ? `<label class="field">执行日期 <input type="date" name="date" data-field="date" value="${h(task.date)}" required /></label>` : ''}</div>
        ${task.schedule === 'weekly' ? `<fieldset class="weekday-picker"><legend>选择星期</legend>${[1, 2, 3, 4, 5, 6, 0].map(day => `<label><input type="checkbox" data-weekday="${day}" ${task.weekdays.includes(day) ? 'checked' : ''}/><span>${dayNames[day]}</span></label>`).join('')}</fieldset>` : ''}
        <div class="field-help">${ico('clock-3')} 默认时区：Asia/Shanghai · 原型按当前浏览器时区预览时间</div>
      </section></div>
      <aside class="editor-aside">${previewPanel(task)}<div class="editor-save"><button class="button button--primary" type="submit">${ico('check')} ${state.editingId ? '保存修改' : '创建任务'}</button><a class="button button--secondary" href="#/tasks">取消</a></div></aside>
    </form>`
}

export function previewPanel(task: Task): string {
  return `<section class="panel editor-preview"><div class="preview-heading">执行预览</div><div class="preview-line"><small>执行时间</small><strong>${h(scheduleLabel(task))}</strong></div><div class="preview-line"><small>发送方式</small><strong>${task.kind === 'loop' ? '循环发送' : '单次发送'}</strong></div><div class="preview-line"><small>消息数量</small><strong>${task.messages.filter(message => message.trim()).length} 条</strong></div>${task.kind === 'loop' ? `<div class="preview-line"><small>发送间隔</small><strong>${task.interval} 秒</strong></div><div class="preview-line"><small>停止条件</small><strong>${task.maxCount} 次 / ${task.maxDuration || '不限'} 秒</strong></div>` : ''}<div class="preview-note">仅预览计划，不会实际调度或发送。</div></section>`
}
