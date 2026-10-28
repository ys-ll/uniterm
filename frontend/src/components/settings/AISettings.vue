<template>
  <div class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.ai') }}</h2>

    <div class="settings-group">
      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.maxTurns') }}</div>
          <div class="setting-desc">{{ t('settings.maxTurnsDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-input-number
            v-model="settingsStore.settings.ai.maxTurns"
            :min="0"
            :max="100"
            controls-position="right"
            @change="settingsStore.save()"
          />
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.modelList') }}</div>
          <div class="setting-desc">{{ t('settings.modelListDesc') }}</div>
        </div>
        <div class="setting-control action-control">
          <el-button @click="openNewModelForm"><Plus :size="'0.875rem'" /> {{ t('settings.addModel') }}</el-button>
        </div>
      </div>

      <div
        v-for="model in settingsStore.settings.ai.models"
        :key="model.id"
        class="model-card"
        :class="{ active: model.id === settingsStore.settings.ai.activeModelId }"
      >
        <div class="model-main">
          <el-radio
            :model-value="settingsStore.settings.ai.activeModelId"
            :value="model.id"
            @change="settingsStore.setActiveModel(model.id)"
          >
            <span class="model-name">{{ model.name }}</span>
          </el-radio>
          <span class="model-detail">{{ model.model }} @ {{ model.baseURL }}</span>
        </div>
        <div class="model-actions">
          <el-button link :title="t('common.copy')" @click="duplicateModel(model)">
            <el-icon><Copy :size="'0.875rem'" /></el-icon>
          </el-button>
          <el-button link @click="editModel(model)">
            <el-icon><Pencil :size="'0.875rem'" /></el-icon>
          </el-button>
          <el-button link type="danger" @click="removeModelConfirm(model)">
            <el-icon><Trash2 :size="'0.875rem'" /></el-icon>
          </el-button>
        </div>
      </div>
    </div>

    <!-- Model Form Dialog -->
    <el-dialog append-to-body v-model="showModelForm" :title="editingModel ? t('settings.editModel') : t('settings.newModel')" width="25rem">
      <el-form label-width="5rem">
        <el-form-item :label="t('settings.modelProvider')">
          <button type="button" class="provider-trigger" @click.stop="providerMenuRef?.toggle($event.currentTarget)">
            <span class="provider-trigger-text">{{ providerDisplay }}</span>
            <ChevronDown :size="14" class="menu-icon-trailing" />
          </button>
          <Menu ref="providerMenuRef" v-model:visible="providerMenuVisible">
            <MenuItem :class="{ active: selectedProvider === 'custom' }" @click="onProviderSelect('custom')">
              {{ t('settings.providerCustom') }}
            </MenuItem>
            <template v-for="group in providerGroups" :key="group.key">
              <MenuDivider />
              <div class="menu-group-label">{{ t(group.nameKey) }}</div>
              <template v-for="e in group.entries" :key="e.key">
                <!-- Providers with a single access type go straight on the
                     list; only multi-endpoint ones get a submenu. -->
                <MenuSubmenu
                  v-if="e.provider.endpoints.length > 1"
                  :label="e.provider.name"
                >
                  <MenuItem
                    v-for="(endpoint, i) in e.provider.endpoints"
                    :key="i"
                    :class="{ active: selectedProvider === `${e.key}:${i}` }"
                    @click="onProviderSelect(`${e.key}:${i}`)"
                  >{{ endpoint.labelKey ? t(endpoint.labelKey) : endpoint.label }}</MenuItem>
                </MenuSubmenu>
                <MenuItem
                  v-else
                  :class="{ active: selectedProvider === `${e.key}:0` }"
                  @click="onProviderSelect(`${e.key}:0`)"
                >{{ e.provider.name }}</MenuItem>
              </template>
            </template>
          </Menu>
        </el-form-item>
        <el-form-item :label="t('settings.modelName')">
          <el-input v-model="modelForm.name" />
        </el-form-item>
        <el-form-item :label="t('settings.modelProtocol')">
          <el-select v-model="modelForm.protocol" style="width: 100%">
            <el-option label="Anthropic Messages API" value="anthropic" />
            <el-option label="OpenAI Chat Completions API" value="openai" />
            <el-option label="OpenAI Responses API" value="responses" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('settings.modelBaseURL')">
          <el-input v-model="modelForm.baseURL" :placeholder="modelForm.protocol === 'anthropic' ? 'https://api.anthropic.com' : 'https://api.openai.com/v1'" />
        </el-form-item>
        <el-form-item :label="t('settings.modelUserAgent')">
          <el-select v-model="modelForm.userAgent" style="width: 100%" filterable allow-create>
            <el-option
              v-for="ua in USER_AGENT_PRESETS"
              :key="ua.value"
              :label="ua.label"
              :value="ua.value"
            />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('settings.modelApiKey')">
          <el-input v-model="modelForm.apiKey" type="password" show-password />
        </el-form-item>
        <el-form-item :label="t('conn.proxy')">
          <div class="inline-add-row">
            <el-select v-model="modelForm.proxyId" clearable filterable style="flex: 1; min-width: 0" :placeholder="t('settings.modelProxyPlaceholder')">
              <el-option :label="t('conn.proxySystemOption')" value="system" />
              <el-option
                v-for="p in proxyStore.proxies"
                :key="p.id"
                :label="`${p.name} (${p.kind} ${p.host}:${p.port})`"
                :value="p.id"
                :disabled="p.enabled === false"
              />
            </el-select>
            <el-button class="inline-add-btn" :title="t('conn.newProxy')" @click="modelProxyDialogVisible = true">
              <Plus :size="'0.875rem'" />
            </el-button>
          </div>
        </el-form-item>
        <el-form-item :label="t('settings.modelSessionHeader')">
          <el-input v-model="modelForm.sessionHeader" :placeholder="t('settings.modelSessionHeaderPlaceholder')" />
        </el-form-item>
        <el-form-item :label="t('settings.modelModel')">
          <div class="model-fetch-row">
            <el-select
              v-model="modelForm.model"
              class="model-autocomplete"
              filterable
              allow-create
              default-first-option
              :reserve-keyword="false"
              :placeholder="t('settings.modelModelPlaceholder')"
            >
              <el-option
                v-for="s in modelSelectOptions"
                :key="s.value"
                :label="s.label"
                :value="s.value"
              />
            </el-select>
            <el-button :loading="modelFetching" @click="fetchModelList">
              {{ t('settings.fetchModels') }}
            </el-button>
          </div>
        </el-form-item>
        <el-form-item>
          <el-button :loading="testingConnection" @click="testConnection">
            {{ t('settings.testConnection') }}
          </el-button>
          <span v-if="testResult != null" :class="testResult ? 'test-ok' : 'test-fail'" style="margin-left: 0.5rem; font-size: 0.8125rem;">
            {{ testResult ? t('settings.testSuccess') : t('settings.testFailed') }}
          </span>
          <span v-if="testError" style="margin-left: 0.5rem; font-size: 0.75rem; color: var(--error); word-break: break-all;">{{ testError }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showModelForm = false">{{ t('settings.cancel') }}</el-button>
        <el-button type="primary" @click="saveModel">{{ t('settings.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- Quick-create proxy from the model form's "+" button; must live outside
         the per-category v-if sections so it renders on the AI tab too. -->
    <ProxyEditDialog v-model:visible="modelProxyDialogVisible" :proxy="null" @saved="onModelProxySaved" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, computed, onMounted } from 'vue'
import { Plus, Pencil, Copy, Trash2, ChevronDown } from '@lucide/vue'
import { msg } from '../../services/message'
import { FetchModels, ChatCompletion } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useSettingsStore } from '../../stores/settingsStore'
import { useProxyStore } from '../../stores/proxyStore'
import { useI18n } from '../../i18n'
import { ElMessageBox } from 'element-plus'
import { USER_AGENT_PRESETS } from '../../types/settings'
import type { AIModelConfig } from '../../types/settings'
import { backendErrorText } from '../../utils/backendError'
import type { Proxy } from '../../types/proxy'
import Menu from '../Menu.vue'
import MenuItem from '../MenuItem.vue'
import MenuSubmenu from '../MenuSubmenu.vue'
import MenuDivider from '../MenuDivider.vue'
import ProxyEditDialog from '../ProxyEditDialog.vue'

defineProps<{ isNarrow: boolean }>()

const settingsStore = useSettingsStore()
const proxyStore = useProxyStore()
const { t } = useI18n()

// Ensure proxy list is available for the model form's proxy dropdown.
// (The original component loaded all stores in a single onMounted.)
onMounted(() => {
  proxyStore.load()
})

const showModelForm = ref(false)
const modelSuggestions = ref<Array<{ value: string; label: string }>>([])
// Always surface the currently-set model as an option so el-select renders it
// when editing an existing model (before any fetch) — el-select won't display a
// bound value that has no matching option, and allow-create only creates
// options for values typed during the session, not a pre-set v-model.
const modelSelectOptions = computed(() => {
  const opts = modelSuggestions.value.slice()
  const cur = modelForm.model?.trim()
  if (cur && !opts.some(o => o.value === cur)) {
    opts.unshift({ value: cur, label: cur })
  }
  return opts
})
const modelFetching = ref(false)
const testingConnection = ref(false)
const testResult = ref<boolean | null>(null)
const testError = ref('')
const editingModel = ref<AIModelConfig | null>(null)
const modelForm = reactive({
  id: '',
  name: '',
  baseURL: '',
  model: '',
  apiKey: '',
  protocol: 'anthropic' as 'anthropic' | 'openai' | 'responses',
  userAgent: 'uniTerm' as string,
  proxyId: '' as string,
  sessionHeader: '' as string,
})

// ── Provider quick presets ───────────────────────────────────────
interface ProviderEndpoint {
  /** i18n key for the label (e.g. pay-as-you-go); omit to use `label` as-is. */
  labelKey?: string
  /** Raw label, kept in English for plan names ("Coding Plan", "Agent Plan"). */
  label?: string
  protocol: 'anthropic' | 'openai' | 'responses'
  baseURL: string
}

interface ProviderPreset {
  /** Brand name, kept in English everywhere (not translated). */
  name: string
  endpoints: ProviderEndpoint[]
}

const EP_PAYG = 'settings.endpointPayAsYouGo'

const PROVIDER_PRESETS: Record<string, ProviderPreset> = {
  openai: { name: 'OpenAI', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.openai.com/v1' }] },
  anthropic: { name: 'Anthropic', endpoints: [{ labelKey: EP_PAYG, protocol: 'anthropic', baseURL: 'https://api.anthropic.com' }] },
  google: { name: 'Google AI', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://generativelanguage.googleapis.com/v1beta' }] },
  xai: { name: 'xAI', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.x.ai/v1' }] },
  mistral: { name: 'Mistral AI', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.mistral.ai/v1' }] },
  deepseek: { name: 'DeepSeek', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.deepseek.com/v1' }] },
  alibaba: { name: 'Alibaba Cloud', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://dashscope.aliyuncs.com/compatible-mode/v1' },
    { label: 'Coding Plan', protocol: 'anthropic', baseURL: 'https://coding.dashscope.aliyuncs.com/apps/anthropic' },
  ] },
  zhipu: { name: 'Zhipu AI', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://open.bigmodel.cn/api/paas/v4' },
    { label: 'Coding Plan', protocol: 'openai', baseURL: 'https://open.bigmodel.cn/api/coding/paas/v4' },
  ] },
  baidu: { name: 'Baidu Cloud', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://qianfan.baidubce.com/v2' },
    { label: 'Token Plan', protocol: 'openai', baseURL: 'https://qianfan.baidubce.com/v2/tokenplan/personal' },
  ] },
  moonshot: { name: 'Moonshot AI', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.moonshot.cn/v1' },
    { label: 'Kimi For Coding', protocol: 'anthropic', baseURL: 'https://api.kimi.com/coding/v1' },
  ] },
  minimax: { name: 'MiniMax', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.minimax.cn/v1' },
    { label: 'Coding Plan', protocol: 'anthropic', baseURL: 'https://api.minimax.cn/anthropic' },
  ] },
  mimo: { name: 'Xiaomi MiMo', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.xiaomimimo.com/v1' },
    { label: 'Token Plan', protocol: 'openai', baseURL: 'https://token-plan-cn.xiaomimimo.com/v1' },
  ] },
  longcat: { name: 'LongCat', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.longcat.chat/openai/v1' }] },
  volcengine: { name: 'Volcengine', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://ark.cn-beijing.volces.com/api/v3' },
    { label: 'Coding Plan', protocol: 'openai', baseURL: 'https://ark.cn-beijing.volces.com/api/coding/v3' },
    { label: 'Agent Plan', protocol: 'openai', baseURL: 'https://ark.cn-beijing.volces.com/api/plan/v3' },
  ] },
  byteplus: { name: 'BytePlus', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://ark.ap-southeast.bytepluses.com/api/v3' },
    { label: 'Coding Plan', protocol: 'openai', baseURL: 'https://ark.ap-southeast.bytepluses.com/api/coding/v3' },
  ] },
  tencent: { name: 'Tencent Cloud', endpoints: [
    { labelKey: EP_PAYG, protocol: 'openai', baseURL: 'https://api.hunyuan.cloud.tencent.com/v1' },
    { label: 'Token Plan', protocol: 'openai', baseURL: 'https://api.lkeap.cloud.tencent.com/plan/v3' },
  ] },
  ollama: { name: 'Ollama', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'http://localhost:11434/v1' }] },
  vllm: { name: 'vLLM', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'http://localhost:8000/v1' }] },
  lmstudio: { name: 'LM Studio', endpoints: [{ labelKey: EP_PAYG, protocol: 'openai', baseURL: 'http://localhost:1234/v1' }] },
}

