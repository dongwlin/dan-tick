<script setup lang="ts">
import { computed, nextTick, shallowRef, useTemplateRef } from 'vue'
import { Browser } from '@wailsio/runtime'
import { ExternalLink, LayoutGrid, List, Pencil, Plus, Radio, Trash2 } from 'lucide-vue-next'
import { ToggleGroupItem, ToggleGroupRoot } from 'reka-ui'
import type { Room } from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { useAddRoom, useDeleteRoom, useRoomPages, useUpdateRoom } from './queries'
import { bindingError } from '@/lib/bindingError'
import UiPanel from '@/components/ui/UiPanel.vue'
import UiAlert from '@/components/ui/UiAlert.vue'
import UiAsyncSection from '@/components/ui/UiAsyncSection.vue'
import UiEmptyState from '@/components/ui/UiEmptyState.vue'
import UiPageHeader from '@/components/ui/UiPageHeader.vue'
import UiIconButton from '@/components/ui/UiIconButton.vue'
import { primaryButton, secondaryButton } from '@/components/ui/buttonStyles'
import UiConfirmDialog from '@/components/ui/UiConfirmDialog.vue'
import RoomCover from './RoomCover.vue'
import RoomForm from './RoomForm.vue'
import RoomRemarkDialog from './RoomRemarkDialog.vue'
import { useRoomMetadataRefresh } from './useRoomMetadataRefresh'

type RoomView = 'cards' | 'list'
const roomViewStorageKey = 'dan-tick-room-view'

function readRoomView(): RoomView {
  try {
    return localStorage.getItem(roomViewStorageKey) === 'list' ? 'list' : 'cards'
  } catch {
    // Restricted WebViews may disable storage; keep the default view usable.
    return 'cards'
  }
}

const pages = useRoomPages()
const add = useAddRoom()
const update = useUpdateRoom()
const remove = useDeleteRoom()
const view = shallowRef<RoomView>(readRoomView())
const showForm = shallowRef(false)
const pendingDelete = shallowRef<Room | null>(null)
const pendingRemark = shallowRef<Room | null>(null)
const remarkDraft = shallowRef('')
const remarkError = shallowRef('')
const message = shallowRef('')
const formError = shallowRef('')
const roomContent = useTemplateRef<HTMLElement>('roomContent')
const rooms = computed(() => pages.data.value?.pages.flatMap((p) => p.items ?? []) ?? [])
const deleteDescription = computed(() => {
  const room = pendingDelete.value
  if (!room) return ''
  return `删除直播间“${roomDisplayName(room)}”后，关联任务会同时删除且无法恢复。`
})
useRoomMetadataRefresh(rooms)
function setRoomView(next: RoomView) {
  view.value = next
  try {
    localStorage.setItem(roomViewStorageKey, next)
  } catch {
    // Preserve the current selection even when persistence is unavailable.
  }
}

function updateRoomView(next: unknown) {
  if (next === 'cards' || next === 'list') setRoomView(next)
}

/** Primary card label: the local remark wins over the remote live-room title. */
function roomDisplayName(room: Room): string {
  return room.remark || room.title || `房间 ${room.platformRoomId}`
}

