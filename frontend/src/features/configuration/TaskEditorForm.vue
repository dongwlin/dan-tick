<script setup lang="ts">
import { computed, shallowRef, watch } from 'vue'
import { RouterLink, onBeforeRouteLeave, useRouter } from 'vue-router'
import { ArrowLeft, Check, Clock3, Info } from 'lucide-vue-next'
import { useForm } from 'vee-validate'
import { z } from 'zod'
import type {
  Task,
  TaskInput,
} from '@bindings/github.com/dongwlin/dan-tick/internal/desktop/models'
import { Kind as TaskKind } from '@bindings/github.com/dongwlin/dan-tick/internal/task/models'
import { Kind as ScheduleKind } from '@bindings/github.com/dongwlin/dan-tick/internal/schedule/models'
import { bindingError, useRoomOptions, useSaveTask } from './queries'
import { usePreferences } from '@/features/application/queries'
import { fieldClass, flatPanel, labelClass, primaryButton, secondaryButton } from './style'

const props = defineProps<{ task?: Task }>()
const router = useRouter()
const rooms = useRoomOptions()
const preferences = usePreferences()
const save = useSaveTask(props.task?.id)
const errorMessage = shallowRef('')
const navigatingAfterSave = shallowRef(false)

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

const messageCount = computed(
  () => values.messages.split(/\r?\n/).filter((line) => line.trim()).length,
)
function toggleWeekday(bit: number, selected: boolean) {
  setFieldValue(
    'weekdays',
    selected ? [...values.weekdays, bit] : values.weekdays.filter((d) => d !== bit),
  )
}
onBeforeRouteLeave(() => {
  if (!navigatingAfterSave.value && meta.value.dirty)
    return window.confirm('任务尚未保存，确定放弃修改并离开吗？')
})

