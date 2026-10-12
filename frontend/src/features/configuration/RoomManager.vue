<script setup lang="ts">
import { computed, shallowRef } from 'vue'
import { Browser } from '@wailsio/runtime'
import { ExternalLink, LayoutGrid, List, Pencil, Plus, Radio, Trash2 } from 'lucide-vue-next'
import type { Room } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { bindingError, useAddRoom, useDeleteRoom, useRoomPages, useUpdateRoom } from './queries'
import { fieldClass, panel, primaryButton, secondaryButton } from './style'
import RoomCover from './RoomCover.vue'
import RoomForm from './RoomForm.vue'

const pages = useRoomPages()
const add = useAddRoom()
const update = useUpdateRoom()
const remove = useDeleteRoom()
const view = shallowRef<'cards' | 'list'>('cards')
const showForm = shallowRef(false)
const editingID = shallowRef<number | null>(null)
const remark = shallowRef('')
const message = shallowRef('')
const rooms = computed(() => pages.data.value?.pages.flatMap((p) => p.items ?? []) ?? [])
const roomActionClass =
  'inline-flex size-8 items-center justify-center rounded-md hover:bg-surface-muted focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring'

function secondaryTitle(room: Room): string {
  if (!room.title) return '未获取到直播标题'
  return room.title.trim() === (room.remark || room.title).trim() ? '' : room.title
}

function roomURL(room: Room): string {
  return `https://live.bilibili.com/${encodeURIComponent(room.platformRoomId)}`
}

async function openRoom(room: Room) {
  message.value = ''
  try {
    await Browser.OpenURL(roomURL(room))
  } catch {
    message.value = '无法打开直播间，请检查系统默认浏览器后重试'
  }
}

