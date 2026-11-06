<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Account, Cooldown, RequestSummary } from '@/types'
import RequestsPanel from './RequestsPanel.vue'
import RecordsTable from './usage/RecordsTable.vue'
import UiSelect from './UiSelect.vue'

defineOptions({ name: 'RequestsPanel' })
const props = defineProps<{
  accounts: Account[]
  cooldowns: Cooldown[]
  requests: RequestSummary[]
  loading: boolean
  cooldownError: string
  requestError: string
}>()
const emit = defineEmits<{ refresh: []; notice: [message: string, tone: 'success' | 'error'] }>()
const view = ref('active')
const channel = ref('')
const state = ref('')
const search = ref('')
const status = ref('')
const refreshKey = ref(0)
const scope = computed(() => `${channel.value}/${state.value}`)
function recentQuery(): URLSearchParams {
  const query = new URLSearchParams({
    from: new Date(Date.now() - 7 * 86400000).toISOString(),
    to: new Date().toISOString(),
  })
  if (channel.value) query.set('channel', channel.value)
  if (state.value) query.set('state', state.value)
  return query
}
</script>

<template>
  <section class="flex min-h-0 flex-1 flex-col">
    <nav aria-label="请求视图" class="flex gap-2 border-b border-line px-4 py-2 md:px-8">
      <button
        v-for="item in [
          { id: 'active', name: '当前请求与冷却' },
          { id: 'history', name: '历史请求' },
        ]"
        :key="item.id"
        type="button"
        class="min-h-11 rounded px-4 text-sm"
        :class="view === item.id ? 'bg-raised text-white' : 'text-gray-400 hover:text-white'"
        :aria-pressed="view === item.id"
        @click="view = item.id"
      >
        {{ item.name }}
      </button>
    </nav>
    <RequestsPanel
      v-if="view === 'active'"
      v-bind="props"
      @refresh="emit('refresh')"
      @notice="(message, tone) => emit('notice', message, tone)"
    />
    <div v-else class="min-h-0 flex-1 space-y-4 overflow-auto p-4 md:p-8">
      <div class="flex flex-wrap items-center gap-3">
        <label class="text-sm text-gray-400"
          >通道
          <UiSelect
            v-model="channel"
            class="ml-2 rounded border border-line bg-canvas p-2 text-sm text-gray-200"
            ><option value="">全部</option>
            <option value="playground">PG</option>
            <option value="assistant">Build 代码助手</option></UiSelect
          >
        </label>
        <label class="text-sm text-gray-400"
          >结果
          <UiSelect
            v-model="state"
            class="ml-2 rounded border border-line bg-canvas p-2 text-sm text-gray-200"
            ><option value="">全部</option>
            <option value="completed">完成</option>
            <option value="tool_calls">工具调用</option>
            <option value="failed">失败</option>
            <option value="cancelled">已取消</option>
            <option value="limited">输出上限</option></UiSelect
          >
        </label>
        <button
          type="button"
          class="ml-auto min-h-11 rounded border border-line px-4 text-sm text-gray-300"
          @click="refreshKey++"
        >
          刷新记录
        </button>
      </div>
      <p class="text-xs text-gray-500">
        最近 7 天。点击请求可查看关联 ID、完整重试记录和阶段耗时；更多时间范围见用量统计。
      </p>
      <RecordsTable
        v-model:status="status"
        v-model:search="search"
        :scope="scope"
        :base-query="recentQuery"
        :total="-1"
        :status-options="['200', '400', '401', '429', '499', '500', '502', '503']"
        :refresh-key="refreshKey"
      />
    </div>
  </section>
</template>
