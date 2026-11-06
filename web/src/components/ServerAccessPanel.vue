<script setup lang="ts">
import { computed, ref } from 'vue'
import type { Model, ServiceConfig } from '@/types'
import CopyButton from './CopyButton.vue'

defineOptions({ name: 'AccessPanel' })
const props = defineProps<{ config: ServiceConfig | null; models: Model[] }>()
defineEmits<{ settings: [] }>()
const address = `${window.location.origin}/v1`
const reveal = ref(false)
const key = computed(
  () => (props.config?.active_proxy_api_key ?? '').split(',').find(Boolean) ?? '',
)
</script>

<template>
  <section class="min-h-0 flex-1 overflow-auto px-4 py-6 md:px-8">
    <div class="mx-auto max-w-3xl space-y-6">
      <header>
        <h2 class="text-xl font-semibold text-white">连接 SillyTavern</h2>
        <p class="mt-2 text-sm text-gray-400">聊天补全 · 自定义 OpenAI 兼容接口</p>
      </header>
      <div class="rounded-lg border border-line bg-panel p-5">
        <p class="mb-2 text-sm font-medium text-gray-200">服务地址</p>
        <div class="flex min-h-11 items-center gap-3 rounded border border-line bg-canvas px-3">
          <code class="min-w-0 flex-1 break-all text-sm text-blue-300">{{ address }}</code>
          <CopyButton :text="address" label="复制服务地址" />
        </div>
        <p class="mt-5 mb-2 text-sm font-medium text-gray-200">调用密钥</p>
        <div class="flex min-h-11 items-center gap-3 rounded border border-line bg-canvas px-3">
          <code class="min-w-0 flex-1 break-all text-sm text-gray-200">{{
            key ? (reveal ? key : '••••••••••••') : '尚未设置，模型调用已关闭'
          }}</code>
          <button
            v-if="key"
            type="button"
            class="min-h-11 text-sm text-blue-400"
            :aria-pressed="reveal"
            @click="reveal = !reveal"
          >
            {{ reveal ? '隐藏' : '显示' }}
          </button>
          <CopyButton v-if="key" :text="key" label="复制调用密钥" />
        </div>
        <button
          type="button"
          class="mt-3 min-h-11 text-sm text-blue-400"
          @click="$emit('settings')"
        >
          管理调用密钥
        </button>
      </div>
      <ol class="list-inside list-decimal space-y-3 text-sm leading-7 text-gray-300">
        <li>在连接设置中选择 Chat Completion，再选择 Custom（OpenAI 兼容）。</li>
        <li>填入上面的服务地址和调用密钥，读取模型列表。</li>
        <li>日常聊天选择普通模型；以 <code>-build</code> 结尾的模型使用独立的 Build 代码助手。</li>
        <li>开启流式输出；需要工具时，在 SillyTavern 中开启 Function Calling。</li>
      </ol>
      <p class="rounded border border-line bg-panel p-4 text-sm leading-6 text-gray-400">
        Gemini 接口保留在
        <code>/v1beta</code>，支持生成、流式生成、模型列表和计数。两种接口共用账号池与统计。
      </p>
    </div>
  </section>
</template>