const submit = handleSubmit(async (data) => {
  errorMessage.value = ''
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
    maxDurationSeconds: data.kind === 'loop' && data.maxDuration ? Number(data.maxDuration) : null,
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
    errorMessage.value = bindingError(error)
  }
})
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
          <h2 class="text-base font-semibold">基本信息</h2>
          <p class="mb-4 text-sm text-text-secondary">先决定把什么内容发到哪里</p>
          <div class="grid gap-4 md:grid-cols-2">
            <div :class="[labelClass, 'md:col-span-2']">
              任务名称
              <input
                v-model="name"
                v-bind="nameAttrs"
                :class="fieldClass"
                placeholder="例如：每日晚安问候"
                aria-label="任务名称"
              />
              <span v-if="errors.name" class="text-xs text-soft-danger-fg">{{ errors.name }}</span>
            </div>
            <div :class="labelClass">
              目标直播间
              <select
                v-model="roomId"
                v-bind="roomAttrs"
                :class="fieldClass"
                aria-label="目标直播间"
              >
                <option value="">请选择直播间</option>
                <option
                  v-for="room in rooms.data.value ?? []"
                  :key="room.id"
                  :value="String(room.id)"
                >
                  {{ room.remark || room.title || room.platformRoomId }} · {{ room.platformRoomId }}
                </option>
              </select>
              <span v-if="errors.roomId" class="text-xs text-soft-danger-fg">{{
                errors.roomId
              }}</span>
              <span v-if="rooms.isError.value" role="alert" class="text-xs text-soft-danger-fg"
                >{{ bindingError(rooms.error.value) }}
                <button type="button" class="underline" @click="rooms.refetch()">重试</button></span
              >
              <RouterLink
                v-if="!rooms.isPending.value && !rooms.data.value?.length"
                to="/rooms"
                class="text-xs text-accent-text"
                >先添加直播间</RouterLink
              >
            </div>
            <div :class="labelClass">
              任务类型
              <select v-model="kind" v-bind="kindAttrs" :class="fieldClass" aria-label="任务类型">
                <option value="single">单次发送</option>
                <option value="loop">循环发送</option>
              </select>
            </div>
          </div>
          <div class="mt-4 flex items-center gap-2 text-sm">
            <input
              v-model="enabled"
              type="checkbox"
              aria-label="启用自动执行"
            />启用这个任务的自动执行
          </div>
        </section>
        <section :class="[flatPanel, 'p-5']">
          <h2 class="text-base font-semibold">弹幕内容</h2>
          <p class="mb-3 text-sm text-text-secondary">
            {{ kind === 'loop' ? '每行一条，按照顺序依次发送，到末尾后循环' : '单次发送一条弹幕' }}
          </p>
          <div :class="labelClass">
            发送队列
            <textarea
              v-model="messages"
              v-bind="messagesAttrs"
              :class="fieldClass"
              rows="5"
              class="resize-y"
              placeholder="在这里输入弹幕内容…"
              aria-label="发送队列"
            />
            <span v-if="errors.messages" class="text-xs text-soft-danger-fg">{{
              errors.messages
            }}</span>
          </div>
          <p class="mt-2 flex items-center gap-2 text-xs text-text-tertiary">
            <Info
              class="size-4"
              aria-hidden="true"
            />每行一条，空行自动忽略；实际发送仍受平台规则限制。
          </p>
          <div v-if="kind === 'loop'" class="mt-4 grid gap-3 md:grid-cols-3">
            <div :class="labelClass">
              发送间隔（秒）
              <input
                v-model="interval"
                v-bind="intervalAttrs"
                :class="fieldClass"
                type="number"
                min="1"
                step="1"
                aria-label="发送间隔"
              />
              <span v-if="errors.interval" class="text-xs text-soft-danger-fg">{{
                errors.interval
              }}</span>
            </div>
            <div :class="labelClass">
              最大发送次数
              <input
                v-model="maxCount"
                v-bind="countAttrs"
                :class="fieldClass"
                type="number"
                min="1"
                step="1"
                aria-label="最大发送次数"
              />
              <span v-if="errors.maxCount" class="text-xs text-soft-danger-fg">{{
                errors.maxCount
              }}</span>
            </div>
            <div :class="labelClass">
              最长运行时长（秒）
              <input
                v-model="maxDuration"
                v-bind="durationAttrs"
                :class="fieldClass"
                type="number"
                min="1"
                step="1"
                aria-label="最长运行时长"
              />
              <span v-if="errors.maxDuration" class="text-xs text-soft-danger-fg">{{
                errors.maxDuration
              }}</span>
            </div>
          </div>
        </section>
        <section :class="[flatPanel, 'p-5']">
          <h2 class="text-base font-semibold">执行计划</h2>
          <p class="mb-4 text-sm text-text-secondary">指定触发方式和时间</p>
          <div class="grid gap-4 md:grid-cols-2">
            <div :class="labelClass">
              调度方式
              <select
                v-model="scheduleKind"
                v-bind="scheduleAttrs"
                :class="fieldClass"
                aria-label="调度方式"
              >
                <option value="daily">每天</option>
                <option value="weekly">每周</option>
                <option value="once">一次性</option>
              </select>
            </div>
            <div :class="labelClass">
              执行时间
              <input
                v-model="localTime"
                v-bind="timeAttrs"
                :class="fieldClass"
                type="time"
                step="1"
                aria-label="执行时间"
              />
              <span v-if="errors.localTime" class="text-xs text-soft-danger-fg">{{
                errors.localTime
              }}</span>
            </div>
            <div v-if="scheduleKind === 'once'" :class="labelClass">
              执行日期
              <input
                v-model="localDate"
                v-bind="dateAttrs"
                :class="fieldClass"
                type="date"
                aria-label="执行日期"
              />
              <span v-if="errors.localDate" class="text-xs text-soft-danger-fg">{{
                errors.localDate
              }}</span>
            </div>
            <div :class="labelClass">
              时区（IANA）
              <input
                v-model="timezone"
                v-bind="timezoneAttrs"
                :class="fieldClass"
                aria-label="时区"
                placeholder="Asia/Shanghai"
              />
              <span v-if="errors.timezone" class="text-xs text-soft-danger-fg">{{
                errors.timezone
              }}</span>
            </div>
          </div>
          <fieldset v-if="scheduleKind === 'weekly'" class="mt-4">
            <legend class="mb-2 text-sm text-text-secondary">选择星期</legend>
            <div class="flex flex-wrap gap-2">
              <div
                v-for="day in weekdays"
                :key="day.bit"
                class="flex cursor-pointer items-center gap-1.5 rounded-lg border border-border-strong px-3 py-2 text-sm"
              >
                <input
                  type="checkbox"
                  :aria-label="day.name"
                  :checked="values.weekdays.includes(day.bit)"
                  @change="toggleWeekday(day.bit, ($event.target as HTMLInputElement).checked)"
                />
                {{ day.name }}
              </div>
            </div>
            <span v-if="errors.weekdays" class="text-xs text-soft-danger-fg">{{
              errors.weekdays
            }}</span>
          </fieldset>
          <p class="mt-3 flex items-center gap-2 text-xs text-text-tertiary">
            <Clock3
              class="size-4"
              aria-hidden="true"
            />执行计划使用明确时区，保存后由后端计算下次执行时间。
          </p>
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
        <p
          v-if="errorMessage"
          role="alert"
          class="rounded-lg bg-soft-danger-bg p-3 text-sm text-soft-danger-fg"
        >
          {{ errorMessage }}
        </p>
        <p v-if="Object.keys(errors).length" role="alert" class="text-sm text-soft-danger-fg">
          请检查表单中标出的字段。
        </p>
        <button
          type="submit"
          :disabled="save.isPending.value || rooms.isPending.value || !rooms.data.value?.length"
          :class="primaryButton"
        >
          <Check class="size-4" aria-hidden="true" />{{
            save.isPending.value ? '保存中…' : task ? '保存修改' : '创建任务'
          }}
        </button>
        <RouterLink to="/tasks" :class="secondaryButton">取消</RouterLink>
      </aside>
    </form>
  </section>
</template>