const selectedProvider = ref('custom')
const providerMenuRef = ref()
const providerMenuVisible = ref(false)

// Menu sections. Cloud vendors (Alibaba/Volcengine/Tencent/Baidu) count as
// model vendors — they all ship first-party models.
const PROVIDER_GROUPS: { key: string; nameKey: string; providers: string[] }[] = [
  {
    key: 'model',
    nameKey: 'settings.providerGroupModels',
    providers: ['openai', 'anthropic', 'google', 'xai', 'mistral', 'deepseek', 'zhipu', 'moonshot', 'minimax', 'mimo', 'longcat', 'alibaba', 'volcengine', 'byteplus', 'tencent', 'baidu'],
  },
  {
    key: 'local',
    nameKey: 'settings.providerGroupLocal',
    providers: ['ollama', 'vllm', 'lmstudio'],
  },
]

const providerGroups = computed(() => PROVIDER_GROUPS.map(group => ({
  ...group,
  entries: group.providers
    .map(key => ({ key, provider: PROVIDER_PRESETS[key] }))
    .filter(e => e.provider)
    // Alphabetical by brand name, so future additions keep the order.
    .sort((a, b) => a.provider.name.localeCompare(b.provider.name)),
})))

// Text shown on the trigger button for the current selection.
const providerDisplay = computed(() => {
  if (selectedProvider.value === 'custom') return t('settings.providerCustom')
  const sep = selectedProvider.value.indexOf(':')
  const provider = PROVIDER_PRESETS[selectedProvider.value.slice(0, sep)]
  const endpoint = provider?.endpoints[Number(selectedProvider.value.slice(sep + 1))]
  if (!provider || !endpoint) return t('settings.providerCustom')
  return `${provider.name} / ${endpoint.labelKey ? t(endpoint.labelKey) : endpoint.label}`
})