async function addRoom(value: string) {
  message.value = ''
  try {
    await add.mutateAsync(value)
    showForm.value = false
  } catch (error) {
    message.value = bindingError(error)
  }
}
function beginEdit(room: Room) {
  editingID.value = room.id
  remark.value = room.remark
  message.value = ''
}
async function saveRemark(id: number) {
  try {
    await update.mutateAsync({ id, remark: remark.value })
    editingID.value = null
    message.value = ''
  } catch (error) {
    message.value = bindingError(error)
  }
}
async function deleteRoom(room: Room) {
  if (
    !window.confirm(
      `删除直播间 ${room.remark || room.title || room.platformRoomId}？关联任务将同时删除。`,
    )
  )
    return
  try {
    await remove.mutateAsync(room.id)
    message.value = ''
  } catch (error) {
    message.value = bindingError(error)
  }
}
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col gap-4">
    <header class="flex flex-wrap items-center justify-between gap-3">
      <div>
        <h1>直播间</h1>
      </div>
      <div class="flex items-center gap-2">
        <div
          class="flex rounded-[10px] border border-border-hairline bg-surface-muted p-1"
          role="group"
          aria-label="直播间显示方式"
        >
          <button
            type="button"
            class="rounded-md px-2.5 py-1.5 text-sm"
            :class="
              view === 'cards' ? 'bg-surface-solid-hover text-text-primary' : 'text-text-secondary'
            "
            :aria-pressed="view === 'cards'"
            @click="view = 'cards'"
          >
            <LayoutGrid class="mr-1 inline size-4" aria-hidden="true" />卡片
          </button>
          <button
            type="button"
            class="rounded-md px-2.5 py-1.5 text-sm"
            :class="
              view === 'list' ? 'bg-surface-solid-hover text-text-primary' : 'text-text-secondary'
            "
            :aria-pressed="view === 'list'"
            @click="view = 'list'"
          >
            <List class="mr-1 inline size-4" aria-hidden="true" />列表
          </button>
        </div>
        <button type="button" :class="primaryButton" @click="showForm = !showForm">
          <Plus class="size-4" aria-hidden="true" />添加直播间
        </button>
      </div>
    </header>
    <RoomForm
      v-if="showForm"
      :pending="add.isPending.value"
      @submit="addRoom"
      @cancel="showForm = false"
    />
    <p v-if="message" role="alert" class="rounded-lg bg-soft-danger-bg p-3 text-soft-danger-fg">
      {{ message }}
    </p>
    <div v-if="pages.isPending.value" role="status" class="p-6 text-text-secondary">
      正在加载直播间…
    </div>
    <div v-else-if="pages.isError.value" role="alert" :class="[panel, 'p-6']">
      <p class="text-soft-danger-fg">{{ bindingError(pages.error.value) }}</p>
      <button type="button" :class="secondaryButton" class="mt-3" @click="pages.refetch()">
        重试读取
      </button>
    </div>
    <div v-else-if="!rooms.length" :class="[panel, 'p-8 text-center']">
      <Radio class="mx-auto mb-3 size-9 text-text-tertiary" aria-hidden="true" />
      <h2 class="mb-1 text-base font-medium">还没有添加直播间</h2>
      <p class="mb-4 text-sm text-text-secondary">添加一个直播间即可创建弹幕任务。</p>
      <button type="button" :class="secondaryButton" @click="showForm = true">添加直播间</button>
    </div>
    <div v-else class="min-h-0 flex-1 overflow-y-auto pb-3" aria-label="直播间内容" tabindex="0">
      <div
        :class="
          view === 'cards'
            ? 'grid grid-cols-1 gap-4 min-[601px]:grid-cols-2 min-[1101px]:grid-cols-3'
            : 'flex flex-col gap-3'
        "
      >
        <article
          v-for="(room, index) in rooms"
          :key="room.id"
          :class="[panel, 'min-w-0 overflow-hidden']"
        >
          <RoomCover
            v-if="view === 'cards'"
            :room-id="room.id"
            :platform-room-id="room.platformRoomId"
            :index="index"
          />
          <div
            class="p-5"
            :class="view === 'list' ? 'flex flex-col gap-4 pr-7 md:flex-row md:items-center' : ''"
          >
            <div :class="view === 'list' ? 'min-w-0 flex-1' : ''">
              <div :class="view === 'list' ? 'flex flex-wrap items-center gap-x-7 gap-y-2' : ''">
                <div class="flex min-w-0 flex-wrap items-center gap-2">
                  <h2 class="min-w-0 truncate text-base font-semibold">
                    {{ room.remark || room.title || `房间 ${room.platformRoomId}` }}
                  </h2>
                  <span
                    v-if="room.remark"
                    class="rounded-md bg-soft-accent-bg px-2 py-0.5 text-xs text-soft-accent-fg"
                    >备注</span
                  >
                </div>
                <p
                  v-if="secondaryTitle(room)"
                  class="truncate text-sm text-text-secondary"
                  :class="view === 'list' ? 'min-w-0 max-w-full' : 'mt-1'"
                >
                  {{ secondaryTitle(room) }}
                </p>
                <p
                  class="text-xs text-text-tertiary"
                  :class="view === 'list' ? 'min-w-0 max-w-full' : 'mt-2'"
                >
                  主播：{{ room.anchorName || '未知' }} · 房间号 {{ room.platformRoomId }}
                </p>
              </div>
              <form
                v-if="editingID === room.id"
                class="mt-3 flex flex-wrap gap-2"
                @submit.prevent="saveRemark(room.id)"
              >
                <span class="sr-only">备注名称</span>
                <input
                  :id="`remark-${room.id}`"
                  v-model="remark"
                  aria-label="备注名称"
                  :class="fieldClass"
                  class="min-w-30 flex-1"
                  maxlength="100"
                  placeholder="备注名称"
                />
                <button type="submit" :disabled="update.isPending.value" :class="primaryButton">
                  保存备注
                </button>
                <button type="button" :class="secondaryButton" @click="editingID = null">
                  取消
                </button>
              </form>
            </div>
            <div
              class="flex items-center justify-between"
              :class="
                view === 'list'
                  ? 'shrink-0 gap-5 md:ml-auto'
                  : 'mt-4 border-t border-border-hairline pt-3'
              "
            >
              <span class="text-xs text-text-secondary"
                >关联 <strong class="text-text-primary">{{ room.taskCount }}</strong> 个任务</span
              >
              <div class="flex items-center gap-2">
                <a
                  :class="roomActionClass"
                  :href="roomURL(room)"
                  target="_blank"
                  rel="noopener noreferrer"
                  :aria-label="`打开直播间 ${room.platformRoomId}`"
                  title="打开直播间"
                  @click.prevent="openRoom(room)"
                  ><ExternalLink class="size-4" aria-hidden="true"
                /></a>
                <button
                  type="button"
                  :class="roomActionClass"
                  :aria-label="`修改 ${room.platformRoomId} 的备注`"
                  title="修改备注"
                  @click="beginEdit(room)"
                >
                  <Pencil class="size-4" aria-hidden="true" />
                </button>
                <button
                  type="button"
                  class="text-soft-danger-fg"
                  :class="roomActionClass"
                  :aria-label="`删除直播间 ${room.platformRoomId}`"
                  title="删除直播间"
                  :disabled="remove.isPending.value"
                  @click="deleteRoom(room)"
                >
                  <Trash2 class="size-4" aria-hidden="true" />
                </button>
              </div>
            </div>
          </div>
        </article>
      </div>
      <div class="mt-5 flex justify-center">
        <button
          v-if="pages.hasNextPage.value"
          type="button"
          :class="secondaryButton"
          :disabled="pages.isFetchingNextPage.value"
          @click="pages.fetchNextPage()"
        >
          {{ pages.isFetchingNextPage.value ? '加载中…' : '加载更多' }}
        </button>
        <p v-else class="text-xs text-text-tertiary">已显示全部直播间</p>
      </div>
      <p
        v-if="pages.isFetchNextPageError.value"
        role="alert"
        class="mt-2 text-center text-soft-danger-fg"
      >
        加载下一页失败。<button type="button" class="underline" @click="pages.fetchNextPage()">
          重试
        </button>
      </p>
    </div>
  </section>
</template>
