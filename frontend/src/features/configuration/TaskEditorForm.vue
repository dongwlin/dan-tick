<script setup lang="ts">
import { computed, nextTick, shallowRef, watch } from 'vue'
import { RouterLink, onBeforeRouteLeave, useRouter } from 'vue-router'
import { ArrowLeft, Check } from 'lucide-vue-next'
import { useForm } from 'vee-validate'
import { z } from 'zod'
import type {
  Task,
  TaskInput,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { Kind as TaskKind } from '@bindings/github.com/dongwlin/dan-tick/internal/task/models'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import { useRoomOptions, useSaveTask } from './queries'
import { usePreferences } from '@/features/application/queries'
import UiButton from '@/components/ui/UiButton.vue'
import UiConfirmDialog from '@/components/ui/UiConfirmDialog.vue'
import UiField from '@/components/ui/UiField.vue'
import { fieldClass, flatPanel } from '@/components/ui/uiStyles'
import { useToast } from '@/composables/useToast'
import { bindingError } from '@/lib/bindingError'

const props = defineProps<{ task?: Task }>()
const router = useRouter()
const rooms = useRoomOptions()
const preferences = usePreferences()
const save = useSaveTask(props.task?.id)
const saveWarning = shallowRef('')
const { notify, dismiss } = useToast()
const navigatingAfterSave = shallowRef(false)
const leaveDialogOpen = shallowRef(false)
let leaveDecision: ((leave: boolean) => void) | null = null

const weekdays = [
  { name: '周一', bit: 0 },
  { name: '周二', bit: 1 },
  { name: '周三', bit: 2 },
  { name: '周四', bit: 3 },
  { name: '周五', bit: 4 },
  { name: '周六', bit: 5 },
  { name: '周日', bit: 6 },
]

const schema = z
  .object({
    name: z.string().trim().min(1, '请输入任务名称').max(100, '任务名称过长'),
    roomId: z.string().regex(/^[1-9]\d*$/, '请选择直播间'),
    kind: z.enum(['single', 'loop']),
    messages: z.string().trim().min(1, '请填写至少一条弹幕'),
    interval: z.string(),
    maxCount: z.string(),
    maxDuration: z.string(),
    scheduleKind: z.enum(['once', 'daily', 'weekly']),
    localDate: z.string(),
    localTime: z.string().regex(/^\d{2}:\d{2}(:\d{2})?$/, '请选择执行时间'),
    timezone: z.string().trim().min(1, '请填写时区'),
    weekdays: z.array(z.number().int().min(0).max(6)),
    enabled: z.boolean(),
  })
  .superRefine((data, ctx) => {
    const messages = data.messages
      .split(/\r?\n/)
      .map((s) => s.trim())
      .filter(Boolean)
    if (data.kind === 'single' && messages.length !== 1)
      ctx.addIssue({ code: 'custom', path: ['messages'], message: '单次任务只能填写一条弹幕' })
    if (data.kind === 'loop') {
      const positive = (value: string) => /^[1-9]\d*$/.test(value)
      if (!positive(data.interval))
        ctx.addIssue({ code: 'custom', path: ['interval'], message: '发送间隔必须为正整数' })
      if (data.maxCount && !positive(data.maxCount))
        ctx.addIssue({ code: 'custom', path: ['maxCount'], message: '次数必须为正整数' })
      if (data.maxDuration && !positive(data.maxDuration))
        ctx.addIssue({ code: 'custom', path: ['maxDuration'], message: '时长必须为正整数' })
      if (!data.maxCount && !data.maxDuration)
        ctx.addIssue({ code: 'custom', path: ['maxCount'], message: '至少填写一种停止条件' })
    }
    if (data.scheduleKind === 'once' && !/^\d{4}-\d{2}-\d{2}$/.test(data.localDate))
      ctx.addIssue({ code: 'custom', path: ['localDate'], message: '请选择执行日期' })
    if (data.scheduleKind === 'weekly' && !data.weekdays.length)
      ctx.addIssue({ code: 'custom', path: ['weekdays'], message: '请至少选择一天' })
  })

type EditorValues = z.input<typeof schema>
const original = props.task
const { defineField, values, errors, meta, handleSubmit, setFieldValue, resetForm } = useForm<
  EditorValues,
  EditorValues,
  typeof schema
>({
  validationSchema: schema,
  initialValues: {
    name: original?.name ?? '',
    roomId: original ? String(original.roomId) : '',
    kind: original?.kind === TaskKind.Loop ? 'loop' : 'single',
    messages: original?.messages?.join('\n') ?? '',
    interval: String(original?.intervalSeconds ?? 15),
    maxCount: String(original?.maxCount ?? 1),
    maxDuration: String(original?.maxDurationSeconds ?? ''),
    scheduleKind:
      original?.schedule.kind === ScheduleKind.Weekly
        ? 'weekly'
        : original?.schedule.kind === ScheduleKind.Once
          ? 'once'
          : 'daily',
    localDate: original?.schedule.localDate ?? '',
    localTime: original?.schedule.localTime ?? '21:00:00',
    timezone: original?.schedule.timezone ?? 'Asia/Shanghai',
    weekdays: original
      ? weekdays
          .filter((day) => (original.schedule.weekdays & (1 << day.bit)) !== 0)
          .map((day) => day.bit)
      : [0, 1, 2, 3, 4],
    enabled: original?.enabled ?? true,
  },
})
// Apply the persisted default only before the user edits a new task. Reset
// the initial value too, so asynchronously loading it doesn't mark a fresh
// editor as dirty or trigger an unnecessary leave confirmation.
watch(
  () => preferences.data.value?.defaultTimezone,
  (value) => {
    if (!original && value && !meta.value.dirty) {
      resetForm({ values: { ...values, timezone: value } })
    }
  },
  { immediate: true },
)
const [name, nameAttrs] = defineField('name')
const [roomId, roomAttrs] = defineField('roomId')
const [kind, kindAttrs] = defineField('kind')
const [messages, messagesAttrs] = defineField('messages')
const [interval, intervalAttrs] = defineField('interval')
const [maxCount, countAttrs] = defineField('maxCount')
const [maxDuration, durationAttrs] = defineField('maxDuration')
const [scheduleKind, scheduleAttrs] = defineField('scheduleKind')
const [localDate, dateAttrs] = defineField('localDate')
const [localTime, timeAttrs] = defineField('localTime')
const [timezone, timezoneAttrs] = defineField('timezone')
const [enabled] = defineField('enabled')

const fieldNames = {
  name: '任务名称',
  roomId: '目标直播间',
  messages: '发送队列',
  interval: '发送间隔',
  maxCount: '最大发送次数',
  maxDuration: '最长运行时长',
  localTime: '执行时间',
  localDate: '执行日期',
  timezone: '时区',
  weekdays: '选择星期',
} as const
type ErrorField = keyof typeof fieldNames

function focusErrorField(field: ErrorField) {
  document.getElementById(`task-${field}`)?.focus()
}

const messageCount = computed(
  () => values.messages.split(/\r?\n/).filter((line) => line.trim()).length,
)
function toggleWeekday(bit: number, selected: boolean) {
  setFieldValue(
    'weekdays',
    selected ? [...values.weekdays, bit] : values.weekdays.filter((d) => d !== bit),
  )
}
function askToLeave(): Promise<boolean> {
  leaveDialogOpen.value = true
  return new Promise((resolve) => {
    leaveDecision = resolve
  })
}

function decideLeave(leave: boolean) {
  leaveDialogOpen.value = false
  const resolve = leaveDecision
  leaveDecision = null
  resolve?.(leave)
}

function handleLeaveDialogOpen(open: boolean) {
  if (!open) decideLeave(false)
}

onBeforeRouteLeave(() => {
  if (!navigatingAfterSave.value && meta.value.dirty) return askToLeave()
})

const submit = handleSubmit(
  async (data) => {
    dismiss()
    saveWarning.value = ''
    const input: TaskInput = {
      name: data.name.trim(),
      roomId: Number(data.roomId),
      kind: data.kind === 'loop' ? TaskKind.Loop : TaskKind.Single,
      messages: data.messages
        .split(/\r?\n/)
        .map((s) => s.trim())
        .filter(Boolean),
      intervalSeconds: data.kind === 'loop' ? Number(data.interval) : null,
      maxCount: data.kind === 'loop' && data.maxCount ? Number(data.maxCount) : null,
      maxDurationSeconds:
        data.kind === 'loop' && data.maxDuration ? Number(data.maxDuration) : null,
      schedule: {
        kind:
          data.scheduleKind === 'once'
            ? ScheduleKind.Once
            : data.scheduleKind === 'weekly'
              ? ScheduleKind.Weekly
              : ScheduleKind.Daily,
        localDate: data.scheduleKind === 'once' ? data.localDate : '',
        localTime: data.localTime.length === 5 ? `${data.localTime}:00` : data.localTime,
        timezone: data.timezone.trim(),
        weekdays:
          data.scheduleKind === 'weekly'
            ? data.weekdays.reduce((bits, day) => bits | (1 << day), 0)
            : 0,
      },
      enabled: data.enabled,
    }
    try {
      await save.mutateAsync(input)
      navigatingAfterSave.value = true
      await router.push('/tasks')
    } catch (error) {
      const message = bindingError(error)
      if (message === '修改已保存，但定时调度同步失败，请重新启动应用后检查') {
        // This result needs to remain visible: the database write may have succeeded.
        saveWarning.value = message
      } else {
        notify(message, 'error')
      }
    }
  },
  ({ errors: invalidErrors }) => {
    const invalidFields = (Object.keys(fieldNames) as ErrorField[]).filter((field) =>
      Boolean(invalidErrors[field]),
    )
    notify('表单校验未通过，请根据字段提示修改后重试。', 'error')
    void nextTick(() => {
      if (invalidFields[0]) focusErrorField(invalidFields[0])
    })
  },
)
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col gap-3">
    <RouterLink
      to="/tasks"
      class="inline-flex w-fit items-center gap-2 text-sm text-text-secondary hover:text-text-primary"
      ><ArrowLeft class="size-4" aria-hidden="true" />返回任务列表</RouterLink
    >
    <header>
      <h1>{{ task ? '编辑任务' : '新建任务' }}</h1>
    </header>
    <form
      class="grid min-h-0 flex-1 grid-cols-1 gap-4 lg:grid-cols-[minmax(0,1fr)_280px]"
      novalidate
      @submit.prevent="submit"
    >
      <div
        class="min-h-0 space-y-4 overflow-y-auto pb-4 lg:pr-2"
        tabindex="0"
        aria-label="任务配置表单"
      >
        <section :class="[flatPanel, 'p-5']">
          <h2 class="mb-4 text-base font-semibold">基本信息</h2>
          <div class="grid gap-4 md:grid-cols-2">
            <UiField
              id="task-name"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="任务名称"
              :error="errors.name"
              class="md:col-span-2"
            >
              <input
                :id="id"
                v-model="name"
                v-bind="nameAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                placeholder="例如：每日晚安问候"
              />
            </UiField>
            <UiField
              id="task-roomId"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="目标直播间"
              :error="errors.roomId"
            >
              <select
                v-if="rooms.isPending.value || (!rooms.isError.value && rooms.data.value?.length)"
                :id="id"
                v-model="roomId"
                v-bind="roomAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                :disabled="rooms.isPending.value"
              >
                <option value="">
                  {{ rooms.isPending.value ? '正在读取直播间…' : '请选择直播间' }}
                </option>
                <option
                  v-for="room in rooms.data.value ?? []"
                  :key="room.id"
                  :value="String(room.id)"
                >
                  {{ room.remark || room.title || room.platformRoomId }} · {{ room.platformRoomId }}
                </option>
              </select>
              <button
                v-else-if="rooms.isError.value"
                :id="id"
                type="button"
                :class="[fieldClass, 'text-left text-soft-danger-fg']"
                :aria-label="`${bindingError(rooms.error.value)}，重试读取直播间`"
                @click="rooms.refetch()"
              >
                读取直播间失败，点击重试
              </button>
              <RouterLink
                v-else
                :id="id"
                to="/rooms"
                :class="[fieldClass, 'flex items-center text-accent-text']"
                >暂无直播间，先添加直播间</RouterLink
              >
            </UiField>
            <UiField id="task-kind" v-slot="{ id, ariaInvalid, describedBy }" label="任务类型">
              <select
                :id="id"
                v-model="kind"
                v-bind="kindAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
              >
                <option value="single">单次发送</option>
                <option value="loop">循环发送</option>
              </select>
            </UiField>
          </div>
          <div class="mt-4 flex items-center gap-2 text-sm">
            <input id="task-enabled" v-model="enabled" type="checkbox" />
            <label for="task-enabled">启用这个任务的自动执行</label>
          </div>
        </section>
        <section :class="[flatPanel, 'p-5']">
          <h2 class="mb-3 text-base font-semibold">弹幕内容</h2>
          <UiField
            id="task-messages"
            v-slot="{ id, ariaInvalid, describedBy }"
            label="发送队列"
            :error="errors.messages"
            description="每行一条，空行自动忽略；实际发送仍受平台规则限制。"
          >
            <textarea
              :id="id"
              v-model="messages"
              v-bind="messagesAttrs"
              :class="[fieldClass, 'resize-y']"
              :aria-invalid="ariaInvalid"
              :aria-describedby="describedBy"
              rows="5"
              placeholder="在这里输入弹幕内容…"
            />
          </UiField>
          <div v-if="kind === 'loop'" class="mt-4 grid gap-3 md:grid-cols-3">
            <UiField
              id="task-interval"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="发送间隔（秒）"
              :error="errors.interval"
            >
              <input
                :id="id"
                v-model="interval"
                v-bind="intervalAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                type="number"
                min="1"
                step="1"
              />
            </UiField>
            <UiField
              id="task-maxCount"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="最大发送次数"
              :error="errors.maxCount"
            >
              <input
                :id="id"
                v-model="maxCount"
                v-bind="countAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                type="number"
                min="1"
                step="1"
              />
            </UiField>
            <UiField
              id="task-maxDuration"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="最长运行时长（秒）"
              :error="errors.maxDuration"
            >
              <input
                :id="id"
                v-model="maxDuration"
                v-bind="durationAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                type="number"
                min="1"
                step="1"
              />
            </UiField>
          </div>
        </section>
        <section :class="[flatPanel, 'p-5']">
          <h2 class="mb-4 text-base font-semibold">执行计划</h2>
          <div class="grid gap-4 md:grid-cols-2">
            <UiField
              id="task-scheduleKind"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="调度方式"
            >
              <select
                :id="id"
                v-model="scheduleKind"
                v-bind="scheduleAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
              >
                <option value="daily">每天</option>
                <option value="weekly">每周</option>
                <option value="once">一次性</option>
              </select>
            </UiField>
            <UiField
              id="task-localTime"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="执行时间"
              :error="errors.localTime"
            >
              <input
                :id="id"
                v-model="localTime"
                v-bind="timeAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                type="time"
                step="1"
              />
            </UiField>
            <UiField
              v-if="scheduleKind === 'once'"
              id="task-localDate"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="执行日期"
              :error="errors.localDate"
            >
              <input
                :id="id"
                v-model="localDate"
                v-bind="dateAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                type="date"
              />
            </UiField>
            <UiField
              id="task-timezone"
              v-slot="{ id, ariaInvalid, describedBy }"
              label="时区（IANA）"
              :error="errors.timezone"
            >
              <input
                :id="id"
                v-model="timezone"
                v-bind="timezoneAttrs"
                :class="fieldClass"
                :aria-invalid="ariaInvalid"
                :aria-describedby="describedBy"
                placeholder="Asia/Shanghai"
              />
            </UiField>
          </div>
          <UiField
            v-if="scheduleKind === 'weekly'"
            id="task-weekdays"
            v-slot="{ describedBy }"
            group
            class="mt-4"
            label="选择星期"
            :error="errors.weekdays"
          >
            <div class="flex flex-wrap gap-2">
              <div
                v-for="day in weekdays"
                :key="day.bit"
                class="flex cursor-pointer items-center gap-1.5 rounded-lg border border-border-strong px-3 py-2 text-sm"
              >
                <input
                  :id="day.bit === 0 ? 'task-weekdays' : undefined"
                  type="checkbox"
                  :aria-label="day.name"
                  :aria-invalid="errors.weekdays ? true : undefined"
                  :aria-describedby="describedBy"
                  :checked="values.weekdays.includes(day.bit)"
                  @change="toggleWeekday(day.bit, ($event.target as HTMLInputElement).checked)"
                />
                {{ day.name }}
              </div>
            </div>
          </UiField>
        </section>
      </div>
      <aside class="flex flex-col gap-4">
        <section :class="[flatPanel, 'p-5']">
          <h2 class="mb-4 text-base font-semibold">执行预览</h2>
          <dl class="space-y-3 text-sm">
            <div class="flex justify-between gap-3">
              <dt class="text-text-tertiary">执行时间</dt>
              <dd class="text-right">
                {{
                  scheduleKind === 'once' ? localDate : scheduleKind === 'daily' ? '每天' : '每周'
                }}
                {{ localTime }}
              </dd>
            </div>
            <div class="flex justify-between gap-3">
              <dt class="text-text-tertiary">发送方式</dt>
              <dd>{{ kind === 'loop' ? '循环发送' : '单次发送' }}</dd>
            </div>
            <div class="flex justify-between gap-3">
              <dt class="text-text-tertiary">消息数量</dt>
              <dd>{{ messageCount }} 条</dd>
            </div>
          </dl>
          <p class="mt-4 text-xs text-text-tertiary">仅预览配置，不会立即发送弹幕。</p>
        </section>
        <UiButton
          type="submit"
          variant="primary"
          :loading="save.isPending.value"
          :disabled="rooms.isPending.value || !rooms.data.value?.length"
        >
          <Check class="size-4" aria-hidden="true" />{{ task ? '保存修改' : '创建任务' }}
        </UiButton>
        <UiButton to="/tasks">取消</UiButton>
        <p
          v-if="saveWarning"
          role="alert"
          class="rounded-lg border border-soft-warning-border bg-soft-warning-bg p-3 text-sm text-soft-warning-fg"
        >
          {{ saveWarning }}
        </p>
      </aside>
    </form>
    <UiConfirmDialog
      :open="leaveDialogOpen"
      dismissible
      title="放弃未保存的修改？"
      description="离开后，当前任务尚未保存的修改会丢失。"
      cancel-label="继续编辑"
      confirm-label="放弃更改"
      variant="danger"
      @update:open="handleLeaveDialogOpen"
      @confirm="decideLeave(true)"
      @cancel="decideLeave(false)"
    />
  </section>
</template>