function onProviderSelect(index: string) {
  providerMenuRef.value?.close()
  selectedProvider.value = index
  // "custom" is the default — the user fills everything manually. Reset
  // protocol + baseURL so nothing leaks over from a previously picked preset.
  if (index === 'custom') {
    modelForm.protocol = '' as typeof modelForm.protocol
    modelForm.baseURL = ''
    return
  }
  const sep = index.indexOf(':')
  const provider = PROVIDER_PRESETS[index.slice(0, sep)]
  const endpoint = provider?.endpoints[Number(index.slice(sep + 1))]
  if (!endpoint) return
  modelForm.name = provider.name
  modelForm.protocol = endpoint.protocol
  modelForm.baseURL = endpoint.baseURL
  modelForm.model = ''
  // Clear any stale fetched model suggestions so "Fetch models" starts fresh.
  modelSuggestions.value = []
}

function openNewModelForm() {
  editingModel.value = null
  resetModelForm()
  selectedProvider.value = 'custom'
  testResult.value = null
  testError.value = ''
  showModelForm.value = true
}

function editModel(model: AIModelConfig) {
  editingModel.value = model
  modelSuggestions.value = []
  selectedProvider.value = 'custom'
  testResult.value = null
  testError.value = ''
  Object.assign(modelForm, { ...model })
  showModelForm.value = true
}

