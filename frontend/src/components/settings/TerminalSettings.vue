<template>
  <div class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.appearance') }}</h2>

    <div class="settings-group">
      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.colorScheme') }}</div>
          <div class="setting-desc">{{ t('settings.colorSchemeDesc') }}</div>
        </div>
        <div class="setting-control">
          <div class="theme-select-row">
            <el-select v-model="settingsStore.settings.terminal.theme" @change="settingsStore.save()" popper-class="theme-select-popper">
              <el-option :label="t('settings.followAppTheme')" :value="FOLLOW_APP_THEME" />
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
              <Plus :size="'0.875rem'" />
            </button>
            <button
              v-if="isCustomTheme(settingsStore.settings.terminal.theme)"
              class="btn btn-ghost btn-icon btn-sm"
              :title="t('theme.editTitle')"
              @click="openThemeEditor(settingsStore.settings.terminal.theme)"
            >
              <Pencil :size="'0.875rem'" />
            </button>
          </div>
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.fontPrimary') }}</div>
          <div class="setting-desc">{{ t('settings.fontDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.fontFamily" @change="settingsStore.save()">
            <template #header>
              <div style="padding:0.25rem 0.75rem">
                <el-checkbox v-model="firstMonoOnly" @click.stop>{{ t('settings.fontMonoOnly') }}</el-checkbox>
              </div>
            </template>
            <el-option
              v-for="f in fontOptions"
              :key="f.value"
              :label="f.label"
              :value="f.value"
              :style="{ fontFamily: formatFontFamily(f.value) }"
            />
          </el-select>
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.fontFallback') }}</div>
          <div class="setting-desc">{{ t('settings.fontFallbackDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.fallbackFont" @change="settingsStore.save()">
            <template #header>
              <div style="padding:0.25rem 0.75rem">
                <el-checkbox v-model="secondMonoOnly" @click.stop>{{ t('settings.fontMonoOnly') }}</el-checkbox>
              </div>
            </template>
            <el-option value="" :label="t('settings.fontNone')" />
            <el-option
              v-for="f in secondFontOptions"
              :key="f.value"
              :label="f.label"
              :value="f.value"
              :style="{ fontFamily: formatFontFamily(f.value) }"
            />
          </el-select>
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.fontWeight') }}</div>
          <div class="setting-desc">{{ t('settings.fontWeightDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.fontWeight" @change="settingsStore.save()">
            <el-option
              v-for="w in FONT_WEIGHT_OPTIONS"
              :key="w.value"
              :label="t(w.labelKey)"
              :value="w.value"
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
            controls-position="right"
            @change="settingsStore.save()"
          />
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.cursorBlink') }}</div>
          <div class="setting-desc">{{ t('settings.cursorBlinkDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch :model-value="settingsStore.settings.terminal.cursorBlink ?? true" @update:model-value="(v: boolean) => { settingsStore.settings.terminal.cursorBlink = v; settingsStore.save() }" />
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.cursorStyle') }}</div>
          <div class="setting-desc">{{ t('settings.cursorStyleDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.cursorStyle" @change="settingsStore.save()">
            <el-option v-for="opt in cursorStyleOptions" :key="opt.value" :label="t(opt.labelKey)" :value="opt.value" />
          </el-select>
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.terminalContrast') }}</div>
          <div class="setting-desc">{{ t('settings.terminalContrastDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.minimumContrast" @change="settingsStore.save()">
            <el-option :label="t('settings.contrastOff')" :value="1" />
            <el-option :label="t('settings.contrastStandard')" :value="4.5" />
            <el-option :label="t('settings.contrastHigh')" :value="7" />
          </el-select>
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

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.showLineNumbers') }}</div>
          <div class="setting-desc">{{ t('settings.showLineNumbersDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch :model-value="settingsStore.settings.terminal.showLineNumbers ?? false" @update:model-value="(v: boolean) => { settingsStore.settings.terminal.showLineNumbers = v; settingsStore.save() }" />
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.showTimestamps') }}</div>
          <div class="setting-desc">{{ t('settings.showTimestampsDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch :model-value="settingsStore.settings.terminal.showTimestamps ?? false" @update:model-value="(v: boolean) => { settingsStore.settings.terminal.showTimestamps = v; settingsStore.save() }" />
        </div>
      </div>

      <div class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.timestampFormat') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.timestampFormat" @change="settingsStore.save()">
            <el-option v-for="opt in TIMESTAMP_FORMATS" :key="opt.value" :label="`${t(opt.labelKey)} (${opt.value})`" :value="opt.value" />
          </el-select>
        </div>
      </div>
    </div>

    <h2 class="section-title">{{ t('settings.interaction') }}</h2>
    <div class="settings-group">
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
          <div class="setting-title">{{ t('settings.wordSeparator') }}</div>
          <div class="setting-desc">{{ t('settings.wordSeparatorDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-input
            v-model="settingsStore.settings.terminal.wordSeparator"
            :placeholder="defaultWordSeparator"
            clearable
            @change="settingsStore.save()"
          />
        </div>
      </div>

      <!-- No mouse buttons on touch screens -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.rightClick') }}</div>
          <div class="setting-desc">{{ t('settings.rightClickDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.rightClickAction" @change="settingsStore.save()">
            <el-option :label="t('settings.rightClickMenu')" value="menu" />
            <el-option :label="t('settings.rightClickPaste')" value="paste" />
            <el-option :label="t('settings.selectionNone')" value="none" />
          </el-select>
        </div>
      </div>

      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.middleClick') }}</div>
          <div class="setting-desc">{{ t('settings.middleClickDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.terminal.middleClickAction" @change="settingsStore.save()">
            <el-option :label="t('settings.rightClickPaste')" value="paste" />
            <el-option :label="t('settings.rightClickMenu')" value="menu" />
            <el-option :label="t('settings.selectionNone')" value="none" />
          </el-select>
        </div>
      </div>

      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.ctrlWheelZoom') }}</div>
          <div class="setting-desc">{{ t('settings.ctrlWheelZoomDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch :model-value="settingsStore.settings.terminal.ctrlWheelZoom ?? true" @update:model-value="(v: boolean) => { settingsStore.settings.terminal.ctrlWheelZoom = v; settingsStore.save() }" />
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
          <div class="setting-title">{{ t('settings.aiTranscription') }}</div>
          <div class="setting-desc">{{ t('settings.aiTranscriptionDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-switch :model-value="settingsStore.settings.terminal.aiTranscription ?? true" @update:model-value="(v: boolean) => { settingsStore.settings.terminal.aiTranscription = v; settingsStore.save() }" />
        </div>
      </div>
    </div>

    <h2 class="section-title">{{ t('settings.session') }}</h2>
    <div class="settings-group">
      <!-- No user shells exist on mobile (same reason the start page hides
           the local terminal button) -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.defaultLocalShell') }}</div>
          <div class="setting-desc">{{ t('settings.defaultLocalShellDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-select v-model="settingsStore.settings.defaultLocalShell" @change="settingsStore.save()">
            <el-option
              v-for="sh in settingsStore.availableShells"
              :key="sh"
              :label="getShellLabel(sh)"
              :value="sh"
            />
          </el-select>
        </div>
      </div>

      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.gitBashPath') }}</div>
          <div class="setting-desc">{{ t('settings.gitBashPathDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-input
            v-model="settingsStore.settings.gitBashPath"
            clearable
            placeholder="C:\Users\you\scoop\apps\git\current"
            @change="onGitBashPathChange"
          />
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
            controls-position="right"
            @change="settingsStore.save()"
          />
        </div>
      </div>

      <!-- Local dirs are not user-selectable under mobile scoped storage -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.zmodemDownloadDir') }}</div>
          <div class="setting-desc">{{ t('settings.zmodemDownloadDirDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-input
            v-model="settingsStore.settings.terminal.zmodemDownloadDir"
            :placeholder="t('settings.zmodemDownloadDirPlaceholder')"
            class="dir-input"
            @change="settingsStore.save()"
            clearable
          >
            <template #append>
              <el-tooltip :content="t('settings.browse')" placement="top">
                <el-button :aria-label="t('settings.browse')" @click="pickZmodemDownloadDir">
                  <el-icon><FolderOpen :size="'1rem'" /></el-icon>
                </el-button>
              </el-tooltip>
            </template>
          </el-input>
        </div>
      </div>

      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.sessionLogDir') }}</div>
          <div class="setting-desc">{{ t('settings.sessionLogDirDesc', { path: defaultLogDir }) }}</div>
        </div>
        <div class="setting-control">
          <el-input
            v-model="settingsStore.settings.terminal.sessionLogDir"
            :placeholder="defaultLogDir"
            class="dir-input"
            @change="settingsStore.save()"
            clearable
          >
            <template #append>
              <el-tooltip :content="t('settings.browse')" placement="top">
                <el-button :aria-label="t('settings.browse')" @click="pickLogDir">
                  <el-icon><FolderOpen :size="'1rem'" /></el-icon>
                </el-button>
              </el-tooltip>
            </template>
          </el-input>
        </div>
      </div>

      <!-- Session logs go to the app-private dir on mobile; the filename
           pattern is not user-serviceable there -->
      <div v-if="!isMobile" class="setting-card">
        <div class="setting-info">
          <div class="setting-title">{{ t('settings.sessionLogFilename') }}</div>
          <div class="setting-desc">{{ t('settings.sessionLogFilenameDesc') }}</div>
        </div>
        <div class="setting-control">
          <el-input
            v-model="settingsStore.settings.terminal.sessionLogFilename"
            placeholder="%S_%H_%M%D_%h%m.log"
            @change="settingsStore.save()"
            clearable
          />
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Plus, Pencil, FolderOpen } from '@lucide/vue'
import { msg } from '../../services/message'
import { GetAllFonts, GetDefaultSessionLogDir, OpenDirectoryDialog } from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useSettingsStore } from '../../stores/settingsStore'
import { useI18n } from '../../i18n'
import { FONT_OPTIONS, FONT_WEIGHT_OPTIONS, FOLLOW_APP_THEME, CURSOR_STYLES, TIMESTAMP_FORMATS } from '../../types/settings'
import { formatFontFamily, normalizeFontFamilyValue } from '../../utils/formatFontFamily'
import { getShellLabel as getShellLabelBase } from '../../utils/shellLabel'
import { isMobilePlatform } from '../../utils/platform'
import { useTerminalThemeOptions } from '../../composables/useTerminalThemeOptions'
import { useThemeEditor } from '../../composables/useSettingsForm'

defineProps<{ isNarrow: boolean }>()

const settingsStore = useSettingsStore()
const { t } = useI18n()
const { openThemeEditor } = useThemeEditor()
const isMobile = isMobilePlatform()

// ── Terminal fonts ──
// Single source: GetAllFonts returns every installed family with its mono
// flag (the bundled JetBrains Mono Variable is pinned first). The per-select
// "monospace only" checkbox (inside each dropdown) filters this client-side.
const allFonts = ref<{ Name: string; IsMono: boolean }[]>([])
const firstMonoOnly = ref(true)
const secondMonoOnly = ref(true)

const toFontOption = (name: string) => ({ label: name, value: name })
const fontOptions = computed(() =>
  allFonts.value.length > 0
    ? allFonts.value.filter(f => !firstMonoOnly.value || f.IsMono).map(f => toFontOption(f.Name))
    : FONT_OPTIONS
)
const secondFontOptions = computed(() =>
  allFonts.value.length > 0
    ? allFonts.value.filter(f => !secondMonoOnly.value || f.IsMono).map(f => toFontOption(f.Name))
    : FONT_OPTIONS
)

const { terminalThemeGroups, isCustomTheme } = useTerminalThemeOptions()

const cursorStyleOptions = CURSOR_STYLES

// Session log directory: shown as placeholder when the setting is
// empty. Value comes from backend on mount and reflects the OS
// default plus any current override (so if the user cleared their
// override, the placeholder shows the fallback path).
const defaultLogDir = ref('')

// Default word separator for xterm's double-click selection; shown in
// the settings input as a placeholder so the user can see what to
// reset to.
const defaultWordSeparator = '\\ :;~`!@#$%^&*()=+|[]{}\'",<>?'

async function onGitBashPathChange() {
  await settingsStore.save()
  await settingsStore.refreshAvailableShells()
}

async function pickLogDir() {
  try {
    const chosen = await OpenDirectoryDialog()
    if (chosen) {
      settingsStore.settings.terminal.sessionLogDir = chosen
      await settingsStore.save()
    }
  } catch (e: any) {
    msg.error(String(e?.message ?? e))
  }
}

async function pickZmodemDownloadDir() {
  try {
    const chosen = await OpenDirectoryDialog()
    if (chosen) {
      settingsStore.settings.terminal.zmodemDownloadDir = chosen
      await settingsStore.save()
    }
  } catch (e: any) {
    msg.error(String(e?.message ?? e))
  }
}

function getShellLabel(path: string): string {
  return getShellLabelBase(path, 'Local')
}

onMounted(async () => {
  try {
    const fonts = await GetAllFonts()
    if (fonts && fonts.length > 0) {
      allFonts.value = fonts
      // Migrate a legacy full-stack fontFamily down to its bare family name so
      // it matches a dropdown option (and the new single-name default) — the
      // old stored value was a raw CSS stack this picker can't show cleanly.
      const names = new Set(allFonts.value.map(f => f.Name))
      const cur = settingsStore.settings.terminal.fontFamily
      const normalized = normalizeFontFamilyValue(cur, names)
      if (normalized !== cur) {
        settingsStore.settings.terminal.fontFamily = normalized
        settingsStore.save()
      }
    }
  } catch {
    // Fall back to FONT_OPTIONS
  }
  try {
    defaultLogDir.value = await GetDefaultSessionLogDir()
  } catch {
    defaultLogDir.value = ''
  }
})
</script>

<style scoped>
@import './_shared.css';
</style>