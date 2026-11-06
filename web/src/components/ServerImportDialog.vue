<script setup lang="ts">
import { ref } from 'vue'
import DetailDialog from './DetailDialog.vue'

const emit = defineEmits<{ close: []; saved: [] }>()
const text = ref('')
const label = ref('')
const locale = ref('')
const timezone = ref('')
const authUser = ref(0)
const files = ref<{ name: string; text: string }[]>([])
const busy = ref(false)
const message = ref('')

async function chooseFiles(event: Event): Promise<void> {
  const selected = (event.target as HTMLInputElement).files
  files.value = await Promise.all(
    Array.from(selected ?? []).map(async (file) => ({ name: file.name, text: await file.text() })),
  )
}

async function save(): Promise<void> {
  busy.value = true
  message.value = ''
  let saved = 0
  try {
    const sources = files.value.length > 0 ? files.value : [{ name: '', text: text.value }]
    for (const source of sources) {
      const response = await fetch('/api/accounts/import/state', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          data: source.text,
          label: label.value,
          locale: locale.value,
          timezone: timezone.value,
          auth_user: authUser.value,
        }),
      })
      const result = (await response.json()) as {
        error?: { message: string }
        complete_state?: boolean
      }
      if (!response.ok)
        throw new Error(
          `${source.name ? `${source.name}: ` : ''}${result.error?.message ?? '导入失败'}`,
        )
      saved++
    }
    text.value = ''
    files.value = []
    message.value = `已保存 ${saved} 个账户。新账户保持停用，可在列表中检查后启用。`
  } catch (error) {
    message.value = `已保存 ${saved} 个。${error instanceof Error ? error.message : '导入失败'}`
  } finally {
    busy.value = false
    if (saved > 0) emit('saved')
  }
}
</script>

<template>
  <DetailDialog :open="true" title="导入账户 · 仅保存" @close="!busy && emit('close')">
    <form
      class="w-full max-w-2xl space-y-4 rounded-xl border border-line bg-panel p-6 shadow-xl"
      @submit.prevent="save"
    >
      <h3 id="server-import-title" class="text-lg font-semibold text-white">导入账户 · 仅保存</h3>
      <p class="text-sm leading-6 text-gray-400">
        支持完整状态、Cookie JSON、Netscape 文件和 Cookie 请求头。导入时不访问
        Google。原电脑配对可在账户菜单中使用。
      </p>
      <label class="block text-sm text-gray-300"
        >账户名称或邮箱（完整状态已有身份时可留空）
        <input
          v-model.trim="label"
          autofocus
          class="mt-1 w-full rounded border border-line bg-canvas px-3 py-2.5 text-white"
          autocomplete="off"
        />
      </label>
      <label class="block text-sm text-gray-300"
        >粘贴导出内容
        <textarea
          v-model="text"
          rows="6"
          class="mt-1 w-full rounded border border-line bg-canvas p-3 font-mono text-sm text-white"
          spellcheck="false"
          :disabled="files.length > 0"
        />
      </label>
      <label class="block text-sm text-gray-300"
        >或选择文件（可多选）
        <input
          type="file"
          multiple
          accept=".json,.txt"
          class="mt-2 block w-full text-sm text-gray-400"
          @change="chooseFiles"
        />
      </label>
      <details class="text-sm text-gray-400">
        <summary class="min-h-11 cursor-pointer py-3">高级设置</summary>
        <p class="mb-3 leading-6">
          完整导出中的语言、时区和会话序号优先沿用；缺少时使用默认值，不视为恢复了原浏览器环境。
        </p>
        <div class="grid gap-3 sm:grid-cols-3">
          <label
            >语言<input
              v-model.trim="locale"
              placeholder="沿用或默认"
              class="mt-1 w-full rounded border border-line bg-canvas p-2 text-white"
          /></label>
          <label
            >时区<input
              v-model.trim="timezone"
              placeholder="沿用或默认"
              class="mt-1 w-full rounded border border-line bg-canvas p-2 text-white"
          /></label>
          <label
            >会话序号<input
              v-model.number="authUser"
              type="number"
              min="0"
              class="mt-1 w-full rounded border border-line bg-canvas p-2 text-white"
          /></label>
        </div>
      </details>
      <p class="text-xs leading-5 text-gray-500">
        Cookie
        请求头缺少原始域、路径和有效期，导入会记录为信息不完整。登录失效时需要重新导入，或由配对电脑推送新凭据。
      </p>
      <p v-if="message" role="status" class="text-sm text-blue-300">{{ message }}</p>
      <div class="flex justify-end gap-3">
        <button
          type="button"
          class="min-h-11 rounded border border-line px-4 text-sm text-gray-300"
          :disabled="busy"
          @click="emit('close')"
        >
          关闭
        </button>
        <button
          type="submit"
          class="min-h-11 rounded bg-blue-600 px-5 text-sm font-medium text-white disabled:opacity-50"
          :disabled="busy || (!text.trim() && files.length === 0)"
          :aria-busy="busy"
        >
          {{ busy ? '正在保存…' : '保存账户' }}
        </button>
      </div>
    </form>
  </DetailDialog>
</template>
