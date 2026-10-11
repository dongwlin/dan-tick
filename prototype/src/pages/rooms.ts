import { type Room } from '../data'
import { state } from '../state'
import { h, ico, pageHeading } from '../ui'

function roomActions(room: Room): string {
  return `<div class="room-actions">
    <a class="icon-button" href="https://live.bilibili.com/${encodeURIComponent(room.id)}" target="_blank" rel="noopener noreferrer" title="打开 B 站直播间" aria-label="打开直播间 ${h(room.name)}">${ico('external-link')}</a>
    <button class="icon-button icon-button--danger" type="button" data-action="delete-room" data-id="${h(room.id)}" title="删除直播间" aria-label="删除直播间 ${h(room.name)}">${ico('trash-2')}</button>
  </div>`
}

function roomCard(room: Room, index: number): string {
  const taskCount = state.tasks.filter(task => task.roomId === room.id).length
  return `<article class="room-card">
    <div class="room-card-cover room-card-cover--${index % 3}">
      <span class="room-card-symbol">${ico('radio')}</span>
      <span class="room-card-id">直播间 · ${h(room.id)}</span>
    </div>
    <div class="room-card-content">
      <div class="room-card-heading"><h3>${h(room.name)}</h3>${room.note ? `<span class="room-tag">${h(room.note)}</span>` : ''}</div>
      <p class="room-card-title">${h(room.title || '未填写直播标题')}</p>
      <div class="room-card-streamer">${ico('user-round')} ${h(room.streamer || '未知主播')}</div>
      <div class="room-card-footer"><span>关联 <strong>${taskCount}</strong> 个任务</span>${roomActions(room)}</div>
    </div>
  </article>`
}

function roomRow(room: Room): string {
  const taskCount = state.tasks.filter(task => task.roomId === room.id).length
  return `<article class="room-row">
    <div class="room-primary"><h3>${h(room.name)}</h3><p>${h(room.streamer || '未知主播')} <span class="sub-separator">·</span> 房间号 ${h(room.id)}</p></div>
    <div class="room-description"><strong>${h(room.title || '未填写直播标题')}</strong><small>${h(room.note || '无备注')}</small></div>
    <div class="room-tasks">关联 ${taskCount} 个任务</div>
    ${roomActions(room)}
  </article>`
}

function roomForm(): string {
  if (!state.roomFormOpen) return ''
  return `<form class="panel room-form" data-form="room">
    <div class="section-heading"><h2>添加直播间</h2><button class="icon-button" type="button" data-action="toggle-room-form" aria-label="关闭添加表单">${ico('x')}</button></div>
    <div class="form-grid">
      <label class="field">直播间 ID <input name="roomId" required pattern="[0-9]+" placeholder="例如 22625025" /></label>
      <label class="field">备注名称 <input name="roomName" required maxlength="36" placeholder="给这个直播间起个名字" /></label>
      <label class="field">主播昵称 <input name="streamer" placeholder="选填" /></label>
      <label class="field">直播间标题 <input name="title" placeholder="选填" /></label>
    </div>
    <div class="form-actions"><button class="button button--secondary" type="button" data-action="toggle-room-form">取消</button><button class="button button--primary" type="submit">${ico('plus')} 添加到列表</button></div>
  </form>`
}

export function roomsPage(): string {
  const viewSwitch = `<div class="rooms-view-switch" role="group" aria-label="直播间显示方式">
    <button type="button" data-action="set-rooms-view" data-view="cards" aria-pressed="${state.roomsView === 'cards'}" title="卡片视图">${ico('layout-grid')}<span>卡片</span></button>
    <button type="button" data-action="set-rooms-view" data-view="list" aria-pressed="${state.roomsView === 'list'}" title="列表视图">${ico('list')}<span>列表</span></button>
  </div>`
  return `${pageHeading(`${viewSwitch}<button class="button button--primary" data-action="toggle-room-form" type="button">${ico('plus')} 添加直播间</button>`)}
    <div class="rooms-content scroll-region ${state.roomsView === 'list' ? 'rooms-content--list' : ''} ${state.roomFormOpen ? 'rooms-content--with-form' : ''}" tabindex="0" aria-label="直播间内容">
    ${roomForm()}
    ${state.roomsView === 'cards'
      ? `<div class="room-grid">${state.rooms.length ? state.rooms.map(roomCard).join('') : `<div class="panel empty-state"><p>还没有添加直播间</p><button class="button button--secondary" type="button" data-action="toggle-room-form">添加直播间</button></div>`}</div>`
      : `<section class="panel room-list"><div class="list-toolbar"><div class="list-toolbar-heading"><h2>已添加的直播间 <span>${state.rooms.length}</span></h2></div></div><div class="room-list-body scroll-region" ${state.roomFormOpen ? '' : 'data-list-end="rooms"'} tabindex="0" aria-label="直播间列表">${state.rooms.length ? `${state.rooms.map(roomRow).join('')}<div class="list-endcap">共 ${state.rooms.length} 个直播间，已全部显示</div>` : `<div class="empty-state"><p>还没有添加直播间</p><button class="button button--secondary" type="button" data-action="toggle-room-form">添加直播间</button></div>`}</div></section>`}</div>`
}
