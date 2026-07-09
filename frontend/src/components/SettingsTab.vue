<template>
  <div class="settings-tab">
    <div class="settings-sidebar">
      <div
        v-for="cat in categories"
        :key="cat.key"
        class="settings-category"
        :class="{ active: settingsStore.activeCategory === cat.key }"
        @click="settingsStore.activeCategory = cat.key"
      >
        <el-icon class="category-icon"><component :is="cat.icon" /></el-icon>
        <span class="category-label">{{ cat.label }}</span>
      </div>
    </div>

    <div class="settings-panel">
      <!-- 基础设置 -->
      <div v-if="settingsStore.activeCategory === 'basic'" class="settings-section">
        <h2 class="section-title">{{ t('settings.basic') }}</h2>

        <div class="settings-group">
          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.theme') }}</div>
              <div class="setting-desc">{{ t('settings.themeDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-select v-model="settingsStore.settings.theme" @change="settingsStore.save()">
                <el-option :label="t('settings.themeDark')" value="dark" />
                <el-option :label="t('settings.themeDeepBlue')" value="deep-blue" />
                <el-option :label="t('settings.themeLight')" value="light" />
                <el-option :label="t('settings.themeSystem')" value="system" />
              </el-select>
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.language') }}</div>
              <div class="setting-desc">{{ t('settings.languageDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-select :model-value="settingsStore.settings.language" @change="settingsStore.updateLanguage">
                <el-option
                  v-for="lang in LANGUAGE_OPTIONS"
                  :key="lang.value"
                  :label="lang.native"
                  :value="lang.value"
                />
                <el-option :label="t('settings.langSystem')" value="system" />
              </el-select>
            </div>
          </div>
        </div>
      </div>

      <!-- 终端配置 -->
      <div v-if="settingsStore.activeCategory === 'terminal'" class="settings-section">
        <h2 class="section-title">{{ t('settings.terminal') }}</h2>

        <div class="settings-group">
          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.colorScheme') }}</div>
              <div class="setting-desc">{{ t('settings.colorSchemeDesc') }}</div>
            </div>
            <div class="setting-control">
              <div class="theme-select-row">
                <el-select v-model="settingsStore.settings.terminal.theme" @change="settingsStore.save()" popper-class="theme-select-popper">
                  <el-option-group
                    v-for="group in terminalThemeGroups"
                    :key="group.label"
                    :label="group.label"
                  >
                    <el-option
                      v-for="th in group.options"
                      :key="th.value"
                      :label="th.label"
                      :value="th.value"
                    />
                  </el-option-group>
                </el-select>
                <button class="btn btn-ghost btn-icon btn-sm" :title="t('theme.newTitle')" @click="openThemeEditor()">
                  <Plus :size="14" />
                </button>
                <button
                  v-if="isCustomTheme(settingsStore.settings.terminal.theme)"
                  class="btn btn-ghost btn-icon btn-sm"
                  :title="t('theme.editTitle')"
                  @click="openThemeEditor(settingsStore.settings.terminal.theme)"
                >
                  <Pencil :size="14" />
                </button>
              </div>
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.font') }}</div>
              <div class="setting-desc">{{ t('settings.fontDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-select v-model="settingsStore.settings.terminal.fontFamily" @change="settingsStore.save()">
                <el-option
                  v-for="f in fontOptions"
                  :key="f.value"
                  :label="f.label"
                  :value="f.value"
                  :style="{ fontFamily: f.value }"
                />
              </el-select>
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.fontSize') }}</div>
              <div class="setting-desc">{{ t('settings.fontSizeDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-input-number
                v-model="settingsStore.settings.terminal.fontSize"
                :min="8"
                :max="32"
               
                @change="settingsStore.save()"
              />
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.selectionAction') }}</div>
              <div class="setting-desc">{{ t('settings.selectionActionDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-select v-model="settingsStore.settings.terminal.selectionAction" @change="settingsStore.save()">
                <el-option :label="t('settings.selectionNone')" value="none" />
                <el-option :label="t('settings.selectionCopy')" value="copy" />
              </el-select>
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.rightClick') }}</div>
              <div class="setting-desc">{{ t('settings.rightClickDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-select v-model="settingsStore.settings.terminal.rightClickAction" @change="settingsStore.save()">
                <el-option :label="t('settings.rightClickMenu')" value="menu" />
                <el-option :label="t('settings.rightClickPaste')" value="paste" />
              </el-select>
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.middleClick') }}</div>
              <div class="setting-desc">{{ t('settings.middleClickDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-select v-model="settingsStore.settings.terminal.middleClickAction" @change="settingsStore.save()">
                <el-option :label="t('settings.middleClickNone')" value="none" />
                <el-option :label="t('settings.middleClickPaste')" value="paste" />
              </el-select>
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.maxHistory') }}</div>
              <div class="setting-desc">{{ t('settings.maxHistoryDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-input-number
                v-model="settingsStore.settings.terminal.maxHistoryLines"
                :min="100"
                :max="50000"
                :step="100"
               
                @change="settingsStore.save()"
              />
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.smartCompletion') }}</div>
              <div class="setting-desc">{{ t('settings.smartCompletionDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-switch v-model="settingsStore.settings.terminal.smartCompletion" @change="settingsStore.save()" />
            </div>
          </div>

          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.highlight') }}</div>
              <div class="setting-desc">{{ t('settings.highlightDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-switch :model-value="settingsStore.settings.terminal.highlightEnabled ?? true" @update:model-value="(v: boolean) => { settingsStore.settings.terminal.highlightEnabled = v; settingsStore.save() }" />
            </div>
          </div>

        </div>
      </div>

      <!-- Sync settings -->
      <div v-if="settingsStore.activeCategory === 'sync'" class="settings-section sync-settings">
        <h2 class="section-title">{{ t('settings.sync') }}</h2>
        <p class="section-desc">{{ t('settings.syncDesc') }}</p>

        <!-- Empty state: no repo configured -->
        <div v-if="!syncStore.config.repoUrl" class="sync-card">
          <div class="sync-card-header">{{ t('settings.syncRepoCard') }}</div>
          <div class="sync-card-body empty-state">
            <p class="empty-text">{{ t('settings.syncEmptyDesc') }}</p>
            <el-button type="primary" @click="syncStore.showAddRepo = true">
              {{ t('settings.syncAddRepo') }}
            </el-button>
          </div>
        </div>

        <!-- Configured state -->
        <template v-else>
          <!-- Repo config card -->
          <div class="sync-card">
            <div class="sync-card-header">
              <span>{{ t('settings.syncRepoCard') }}</span>
              <el-button text @click="openEditRepo">{{ t('settings.syncEdit') }}</el-button>
            </div>
            <div class="sync-card-body">
              <div class="repo-info">
                <div class="repo-info-row">
                  <span class="repo-label">{{ t('settings.syncRepoUrl') }}</span>
                  <span class="repo-value">{{ syncStore.config.repoUrl }}</span>
                </div>
                <div class="repo-info-row">
                  <span class="repo-label">{{ t('settings.syncUsername') }}</span>
                  <span class="repo-value">{{ syncStore.config.username }}</span>
                </div>
              </div>
              <div class="repo-actions">
                <el-button @click="syncStore.showChangePassword = true">{{ t('settings.syncChangePassword') }}</el-button>
                <el-button @click="syncStore.showDeleteRepo = true">{{ t('settings.syncDeleteRepo') }}</el-button>
              </div>
            </div>
          </div>

          <!-- Sync card -->
          <div class="sync-card">
            <div class="sync-card-header">{{ t('settings.syncSyncCard') }}</div>
            <div class="sync-card-body">
              <div class="sync-status">
                <div class="sync-status-row">
                  <span class="sync-label">{{ t('settings.syncLastSync') }}</span>
                  <span class="sync-value">{{ syncStore.formatSyncTime() }}</span>
                  <span v-if="syncStore.config.lastSyncStatus === 'success'" class="sync-tag success">{{ t('settings.syncStatusSuccess') }}</span>
                  <span v-else-if="syncStore.config.lastSyncStatus === 'failed'" class="sync-tag failed">{{ t('settings.syncStatusFailed') }}</span>
                </div>
                <div v-if="syncStore.config.lastSyncStatus === 'failed' && syncStore.config.lastSyncError" class="sync-status-row sync-error">
                  <span class="sync-label">{{ t('settings.syncReason') }}</span>
                  <span class="sync-value error-text">{{ syncStore.config.lastSyncError }}</span>
                </div>
              </div>
              <div class="sync-actions-row">
                <el-button
                  type="primary"
                  :loading="syncStore.syncing"
                  @click="handleSyncNow"
                >
                  {{ t('settings.syncNow') }}
                </el-button>
              </div>
              <div class="sync-auto-row">
                <span class="sync-auto-label">{{ t('settings.syncAuto') }}</span>
                <span class="sync-auto-desc">{{ t('settings.syncAutoDesc') }}</span>
                <el-switch v-model="syncStore.config.autoSync" @change="handleAutoSyncToggle" />
              </div>
            </div>
          </div>
        </template>
      </div>

      <!-- 关于 -->
      <div v-if="settingsStore.activeCategory === 'about'" class="settings-section">
        <h2 class="section-title">{{ t('settings.about') }}</h2>
        <div class="about-content">
          <div class="about-appname">uniTerm</div>
          <p class="about-desc">{{ t('settings.aboutDesc') }}</p>
          <div class="about-version">
            {{ t('settings.version') }}: {{ updateCheck.updateInfo?.current || '...' }}
          </div>
          <div class="about-links">
            <a href="#" class="about-link" @click.prevent="BrowserOpenURL('https://uniterm.net')">
              <Globe :size="14" class="about-link-icon" />
              {{ t('settings.homepage') }}
            </a>
            <a href="#" class="about-link" @click.prevent="BrowserOpenURL(locale === 'zh-CN' ? 'https://uniterm.net/guide/zh/introduction' : 'https://uniterm.net/guide/en/introduction')">
              <BookOpen :size="14" class="about-link-icon" />
              {{ t('settings.userManual') }}
            </a>
            <a href="#" class="about-link" @click.prevent="BrowserOpenURL('https://github.com/ys-ll/uniterm')">
              <svg class="about-link-icon" viewBox="0 0 16 16" width="14" height="14" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27s1.36.09 2 .27c1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8z"/></svg>
              GitHub
            </a>
            <a href="#" class="about-link" @click.prevent="BrowserOpenURL('https://gitee.com/ys-l/uniterm')">
              <svg class="about-link-icon" viewBox="0 0 24 24" width="14" height="14" fill="currentColor"><path d="M11.984 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.016 0zm6.09 5.333c.328 0 .593.266.592.593v1.482a.594.594 0 0 1-.593.592H9.777c-.982 0-1.778.796-1.778 1.778v5.63c0 .327.266.592.593.592h5.63c.982 0 1.778-.796 1.778-1.778v-.296a.593.593 0 0 0-.592-.593h-4.15a.592.592 0 0 1-.592-.592v-1.482a.593.593 0 0 1 .593-.592h6.815c.327 0 .593.265.593.592v3.408a4 4 0 0 1-4 4H5.926a.593.593 0 0 1-.593-.593V9.778a4.444 4.444 0 0 1 4.445-4.444h8.296z"/></svg>
              Gitee
            </a>
          </div>
          <div class="about-update-actions">
            <el-button
             
              :loading="updateCheck.checking"
              @click="handleCheckUpdate"
            >
              {{ updateCheck.checking ? t('settings.checking') : t('settings.checkUpdate') }}
            </el-button>
          </div>
          <div class="about-auto-check">
            <el-checkbox
              v-model="updateCheck.autoCheck"
            >
              {{ t('settings.autoCheckUpdate') }}
            </el-checkbox>
          </div>
        </div>
      </div>

      <!-- 快捷键设置 -->
      <div v-if="settingsStore.activeCategory === 'keyboard'" class="settings-section">
        <h2 class="section-title">{{ t('shortcut.title') }}</h2>
        <table class="kb-table">
          <thead>
            <tr>
              <th>{{ t('shortcut.colFunction') }}</th>
              <th>{{ t('shortcut.colBinding') }}</th>
              <th style="width:190px;">{{ t('shortcut.colActions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="action in (Object.keys(SHORTCUT_LABELS) as ShortcutAction[])"
              :key="action"
            >
              <td>{{ t(SHORTCUT_LABELS[action] || action) }}</td>
              <td><kbd class="kb-key">{{ bindingDisplay(action) }}</kbd></td>
              <td class="kb-actions">
                <el-button
                 
                  :type="rebindingAction === action ? 'warning' : 'default'"
                  @click="startRebind(action)"
                >
                  {{ rebindingAction === action ? t('shortcut.pressKey') : t('shortcut.edit') }}
                </el-button>
                <el-button
                  v-if="rebindingAction === action"
                 
                  @click="stopRebind()"
                >
                  {{ t('shortcut.cancel') }}
                </el-button>
                <el-button
                  v-if="rebindingAction === action"
                 
                  type="danger"
                  @click="clearBinding(action)"
                >
                  {{ t('shortcut.clear') }}
                </el-button>
                <el-button
                  v-if="!isDefaultBinding(action) && rebindingAction !== action"
                 
                  type="danger"
                  @click="resetBinding(action)"
                >
                  {{ t('shortcut.reset') }}
                </el-button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- AI助理设置 -->
      <div v-if="settingsStore.activeCategory === 'ai'" class="settings-section">
        <h2 class="section-title">{{ t('settings.ai') }}</h2>

        <div class="settings-group">
          <div class="setting-card">
            <div class="setting-info">
              <div class="setting-title">{{ t('settings.modelList') }}</div>
              <div class="setting-desc">{{ t('settings.modelListDesc') }}</div>
            </div>
            <div class="setting-control">
              <el-button @click="showModelForm = true"><Plus :size="14" /> {{ t('settings.addModel') }}</el-button>
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
                :label="model.id"
                @change="settingsStore.setActiveModel(model.id)"
              >
                <span class="model-name">{{ model.name }}</span>
              </el-radio>
              <span class="model-detail">{{ model.model }} @ {{ model.baseURL }}</span>
            </div>
            <div class="model-actions">
              <el-button link @click="editModel(model)">
                <el-icon><Pencil :size="14" /></el-icon>
              </el-button>
              <el-button link type="danger" @click="settingsStore.removeModel(model.id)">
                <el-icon><Trash2 :size="14" /></el-icon>
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Model Form Dialog -->
    <el-dialog v-model="showModelForm" :title="editingModel ? t('settings.editModel') : t('settings.newModel')" width="400px">
      <el-form label-width="80px">
        <el-form-item :label="t('settings.modelName')">
          <el-input v-model="modelForm.name" />
        </el-form-item>
        <el-form-item :label="t('settings.modelProtocol')">
          <el-select v-model="modelForm.protocol" style="width: 100%">
            <el-option label="Anthropic" value="anthropic" />
            <el-option label="OpenAI" value="openai" />
          </el-select>
        </el-form-item>
        <el-form-item :label="t('settings.modelBaseURL')">
          <el-input v-model="modelForm.baseURL" :placeholder="modelForm.protocol === 'openai' ? 'https://api.openai.com/v1' : 'https://api.anthropic.com/v1'" />
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
                :label="s.value"
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
          <span v-if="testResult != null" :class="testResult ? 'test-ok' : 'test-fail'" style="margin-left: 8px; font-size: 13px;">
            {{ testResult ? t('settings.testSuccess') : t('settings.testFailed') }}
          </span>
          <span v-if="testError" style="margin-left: 8px; font-size: 12px; color: var(--error); word-break: break-all;">{{ testError }}</span>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showModelForm = false">{{ t('settings.cancel') }}</el-button>
        <el-button type="primary" @click="saveModel">{{ t('settings.save') }}</el-button>
      </template>
    </el-dialog>

    <!-- Sync dialogs -->
    <AddRepoDialog />
    <EditRepoDialog />
    <ChangePasswordDialog />
    <DeleteRepoDialog />
    <CustomThemeEditor v-model="themeEditorVisible" :source-theme-id="themeEditorSourceId" />
  </div>
</template>

<script setup lang="ts">
import { ref, reactive, watch, computed, onMounted } from 'vue'
import { Settings, Monitor, MessageCircleMore, Info, RefreshCw, Pencil, Trash2, Globe, Keyboard, Plus, BookOpen } from '@lucide/vue'
import { msg } from '../services/message'
import { FetchModels, ChatCompletion, GetSystemFonts } from '../../wailsjs/go/main/App'
import { useSettingsStore } from '../stores/settingsStore'
import { useSyncStore } from '../stores/syncStore'
import { useUpdateCheck } from '../composables/useUpdateCheck'
import { useI18n, locale } from '../i18n'
import { BrowserOpenURL } from '../../wailsjs/runtime'
import { FONT_OPTIONS, LANGUAGE_OPTIONS, DEFAULT_KEYBOARD, SHORTCUT_LABELS, USER_AGENT_PRESETS } from '../types/settings'
import { formatFontFamily } from '../utils/formatFontFamily'
import type { AIModelConfig, ShortcutAction, KeyBinding, KeyboardSettings } from '../types/settings'
import { useTerminalThemeOptions } from '../composables/useTerminalThemeOptions'
import { uninstallGlobalListener, installGlobalListener } from '../composables/useKeyboardShortcuts'
import AddRepoDialog from './AddRepoDialog.vue'
import EditRepoDialog from './EditRepoDialog.vue'
import ChangePasswordDialog from './ChangePasswordDialog.vue'
import DeleteRepoDialog from './DeleteRepoDialog.vue'
import CustomThemeEditor from './CustomThemeEditor.vue'

const settingsStore = useSettingsStore()
const syncStore = useSyncStore()
const updateCheck = useUpdateCheck()
const { t } = useI18n()

function openEditRepo() {
  syncStore.showEditRepo = true
}

async function handleSyncNow() {
  const result = await syncStore.doSync()
  if (!result) {
    msg.error(syncStore.lastResult || t('settings.syncFailed'))
    return
  }
  if (result.direction === 3) {
    return  // conflict — handled by SyncConflictDialog
  }
  msg.success(result.message || t('settings.syncSuccess'))
}

async function handleAutoSyncToggle() {
  try {
    await syncStore.saveConfig()
  } catch (e) {
    console.error('Failed to save auto sync toggle:', e)
  }
}

async function handleCheckUpdate() {
  await updateCheck.checkForUpdate(true)
}

syncStore.loadConfig()

// ── System fonts ──
const systemFonts = ref<{ label: string; value: string }[]>([])
const fontOptions = computed(() => {
  if (systemFonts.value.length > 0) {
    return systemFonts.value
  }
  return FONT_OPTIONS
})

const { terminalThemeGroups, isCustomTheme } = useTerminalThemeOptions()

const themeEditorVisible = ref(false)
const themeEditorSourceId = ref<string | undefined>(undefined)
function openThemeEditor(sourceThemeId?: string) {
  themeEditorSourceId.value = sourceThemeId
  themeEditorVisible.value = true
}

onMounted(async () => {
  try {
    const fonts = await GetSystemFonts()
    if (fonts && fonts.length > 0) {
      systemFonts.value = fonts.map(f => ({ label: f, value: formatFontFamily(f) }))
    }
  } catch {
    // Fall back to FONT_OPTIONS
  }
})

watch(() => settingsStore.openCategory, (cat) => {
  if (cat && (cat === 'basic' || cat === 'terminal' || cat === 'ai' || cat === 'sync' || cat === 'about' || cat === 'keyboard')) {
    settingsStore.activeCategory = cat
    settingsStore.openCategory = null
  }
})

// ── Keyboard rebinding ──
const rebindingAction = ref<ShortcutAction | null>(null)

function bindingDisplay(action: ShortcutAction): string {
  const b = settingsStore.settings.keyboard[action]
  if (!b) return ''
  const parts: string[] = []
  if (b.ctrl) parts.push('Ctrl')
  if (b.shift) parts.push('Shift')
  if (b.alt) parts.push('Alt')
  parts.push(b.key)
  return parts.join('+')
}

function isDefaultBinding(action: ShortcutAction): boolean {
  const current = settingsStore.settings.keyboard[action]
  const def = DEFAULT_KEYBOARD[action]
  if (!current || !def) return true
  return current.ctrl === def.ctrl && current.shift === def.shift
    && current.alt === def.alt && current.key === def.key
}

function resetBinding(action: ShortcutAction) {
  settingsStore.settings.keyboard = {
    ...settingsStore.settings.keyboard,
    [action]: { ...DEFAULT_KEYBOARD[action] }
  }
  settingsStore.save()
}

let rebindListenerActive = false

function startRebind(action: ShortcutAction) {
  rebindingAction.value = action
  uninstallGlobalListener()
  if (!rebindListenerActive) {
    rebindListenerActive = true
    document.addEventListener('keydown', onRebindKeydown, true)
    window.addEventListener('blur', onRebindBlur)
  }
}

function stopRebind() {
  if (rebindListenerActive) {
    rebindListenerActive = false
    document.removeEventListener('keydown', onRebindKeydown, true)
    window.removeEventListener('blur', onRebindBlur)
  }
  rebindingAction.value = null
  installGlobalListener()
}

function clearBinding(action: ShortcutAction) {
  settingsStore.settings.keyboard = {
    ...settingsStore.settings.keyboard,
    [action]: { ctrl: false, shift: false, alt: false, key: '' }
  }
  settingsStore.save()
  stopRebind()
}

function onRebindKeydown(e: KeyboardEvent) {
  if (!rebindingAction.value) return stopRebind()
  e.preventDefault()
  e.stopPropagation()
  const key = e.key
  if (key === 'Escape') return stopRebind()
  if (key === 'Control' || key === 'Shift' || key === 'Alt' || key === 'Meta') return

  const binding: KeyBinding = {
    ctrl: e.ctrlKey || e.metaKey,
    shift: e.shiftKey,
    alt: e.altKey,
    key: key.toLowerCase(),
  }

  // Check for conflicts and clear them
  const conflictAction = findConflict(binding)
  const kb = { ...settingsStore.settings.keyboard }
  kb[rebindingAction.value] = binding
  if (conflictAction) {
    kb[conflictAction] = { ctrl: false, shift: false, alt: false, key: '' }
  }
  settingsStore.settings.keyboard = kb as KeyboardSettings
  settingsStore.save()
  stopRebind()
}

function findConflict(binding: KeyBinding): ShortcutAction | null {
  const targetKey = `${binding.ctrl ? 'ctrl+' : ''}${binding.shift ? 'shift+' : ''}${binding.alt ? 'alt+' : ''}${binding.key.toLowerCase()}`
  const kb = settingsStore.settings.keyboard
  for (const [action, b] of Object.entries(kb) as [ShortcutAction, KeyBinding][]) {
    if (action === rebindingAction.value) continue
    if (!b.key) continue
    const key = `${b.ctrl ? 'ctrl+' : ''}${b.shift ? 'shift+' : ''}${b.alt ? 'alt+' : ''}${b.key.toLowerCase()}`
    if (key === targetKey) return action
  }
  return null
}

function onRebindBlur() {
  stopRebind()
}

const categories = computed(() => {
  // Explicitly read language to ensure reactivity tracking
  void settingsStore.settings.language
  const cats = [
    { key: 'basic', label: t('settings.basic'), icon: Settings },
    { key: 'terminal', label: t('settings.terminal'), icon: Monitor },
    { key: 'keyboard', label: t('shortcut.title'), icon: Keyboard },
    { key: 'ai', label: t('settings.ai'), icon: MessageCircleMore },
    { key: 'sync', label: t('settings.sync'), icon: RefreshCw },
    { key: 'about', label: t('settings.about'), icon: Info },
  ]
  return cats
})

const showModelForm = ref(false)
const modelSuggestions = ref<Array<{ value: string }>>([])
// Always surface the currently-set model as an option so el-select renders it
// when editing an existing model (before any fetch) — el-select won't display a
// bound value that has no matching option, and allow-create only creates
// options for values typed during the session, not a pre-set v-model.
const modelSelectOptions = computed(() => {
  const opts = modelSuggestions.value.slice()
  const cur = modelForm.model?.trim()
  if (cur && !opts.some(o => o.value === cur)) {
    opts.unshift({ value: cur })
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
  protocol: 'anthropic' as 'anthropic' | 'openai',
  userAgent: 'uniTerm' as string,
})

function editModel(model: AIModelConfig) {
  editingModel.value = model
  modelSuggestions.value = []
  Object.assign(modelForm, { ...model })
  showModelForm.value = true
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
      userAgent: modelForm.userAgent || undefined
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
    const models = await FetchModels(modelForm.apiKey, modelForm.baseURL)
    modelSuggestions.value = (models || []).map(m => ({
      value: m.display_name || m.id
    }))
    msg.success(t('settings.fetchModelsSuccess', { count: modelSuggestions.value.length }))
  } catch (e: any) {
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
      modelForm.userAgent || ''
    )
    testResult.value = true
    msg.success(t('settings.testSuccess'))
  } catch (e: any) {
    testResult.value = false
    testError.value = e?.message || String(e)
    msg.error(t('settings.testFailed'))
  } finally {
    testingConnection.value = false
  }
}

function getShellLabel(path: string): string {
  const lower = path.toLowerCase()
  if (lower.includes('pwsh')) return 'PowerShell'
  if (lower.includes('powershell')) return 'Windows PowerShell'
  if (lower.includes('bash')) return 'Git Bash'
  if (lower.includes('cmd')) return 'Command Prompt'
  return path.split(/[\\/]/).pop() || path
}
</script>

<style scoped>
.settings-tab {
  display: flex;
  width: 100%;
  min-width: 680px;
  max-width: 960px;
  height: 100%;
  margin: 0 auto;
  background: var(--bg-base);
  color: var(--text-primary);
}

.settings-sidebar {
  width: 180px;
  flex-shrink: 0;
  padding: 16px 0;
  border-right: 1px solid var(--border-hover);
}

.settings-category {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  margin: 0 8px;
  font-size: 13px;
  font-family: var(--font-ui);
  cursor: pointer;
  user-select: none;
  color: var(--text-secondary);
  border-radius: var(--radius-sm);
  transition: all 0.12s ease;
  border-left: 3px solid transparent;
}

.settings-category:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.settings-category.active {
  background: var(--accent-subtle);
  color: var(--accent);
  border-left-color: var(--accent);
}

.category-icon {
  font-size: 16px;
}

.category-label {
  line-height: 1;
}

.settings-panel {
  flex: 1;
  padding: 24px 32px;
  overflow-y: auto;
  min-width: 0;
}

.section-title {
  font-size: 18px;
  font-weight: 600;
  font-family: var(--font-ui);
  margin: 0 0 20px 0;
  color: var(--text-primary);
}

.settings-group {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.setting-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 18px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  transition: all 0.12s ease;
}

.setting-card:hover {
  border-color: var(--border-hover);
}

.setting-info {
  flex: 1;
  min-width: 0;
}

.setting-title {
  font-size: 13px;
  font-weight: 500;
  font-family: var(--font-ui);
  color: var(--text-primary);
  margin-bottom: 2px;
}

.setting-desc {
  font-size: 11px;
  font-family: var(--font-ui);
  color: var(--text-muted);
  line-height: 1.4;
}

.setting-control {
  flex-shrink: 0;
  min-width: 210px;
}

.setting-control .el-select,
.setting-control .el-input-number {
  width: 100%;
}

.theme-select-row {
  display: flex;
  align-items: center;
  gap: 4px;
  width: 100%;
}

.theme-select-row .el-select {
  flex: 1;
  min-width: 0;
  width: auto;
}

/* Model cards */
.model-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 18px;
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
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
  gap: 2px;
  flex: 1;
  min-width: 0;
}

.model-name {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.model-detail {
  font-size: 11px;
  font-family: var(--font-mono);
  color: var(--text-muted);
  margin-left: 24px;
}

.model-actions {
  display: flex;
  gap: 4px;
  flex-shrink: 0;
}

.about-content {
  text-align: left;
  padding: 20px 0;
}
.about-appname {
  font-size: 28px;
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: 12px;
}
.about-desc {
  font-size: 14px;
  color: var(--text-secondary);
  margin: 0 0 24px 0;
  line-height: 1.6;
  max-width: 400px;
}
.about-version {
  font-size: 12px;
  color: var(--text-muted);
  font-family: var(--font-mono);
}
.about-links {
  display: flex;
  gap: 16px;
  margin-top: 12px;
}
.about-link {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--accent);
  text-decoration: none;
  transition: opacity 0.12s ease;
}
.about-link:hover {
  opacity: 0.8;
  text-decoration: underline;
}
.about-link-icon {
  flex-shrink: 0;
  opacity: 0.7;
}

.section-desc {
  font-size: 13px;
  color: var(--text-secondary);
  margin: 0;
  line-height: 1.5;
}

.sync-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  margin-bottom: 16px;
  overflow: hidden;
}

.sync-card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  font-weight: 600;
  font-family: var(--font-ui);
  color: var(--text-primary);
  padding: 8px 12px 8px 18px;
  background: var(--bg-hover);
  border-bottom: 1px solid var(--border-subtle);
}

.sync-card-body {
  padding: 16px 18px;
}

.sync-card-body.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 28px 18px;
}

.empty-text {
  font-size: 13px;
  color: var(--text-muted);
  margin: 0;
}

/* Repo config */
.repo-info {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 12px;
}

.repo-info-row {
  display: flex;
  gap: 12px;
  font-size: 13px;
}

.repo-label {
  color: var(--text-muted);
  min-width: 70px;
  flex-shrink: 0;
}

.repo-value {
  color: var(--text-primary);
  font-family: var(--font-mono);
  word-break: break-all;
}

.repo-warning {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 10px 14px;
  background: var(--el-color-warning-light-9);
  border: 1px solid var(--el-color-warning-light-5);
  border-radius: 6px;
  margin-bottom: 14px;
  color: var(--el-color-warning-dark-2);
  font-size: 12px;
  line-height: 1.5;
}

.repo-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}

.repo-actions-left {
  display: flex;
  gap: 8px;
}

/* Sync status */
.sync-status {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-bottom: 14px;
}

.sync-status-row {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
}

.sync-label {
  color: var(--text-muted);
  min-width: 70px;
  flex-shrink: 0;
}

.sync-value {
  color: var(--text-primary);
}

.sync-tag {
  font-size: 11px;
  padding: 2px 8px;
  border-radius: 10px;
  font-weight: 500;
}

.sync-tag.success {
  background: var(--el-color-success-light-9);
  color: var(--el-color-success-dark-2);
}

.sync-tag.failed {
  background: var(--el-color-danger-light-9);
  color: var(--el-color-danger-dark-2);
}

.sync-error {
  align-items: flex-start;
}

.error-text {
  color: var(--el-color-danger);
}

.sync-actions-row {
  margin-bottom: 14px;
}

.sync-auto-row {
  display: flex;
  align-items: center;
  gap: 10px;
  padding-top: 14px;
  border-top: 1px solid var(--border-subtle);
}

.sync-auto-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--text-primary);
}

.sync-auto-desc {
  font-size: 12px;
  color: var(--text-muted);
  flex: 1;
}

.model-fetch-row {
  display: flex;
  gap: 8px;
  width: 100%;
}
.model-autocomplete {
  flex: 1;
}

.about-update-actions {
  margin-top: 20px;
}
.about-auto-check {
  margin-top: 12px;
  font-size: 13px;
  font-family: var(--font-ui);
}

.kb-key {
  display: inline-block;
  padding: 2px 8px;
  background: var(--bg-overlay);
  border: 1px solid var(--border-subtle);
  border-radius: 4px;
  font-family: var(--font-mono);
  font-size: 12px;
  color: var(--text-primary);
}

.kb-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.kb-table th, .kb-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid var(--border-subtle);
}

.kb-table th {
  color: var(--text-muted);
  font-weight: 500;
  font-size: 12px;
  text-transform: uppercase;
}

.kb-table tbody tr:hover {
  background: var(--bg-hover);
}

.kb-actions {
  display: flex;
  gap: 6px;
}
</style>