async function removeModelConfirm(model: AIModelConfig) {
  try {
    await ElMessageBox.confirm(
      t('settings.modelDeleteConfirm', { name: model.name }),
      t('common.delete')
    )
    settingsStore.removeModel(model.id)
  } catch {
    // user cancelled
  }
}

// Copy an existing model as a new entry (name suffixed with "copy"; a
// numeric suffix when that name is taken too). The duplicate starts
// inactive with a fresh id.
function duplicateModel(model: AIModelConfig) {
  const names = new Set(settingsStore.settings.ai.models.map(m => m.name))
  let name = `${model.name} copy`
  for (let i = 2; names.has(name); i++) name = `${model.name} copy ${i}`
  settingsStore.addModel({
    ...model,
    id: `model-${Date.now()}`,
    name,
  })
}

function saveModel() {
  if (editingModel.value) {
    settingsStore.updateModel(editingModel.value.id, { ...modelForm })
  } else {
    settingsStore.addModel({
      id: `model-${Date.now()}`,
      name: modelForm.name || 'Unnamed',
      baseURL: modelForm.baseURL,
      model: modelForm.model,
      apiKey: modelForm.apiKey,
      protocol: modelForm.protocol,
      userAgent: modelForm.userAgent || undefined,
      proxyId: modelForm.proxyId || undefined,
      sessionHeader: modelForm.sessionHeader || undefined
    })
  }
  showModelForm.value = false
  editingModel.value = null
  resetModelForm()
}

