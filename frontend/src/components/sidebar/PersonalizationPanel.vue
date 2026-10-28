<template>
  <div class="personalization-panel">
    <div class="persist-section-title">{{ t('settings.app') }}</div>
    <div class="persist-section">
      <div class="persist-label">{{ t('settings.theme') }}</div>
      <el-select v-model="settingsStore.settings.theme" @change="settingsStore.save()">
        <el-option :label="t('settings.themeDark')" value="dark" />
        <el-option :label="t('settings.themeDeepBlue')" value="deep-blue" />
        <el-option :label="t('settings.themeLight')" value="light" />
        <el-option :label="t('settings.themeSystem')" value="system" />
      </el-select>
    </div>
    <div class="persist-section">
      <div class="persist-label">{{ t('settings.language') }}</div>
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
    <div class="persist-section-title">{{ t('settings.terminal') }}</div>
    <div class="persist-section">
      <div class="persist-label">{{ t('settings.colorScheme') }}</div>
      <div class="theme-select-row">
        <el-select v-model="settingsStore.settings.terminal.theme" @change="settingsStore.save()" popper-class="theme-select-popper">
          <el-option :label="t('settings.followAppTheme')" :value="FOLLOW_APP_THEME" />
          <el-option-group v-for="group in terminalThemeGroups" :key="group.label" :label="group.label">
            <el-option v-for="th in group.options" :key="th.value" :label="th.label" :value="th.value" />
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
    <div class="persist-section">
      <div class="persist-label">{{ t('settings.fontPrimary') }}</div>
      <el-select v-model="settingsStore.settings.terminal.fontFamily" @change="settingsStore.save()">
        <template #header>
          <div style="padding:0.25rem 0.75rem">
            <el-checkbox v-model="fontMonoOnly" @click.stop>{{ t('settings.fontMonoOnly') }}</el-checkbox>
          </div>
        </template>
        <el-option
          v-for="f in personalizationFontOptions"
          :key="f.value"
          :label="f.label"
          :value="f.value"
          :style="{ fontFamily: formatFontFamily(f.value) }"
        />
      </el-select>
    </div>
    <div class="persist-section">
      <div class="persist-label">{{ t('settings.fontFallback') }}</div>
      <el-select v-model="settingsStore.settings.terminal.fallbackFont" @change="settingsStore.save()">
        <template #header>
          <div style="padding:0.25rem 0.75rem">
            <el-checkbox v-model="fallbackFontMonoOnly" @click.stop>{{ t('settings.fontMonoOnly') }}</el-checkbox>
          </div>
        </template>
        <el-option value="" :label="t('settings.fontNone')" />
        <el-option
          v-for="f in personalizationFallbackFontOptions"
          :key="f.value"
          :label="f.label"
          :value="f.value"
          :style="{ fontFamily: formatFontFamily(f.value) }"
        />
      </el-select>
    </div>
    <div class="persist-section">
      <div class="persist-label">{{ t('settings.fontWeight') }}</div>
      <el-select v-model="settingsStore.settings.terminal.fontWeight" @change="settingsStore.save()">
        <el-option
          v-for="w in FONT_WEIGHT_OPTIONS"
          :key="w.value"
          :label="t(w.labelKey)"
          :value="w.value"
        />
      </el-select>
    </div>
    <div class="persist-section">
      <div class="persist-label">{{ t('settings.fontSize') }}</div>
      <el-input-number
        v-model="settingsStore.settings.terminal.fontSize"
        :min="8"
        :max="32"
        controls-position="right"
        @change="settingsStore.save()"
      />
    </div>
  </div>

  <CustomThemeEditor v-model="themeEditorVisible" :source-theme-id="themeEditorSourceId" />
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { Plus, Pencil } from '@lucide/vue'
import { useSettingsStore } from '../../stores/settingsStore'
import { useI18n } from '../../i18n'
import CustomThemeEditor from '../CustomThemeEditor.vue'
import { FONT_OPTIONS, FONT_WEIGHT_OPTIONS, LANGUAGE_OPTIONS, FOLLOW_APP_THEME } from '../../types/settings'
import { formatFontFamily, normalizeFontFamilyValue } from '../../utils/formatFontFamily'
import { useTerminalThemeOptions } from '../../composables/useTerminalThemeOptions'
import { GetAllFonts } from '../../../bindings/github.com/ys-ll/uniterm/app'

const { t } = useI18n()
const settingsStore = useSettingsStore()

// Single source: GetAllFonts returns every installed family with its mono
// flag (the bundled JetBrains Mono Variable is pinned first). Each select's
// embedded "monospace only" checkbox filters this client-side.
const allFonts = ref<{ Name: string; IsMono: boolean }[]>([])
const fontMonoOnly = ref(true)
const fallbackFontMonoOnly = ref(true)

const toFontOption = (name: string) => ({ label: name, value: name })
const personalizationFontOptions = computed(() =>
  allFonts.value.length > 0
    ? allFonts.value.filter(f => !fontMonoOnly.value || f.IsMono).map(f => toFontOption(f.Name))
    : FONT_OPTIONS
)
const personalizationFallbackFontOptions = computed(() =>
  allFonts.value.length > 0
    ? allFonts.value.filter(f => !fallbackFontMonoOnly.value || f.IsMono).map(f => toFontOption(f.Name))
    : FONT_OPTIONS
)

const { terminalThemeGroups, isCustomTheme } = useTerminalThemeOptions()

const themeEditorVisible = ref(false)
const themeEditorSourceId = ref<string | undefined>(undefined)
function openThemeEditor(sourceThemeId?: string) {
  themeEditorSourceId.value = sourceThemeId
  themeEditorVisible.value = true
}

// Load all installed fonts and migrate legacy fontFamily values.
onMounted(async () => {
  try {
    const fonts = await GetAllFonts()
    if (fonts && fonts.length > 0) {
      allFonts.value = fonts
      // Migrate a legacy full-stack fontFamily down to its bare family name so
      // it matches a dropdown option (and the new single-name default).
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
})
</script>

<style scoped>
.personalization-panel {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 1rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.persist-section-title {
  font-size: 0.75rem;
  font-weight: 600;
  font-family: var(--font-ui);
  color: var(--text-secondary);
  padding: 0 0 0.25rem 0;
  margin-bottom: -0.25rem;
}

.persist-section {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.persist-label {
  font-size: 0.6875rem;
  font-family: var(--font-ui);
  color: var(--text-muted);
  padding-left: 0.125rem;
}

.persist-section .el-select,
.persist-section .el-input-number {
  width: 100%;
}

.theme-select-row {
  display: flex;
  align-items: center;
  gap: 0.25rem;
}

.theme-select-row .el-select {
  flex: 1;
  min-width: 0;
}
</style>

<style>
.theme-select-popper .el-select-group__title {
  font-size: 0.625rem;
  color: var(--text-disabled);
  text-align: center;
  padding: 0.375rem 0.75rem 0.125rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}
.theme-select-popper .el-select-group__title::before,
.theme-select-popper .el-select-group__title::after {
  content: '';
  flex: 1;
  height: 1px;
  background: var(--border-subtle);
}
</style>