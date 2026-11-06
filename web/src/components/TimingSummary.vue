<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { RequestTiming } from '@/types'

const props = defineProps<{ timing?: RequestTiming | undefined; requestId?: string | undefined }>()
const loaded = ref<RequestTiming | null>(null)
const error = ref('')
watch(
  () => props.requestId,
  async (id, _, onCleanup) => {
    loaded.value = null
    error.value = ''
    if (!id || props.timing) return
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    try {
      const response = await fetch(`/api/requests/${encodeURIComponent(id)}/timing`, {
        signal: controller.signal,
      })
      if (!response.ok) {
        error.value = '此请求没有保留阶段耗时'
        return
      }
      loaded.value = (await response.json()) as RequestTiming
    } catch {
      if (!controller.signal.aborted) error.value = '阶段耗时读取失败'
    }
  },
  { immediate: true },
)
const report = computed(() => props.timing ?? loaded.value)
const values = computed(() => {
  const data = report.value
  if (!data) return []
  return [
    ['排队', data.stages_ms.queue],
    ['账号初始化', data.stages_ms.account_init],
    ['WAA 签名', data.stages_ms.waa_signing],
    ['Build 提交', data.stages_ms.build_submit],
    ['上游响应头', data.stages_ms.upstream_headers],
    ['首个思考', data.first_thought_ms],
    ['正文首字', data.first_body_ms],
    ['首个工具调用', data.first_tool_ms],
    ['生成完成', data.generation_ms],
    ['本地清理完成', data.cleanup_ms],
    ['后台清理完成', data.background_cleanup_ms],
  ] as const
})
function duration(value: number | null | undefined): string {
  return value == null ? '—' : `${(value / 1000).toFixed(2)} s`
}

function result(state: string): string {
  return (
    ({ completed: '完成', failed: '失败', running: '进行中' } as Record<string, string>)[state] ??
    state
  )
}
</script>

<template>
  <section class="rounded-lg border border-line bg-canvas p-4">
    <h3 class="text-sm font-medium text-gray-200">阶段耗时</h3>
    <p v-if="error" role="status" class="mt-2 text-xs text-gray-500">{{ error }}</p>
    <template v-else-if="report">
      <p class="mt-1 text-xs leading-5 text-gray-500">
        首字与完成时间从请求进入服务开始计算；排队、初始化、签名和提交为各次尝试累计耗时，阶段可能重叠。
      </p>
      <dl class="mt-3 grid grid-cols-1 gap-x-6 gap-y-2 text-xs sm:grid-cols-2">
        <div v-for="[label, value] in values" :key="label" class="flex justify-between gap-3">
          <dt class="text-gray-400">{{ label }}</dt>
          <dd class="font-mono text-gray-200">{{ duration(value) }}</dd>
        </div>
      </dl>
      <p class="mt-3 text-xs text-gray-500">
        正文事件 {{ report.body_events }} · 思考事件 {{ report.thought_events }} · 工具调用
        {{ report.tool_events }}。未出现的输出显示“—”。
      </p>
      <ol v-if="report.attempts?.length" class="mt-3 space-y-1 text-xs text-gray-400">
        <li v-for="attempt in report.attempts" :key="attempt.number">
          尝试 {{ attempt.number }} ·
          {{ attempt.channel === 'assistant' ? 'Build 代码助手' : 'PG' }} ·
          {{ duration(attempt.duration_ms) }} · {{ result(attempt.state) }}
        </li>
      </ol>
      <p v-if="report.upstream_rounds?.length" class="mt-3 text-xs text-gray-500">
        上游生成轮次：{{ report.upstream_rounds.length }}（包括内部重试）
      </p>
      <ol class="mt-1 space-y-1 text-xs text-gray-400">
        <li v-for="round in report.upstream_rounds ?? []" :key="round.number">
          第 {{ round.number }} 轮 · {{ duration(round.duration_ms) }} · {{ result(round.state) }}
        </li>
      </ol>
    </template>
    <p v-else class="mt-2 text-xs text-gray-500" role="status">正在读取…</p>
  </section>
</template>