function secondaryTitle(room: Room): string {
  if (!room.title) return '未获取到直播标题'
  return room.title.trim() === roomDisplayName(room).trim() ? '' : room.title
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

function openForm() {
  message.value = ''
  formError.value = ''
  showForm.value = true
}

async function addRoom(input: { value: string; remark: string }) {
  formError.value = ''
  try {
    await add.mutateAsync(input)
    showForm.value = false
    // Reka restores focus to the element captured when the Dialog opened, but
    // adding the first room replaces that empty-state action. Name the
    // successor explicitly so focus never falls back to <body> (规范 5.3).
    await nextTick()
    roomContent.value?.focus()
  } catch (error) {
    formError.value = bindingError(error)
  }
}
function openRemarkDialog(room: Room) {
  message.value = ''
  remarkError.value = ''
  remarkDraft.value = room.remark
  pendingRemark.value = room
}

function closeRemarkDialog() {
  pendingRemark.value = null
  remarkDraft.value = ''
  remarkError.value = ''
}

async function saveRemark(remark: string) {
  const room = pendingRemark.value
  if (!room) return
  remarkError.value = ''
  try {
    await update.mutateAsync({ id: room.id, remark })
    closeRemarkDialog()
    message.value = ''
  } catch (error) {
    // The Dialog stays open with the draft, so the user can retry (规范 5.3).
    remarkError.value = bindingError(error)
  }
}
function requestDelete(room: Room) {
  pendingDelete.value = room
}

function closeDeleteDialog() {
  pendingDelete.value = null
}

async function deleteRoom() {
  const room = pendingDelete.value
  if (!room) return
  pendingDelete.value = null
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
    <UiPageHeader title="直播间">
      <template #actions>
        <ToggleGroupRoot
          :model-value="view"
          type="single"
          orientation="horizontal"
          aria-label="直播间显示方式"
          class="flex rounded-[10px] border border-border-hairline bg-surface-muted p-1"
          @update:model-value="updateRoomView"
        >
          <ToggleGroupItem
            value="cards"
            class="rounded-md px-2.5 py-1.5 text-sm text-text-secondary hover:text-text-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring data-[state=on]:bg-surface-solid-hover data-[state=on]:text-text-primary"
          >
            <LayoutGrid class="mr-1 inline size-4" aria-hidden="true" />卡片
          </ToggleGroupItem>
          <ToggleGroupItem
            value="list"
            class="rounded-md px-2.5 py-1.5 text-sm text-text-secondary hover:text-text-primary focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-focus-ring data-[state=on]:bg-surface-solid-hover data-[state=on]:text-text-primary"
          >
            <List class="mr-1 inline size-4" aria-hidden="true" />列表
          </ToggleGroupItem>
        </ToggleGroupRoot>
        <button type="button" :class="primaryButton" @click="openForm">
          <Plus class="size-4" aria-hidden="true" />添加直播间
        </button>
      </template>
    </UiPageHeader>
    <RoomForm
      v-model:open="showForm"
      v-model:error="formError"
      :pending="add.isPending.value"
      @submit="addRoom"
    />
    <RoomRemarkDialog
      v-model:remark="remarkDraft"
      v-model:error="remarkError"
      :open="pendingRemark !== null"
      :room-id="pendingRemark?.id ?? null"
      :room-label="pendingRemark ? `直播间 ${pendingRemark.platformRoomId}` : ''"
      :pending="update.isPending.value"
      @update:open="closeRemarkDialog"
      @submit="saveRemark"
    />
    <UiAlert v-if="message" role="alert">
      {{ message }}
    </UiAlert>
    <UiConfirmDialog
      :open="pendingDelete !== null"
      title="删除直播间"
      :description="deleteDescription"
      confirm-label="删除直播间"
      variant="danger"
      @update:open="closeDeleteDialog"
      @confirm="deleteRoom"
    />
    <UiAsyncSection
      :pending="pages.isPending.value"
      :error="pages.error.value"
      loading-text="正在加载直播间…"
      @retry="pages.refetch()"
    >
      <UiEmptyState
        v-if="!rooms.length"
        :icon="Radio"
        title="还没有添加直播间"
        description="添加一个直播间即可创建弹幕任务。"
        action-label="添加直播间"
        @action="openForm"
      />
      <div
        v-else
        ref="roomContent"
        class="min-h-0 flex-1 overflow-y-auto pb-3"
        aria-label="直播间内容"
        tabindex="0"
      >
        <div
          :class="
            view === 'cards'
              ? 'grid grid-cols-1 gap-4 min-[601px]:grid-cols-2 min-[1101px]:grid-cols-3'
              : 'flex flex-col gap-3'
          "
        >
          <UiPanel
            v-for="(room, index) in rooms"
            :key="room.id"
            as="article"
            :padded="false"
            class="min-w-0 overflow-hidden"
          >
            <RoomCover v-if="view === 'cards'" :cover-url="room.coverUrl" :index="index" />
            <div
              class="p-5"
              :class="view === 'list' ? 'flex flex-col gap-4 pr-7 md:flex-row md:items-center' : ''"
            >
              <div :class="view === 'list' ? 'min-w-0 flex-1' : ''">
                <div :class="view === 'list' ? 'flex flex-wrap items-center gap-x-7 gap-y-2' : ''">
                  <div class="flex min-w-0 flex-wrap items-center gap-2">
                    <h2 class="min-w-0 truncate text-base font-semibold">
                      {{ roomDisplayName(room) }}
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
                    {{ view === 'list' ? '主播：' : '' }}{{ room.anchorName || '未知' }} ·
                    {{ room.platformRoomId }}
                  </p>
                </div>
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
                  <UiIconButton
                    :href="roomURL(room)"
                    target="_blank"
                    rel="noopener noreferrer"
                    :label="`打开直播间 ${room.platformRoomId}`"
                    title="打开直播间"
                    @click.prevent="openRoom(room)"
                  >
                    <ExternalLink class="size-4" aria-hidden="true" />
                  </UiIconButton>
                  <UiIconButton
                    :label="`修改 ${room.platformRoomId} 的备注`"
                    title="修改备注"
                    :disabled="update.isPending.value"
                    @click="openRemarkDialog(room)"
                  >
                    <Pencil class="size-4" aria-hidden="true" />
                  </UiIconButton>
                  <UiIconButton
                    variant="danger"
                    :label="`删除直播间 ${room.platformRoomId}`"
                    title="删除直播间"
                    :disabled="remove.isPending.value"
                    @click="requestDelete(room)"
                  >
                    <Trash2 class="size-4" aria-hidden="true" />
                  </UiIconButton>
                </div>
              </div>
            </div>
          </UiPanel>
        </div>
        <div class="mt-5 flex justify-center">
          <button
            v-if="pages.hasNextPage.value"
            type="button"
            :class="secondaryButton"
            :disabled="pages.isFetchingNextPage.value"
            :aria-busy="pages.isFetchingNextPage.value"
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
    </UiAsyncSection>
  </section>
</template>