function resetModelForm() {
  modelForm.id = ''
  modelForm.name = ''
  modelForm.baseURL = ''
  modelForm.model = ''
  modelForm.apiKey = ''
  modelForm.protocol = 'anthropic'
  modelForm.userAgent = 'uniTerm'
  modelForm.proxyId = ''
  modelForm.sessionHeader = ''
  modelSuggestions.value = []
}

async function fetchModelList() {
  if (!modelForm.apiKey || !modelForm.baseURL) {
    msg.warning(t('settings.fetchModelsHint'))
    return
  }
  modelFetching.value = true
  modelSuggestions.value = []
  try {
    const models = await FetchModels(modelForm.apiKey, modelForm.baseURL, modelForm.protocol, modelForm.proxyId || '')
    modelSuggestions.value = (models || []).map((m: { id: string; display_name: string }) => ({
      value: m.id,
      label: m.display_name || m.id
    }))
    msg.success(t('settings.fetchModelsSuccess', { count: modelSuggestions.value.length }))
  } catch {
    msg.error(t('settings.fetchModelsFailed'))
  } finally {
    modelFetching.value = false
  }
}

async function testConnection() {
  if (!modelForm.apiKey || !modelForm.baseURL || !modelForm.model) {
    msg.warning(t('settings.testConnectionHint'))
    return
  }
  testingConnection.value = true
  testResult.value = null
  testError.value = ''
  try {
    const testMsg = JSON.stringify({
      model: modelForm.model,
      max_tokens: 10,
      system: 'Reply with exactly the word: ok',
      messages: [{ role: 'user', content: 'Say ok' }]
    })
    await ChatCompletion(
      modelForm.apiKey,
      modelForm.baseURL,
      modelForm.model,
      testMsg,
      modelForm.protocol,
      modelForm.userAgent || '',
      modelForm.proxyId || '',
      `uniterm-test-${Date.now()}`,
      modelForm.sessionHeader || ''
    )
    testResult.value = true
    msg.success(t('settings.testSuccess'))
  } catch (e: any) {
    testResult.value = false
    testError.value = backendErrorText(e)
    msg.error(t('settings.testFailed'))
  } finally {
    testingConnection.value = false
  }
}

// Quick-create proxy from the AI model form's "+" button (mirrors the
// connection form): after saving, select the new proxy immediately.
const modelProxyDialogVisible = ref(false)
function onModelProxySaved(p: Proxy) {
  modelForm.proxyId = p.id
}
</script>

<style scoped>
@import './_shared.css';

/* Model cards */
.model-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  padding: 0.75rem 1.125rem;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  transition: all 0.12s ease;
}

.model-card:hover {
  border-color: var(--border-hover);
}

.model-card.active {
  border-color: var(--accent);
  background: var(--accent-subtle);
}

.model-main {
  display: flex;
  flex-direction: column;
  gap: 0.125rem;
  flex: 1;
  min-width: 0;
}

.model-name {
  font-size: 0.8125rem;
  font-weight: 500;
  color: var(--text-primary);
}

.model-detail {
  font-size: 0.6875rem;
  font-family: var(--font-mono);
  color: var(--text-muted);
  margin-left: 1.5rem;
}

.model-actions {
  display: flex;
  gap: 0.25rem;
  flex-shrink: 0;
}

.model-fetch-row {
  display: flex;
  gap: 0.5rem;
  width: 100%;
}
.model-autocomplete {
  flex: 1;
}

/* ── Inline select + "+" rows (proxy quick-create, mirrors ConnectionForm) ── */
.inline-add-row {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  width: 100%;
}
.inline-add-btn {
  flex-shrink: 0;
  width: 2rem;
  height: 2rem;
  padding: 0;
}

/* Provider picker trigger — styled like the surrounding form inputs. */
.provider-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.375rem;
  width: 100%;
  padding: 0.375rem 0.625rem;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  font-size: 0.8125rem;
  /* <button> doesn't inherit the document font by default — match the inputs. */
  font-family: inherit;
  cursor: pointer;
  transition: border-color 0.1s ease;
}
.provider-trigger:hover {
  background: var(--bg-hover);
}

.provider-trigger-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Non-clickable section header inside the provider menu. */
.menu-group-label {
  padding: 0.375rem 0.875rem 0.125rem;
  font-size: 0.6875rem;
  color: var(--text-muted, var(--text-disabled));
  user-select: none;
  white-space: nowrap;
}
</style>