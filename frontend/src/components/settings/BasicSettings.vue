<template>
  <div class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.appearance') }}</h2>
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

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.uiFontSize') }}</div>
          <div class="setting-desc">{{ t('settings.uiFontSizeDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-input-number
            v-model="settingsStore.settings.uiFontSize"
            :min="10"
            :max="18"
            :step="1"
            controls-position="right"
            @change="settingsStore.save()"
          />
        </div>
      </div>

      <!-- No OS window title bar on mobile -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.systemTitleBar') }}</div>
          <div class="setting-desc">{{ t('settings.systemTitleBarDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch
            :model-value="localStateStore.state.systemTitleBar"
            @update:model-value="(v: boolean) => onToggleSystemTitleBar(v)"
          />
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.bgEnable') }}</div>
          <div class="setting-desc">{{ t('settings.bgEnableDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch
            :model-value="localStateStore.state.backgroundEnabled"
            @update:model-value="(v: boolean) => localStateStore.update({ backgroundEnabled: v })"
          />
        </div>
      </div>

      <template v-if="localStateStore.state.backgroundEnabled">
        <div class="setting-card">
          <div class="setting-info">
            <div class="setting-title">{{ t('settings.bgImage') }}</div>
          </div>
          <div class="setting-control">
            <div class="bg-image-row">
              <div
                v-if="bgPreview"
                class="bg-thumb"
                :style="{ backgroundImage: `url('${bgPreview}')` }"
              ></div>
              <el-button size="small" @click="chooseBackground">{{ t('settings.bgChoose') }}</el-button>
              <el-button
                v-if="localStateStore.state.backgroundImage"
                size="small"
                @click="clearBackground"
              >{{ t('settings.bgClear') }}</el-button>
            </div>
          </div>
        </div>

        <div class="setting-card">
          <div class="setting-info">
            <div class="setting-title">{{ t('settings.bgOpacity') }}</div>
          </div>
          <div class="setting-control">
            <el-slider
              :model-value="localStateStore.state.backgroundOpacity"
              :min="0" :max="100"
              @update:model-value="(v: number) => localStateStore.update({ backgroundOpacity: v as number })"
            />
          </div>
        </div>

        <div class="setting-card">
          <div class="setting-info">
            <div class="setting-title">{{ t('settings.bgBlur') }}</div>
          </div>
          <div class="setting-control">
            <el-slider
              :model-value="localStateStore.state.backgroundBlur"
              :min="0" :max="20"
              @update:model-value="(v: number) => localStateStore.update({ backgroundBlur: v as number })"
            />
          </div>
        </div>

        <div class="setting-card">
          <div class="setting-info">
            <div class="setting-title">{{ t('settings.bgFit') }}</div>
          </div>
          <div class="setting-control">
            <el-select
              :model-value="localStateStore.state.backgroundFit"
              @update:model-value="(v: string) => localStateStore.update({ backgroundFit: v })"
            >
              <el-option :label="t('settings.bgFitCover')" value="cover" />
              <el-option :label="t('settings.bgFitContain')" value="contain" />
              <el-option :label="t('settings.bgFitCenter')" value="center" />
              <el-option :label="t('settings.bgFitTile')" value="tile" />
            </el-select>
          </div>
        </div>
      </template>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.tabCloseButton') }}</div>
          <div class="setting-desc">{{ t('settings.tabCloseButtonDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.tabCloseButton" @change="settingsStore.save()">
            <el-option :label="t('settings.tabCloseButtonLeft')" value="left" />
            <el-option :label="t('settings.tabCloseButtonRight')" value="right" />
          </el-select>
        </div>
      </div>

      <!-- Tab shortcut badges are already suppressed on mobile -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.showTabShortcutHints') }}</div>
          <div class="setting-desc">{{ t('settings.showTabShortcutHintsDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch v-model="settingsStore.settings.showTabShortcutHints" @change="settingsStore.save()" />
        </div>
      </div>

      <!-- Touch has no hover: 'rightclick' style would leave host rows
           with no visible menu affordance (long-press still works) -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.hostListMenuStyle') }}</div>
          <div class="setting-desc">{{ t('settings.hostListMenuStyleDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.hostListMenuStyle" @change="settingsStore.save()">
            <el-option :label="t('settings.hostListMenuStyleButton')" value="button" />
            <el-option :label="t('settings.hostListMenuStyleRightClick')" value="rightclick" />
          </el-select>
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.sidebarTabs') }}</div>
          <div class="setting-desc">{{ t('settings.sidebarTabsDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select
            v-model="visibleSidebarTabs"
            multiple
            collapse-tags
            :collapse-tags-limit="3"
            @change="onSidebarTabsChange"
          >
            <!-- "connections" is fixed and never rendered as an option;
                 the computed keeps it force-selected. -->
            <el-option
              v-for="tab in SIDEBAR_TAB_ORDER.filter(tab => tab.key !== 'connections')"
              :key="tab.key"
              :label="t(tab.labelKey)"
              :value="tab.key"
            />
          </el-select>
        </div>
      </div>

      <!-- Tab bar "+" default action (issue #1043): start page or local
           terminal. Also applies to the new-tab keyboard shortcut. -->
      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.newTabAction') }}</div>
          <div class="setting-desc">{{ t('settings.newTabActionDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.newTabAction" @change="settingsStore.save()">
            <el-option :label="t('settings.newTabActionStartPage')" value="startPage" />
            <el-option :label="t('settings.newTabActionLocalTerminal')" value="localTerminal" />
          </el-select>
        </div>
      </div>
    </div>

    <h2 class="section-title">{{ t('settings.interaction') }}</h2>
    <div class="settings-group">
      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.closeTabPrompt') }}</div>
        </div>
        <div class="setting-control">
          <el-switch v-model="settingsStore.settings.closeTabPrompt" @change="settingsStore.save()" />
        </div>
      </div>

      <!-- Mobile apps don't "quit" (back button = background) -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.closeAppPrompt') }}</div>
        </div>
        <div class="setting-control">
          <el-switch v-model="settingsStore.settings.closeAppPrompt" @change="settingsStore.save()" />
        </div>
      </div>

      <!-- Reopen last session's tabs at startup (issue #937) -->
      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.restoreTabsPrompt') }}</div>
          <div class="setting-desc">{{ t('settings.restoreTabsPromptDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch v-model="settingsStore.settings.restoreTabsPrompt" @change="settingsStore.save()" />
        </div>
      </div>

      <!-- No local CLI editors on mobile -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.externalEditor') }}</div>
          <div class="setting-desc">{{ t('settings.externalEditorDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select
            :model-value="localStateStore.state.externalEditor"
            class="editor-select"
            filterable
            allow-create
            default-first-option
            clearable
            placeholder='code -w'
            @update:model-value="(v: string) => localStateStore.update({ externalEditor: v as string })"
          >
            <el-option
              v-for="p in detectedEditors"
              :key="p.value"
              :label="p.label"
              :value="p.value"
            />
          </el-select>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { msg } from '../../services/message'
import { ListExternalEditors, OpenFileDialogFiltered, SetBackgroundImage, ClearBackgroundImage, GetBackgroundImage, RelaunchApp } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useSettingsStore } from '../../stores/settingsStore'
import { useLocalStateStore } from '../../stores/localStateStore'
import { useI18n } from '../../i18n'
import { ElMessageBox } from 'element-plus'
import { LANGUAGE_OPTIONS, SIDEBAR_TAB_ORDER, SIDEBAR_TAB_DEFAULTS } from '../../types/settings'
import { isMobilePlatform } from '../../utils/platform'

defineProps<{ isNarrow: boolean }>()

const settingsStore = useSettingsStore()
const localStateStore = useLocalStateStore()
const { t } = useI18n()
const isMobile = isMobilePlatform()

// Editors detected on this host (from the backend). The dropdown shows exactly
// these; empty means no editor was found installed, so nothing is listed.
const detectedEditors = ref<{ label: string; value: string }[]>([])

// ── Sidebar tab visibility ──
// Writable computed over AppSettings.sidebarTabs. "connections" never appears
// in the select (no option, no tag) — it is forced on at the data level.
const visibleSidebarTabs = computed<string[]>({
  get: () => SIDEBAR_TAB_ORDER
    .map(tab => tab.key)
    .filter(key => key !== 'connections')
    .filter(key => settingsStore.settings.sidebarTabs?.[key] ?? SIDEBAR_TAB_DEFAULTS[key] ?? true),
  set: (keys: string[]) => {
    const tabs = settingsStore.settings.sidebarTabs
    for (const tab of SIDEBAR_TAB_ORDER) {
      tabs[tab.key] = tab.key === 'connections' || keys.includes(tab.key)
    }
  },
})
function onSidebarTabsChange() {
  settingsStore.save()
}

// ── Background image ──
const bgPreview = ref('')

async function refreshBgPreview() {
  const name = localStateStore.state.backgroundImage
  bgPreview.value = name ? await GetBackgroundImage(name).catch(() => '') : ''
}

async function chooseBackground() {
  const path = await OpenFileDialogFiltered(
    t('settings.bgChoose'),
    'Images',
    '*.png;*.jpg;*.jpeg;*.webp'
  )
  if (!path) return
  try {
    const name = await SetBackgroundImage(path)
    await localStateStore.update({ backgroundImage: name, backgroundEnabled: true })
    await refreshBgPreview()
  } catch {
    msg.error(t('settings.bgUnsupported'))
  }
}

async function clearBackground() {
  await ClearBackgroundImage()
  await localStateStore.update({ backgroundImage: '' })
  bgPreview.value = ''
}

// The window frame is fixed at startup (Wails limitation) — changing it needs
// a restart. Persist the choice, then offer to quit so the user can reopen.
async function onToggleSystemTitleBar(v: boolean) {
  await localStateStore.update({ systemTitleBar: v })
  try {
    await ElMessageBox.confirm(
      t('settings.titleBarRestartMsg'),
      t('settings.titleBarRestartTitle'),
      { confirmButtonText: t('settings.restartNow'), cancelButtonText: t('conn.cancel'), type: 'warning' }
    )
  } catch {
    return
  }
  await RelaunchApp()
}

onMounted(async () => {
  // Populate the external-editor dropdown with editors actually installed on
  // this host; fall back to the curated presets if detection returns nothing.
  try {
    const editors = await ListExternalEditors()
    detectedEditors.value = (editors || []).map((e: { label: string; value: string }) => ({ label: e.label, value: e.value }))
  } catch {
    detectedEditors.value = []
  }
  try {
    await refreshBgPreview()
  } catch {
    // Ignore preview errors
  }
})

</script>

<style scoped>
@import './_shared.css';

.bg-image-row { display: flex; align-items: center; gap: 0.5rem; }
.bg-thumb {
  width: 4rem; height: 2.5rem; border-radius: var(--radius-sm);
  background-size: cover; background-position: center;
  border: 1px solid var(--border-subtle);
}
</style>