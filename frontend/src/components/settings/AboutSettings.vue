<template>
  <div class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.about') }}</h2>
    <div class="about-content">
      <div class="about-appname">uniTerm</div>
      <p class="about-desc">{{ t('settings.aboutDesc') }}</p>
      <div class="about-version">
        {{ t('settings.version') }}: {{ updateCheck.updateInfo?.current || '...' }}
      </div>
      <div class="about-links">
        <a href="#" class="about-link" @click.prevent="Browser.OpenURL('https://uniterm.net')">
          <Globe :size="'0.875rem'" class="about-link-icon" />
          {{ t('settings.homepage') }}
        </a>
        <a href="#" class="about-link" @click.prevent="Browser.OpenURL(locale === 'zh-CN' ? 'https://uniterm.net/guide/zh/introduction' : 'https://uniterm.net/guide/en/introduction')">
          <BookOpen :size="'0.875rem'" class="about-link-icon" />
          {{ t('settings.userManual') }}
        </a>
        <a href="#" class="about-link" @click.prevent="Browser.OpenURL('https://github.com/ys-ll/uniterm')">
          <GitHubLogo class="about-link-icon" />
          GitHub
        </a>
        <a href="#" class="about-link" @click.prevent="Browser.OpenURL('https://gitee.com/ys-l/uniterm')">
          <GiteeLogo class="about-link-icon" />
          Gitee
        </a>
      </div>
      <!-- In-app update download is desktop-only; mobile updates via APK -->
      <div v-if="!isMobile" class="about-update-actions">
        <el-button

          :loading="updateCheck.checking"
          @click="handleCheckUpdate"
        >
          {{ updateCheck.checking ? t('settings.checking') : t('settings.checkUpdate') }}
        </el-button>
      </div>
      <div v-if="!isMobile" class="about-auto-check">
        <el-checkbox
          v-model="updateCheck.autoCheck"
        >
          {{ t('settings.autoCheckUpdate') }}
        </el-checkbox>
      </div>
      <div v-if="!isMobile" class="about-update-source">
        <span class="about-update-source-label">{{ t('settings.updateSource') }}</span>
        <el-select
          v-model="updateCheck.source"
          size="small"
          class="about-update-source-select"
          @change="(v: 'auto' | 'github' | 'gitee') => updateCheck.setSource(v)"
        >
          <el-option :label="t('settings.updateSourceAuto')" value="auto" />
          <el-option :label="t('settings.updateSourceGithub')" value="github" />
          <el-option :label="t('settings.updateSourceGitee')" value="gitee" />
        </el-select>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { Globe, BookOpen } from '@lucide/vue'
import { useUpdateCheck } from '../../composables/useUpdateCheck'
import { useI18n, locale } from '../../i18n'
import { isMobilePlatform } from '../../utils/platform'
import { Browser } from '@wailsio/runtime'
import GitHubLogo from '../icons/GitHubLogo.vue'
import GiteeLogo from '../icons/GiteeLogo.vue'

defineProps<{ isNarrow: boolean }>()

const updateCheck = useUpdateCheck()
const { t } = useI18n()
const isMobile = isMobilePlatform()

async function handleCheckUpdate() {
  await updateCheck.checkForUpdate(true)
}
</script>

<style scoped>
@import './_shared.css';

.about-content {
  text-align: left;
  padding: 1.25rem 0;
}
.about-appname {
  font-size: 1.75rem;
  font-weight: 700;
  color: var(--text-primary);
  margin-bottom: 0.75rem;
}
.about-desc {
  font-size: 0.875rem;
  color: var(--text-secondary);
  margin: 0 0 1.5rem 0;
  line-height: 1.6;
  max-width: 25rem;
}
.about-version {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-family: var(--font-mono);
}
.about-links {
  display: flex;
  flex-wrap: wrap;
  gap: 1rem;
  margin-top: 0.75rem;
}
.about-link {
  display: inline-flex;
  align-items: center;
  gap: 0.375rem;
  font-size: 0.75rem;
  color: var(--accent);
  text-decoration: none;
  transition: opacity 0.12s ease;
}
.about-link:hover {
  opacity: 0.8;
  text-decoration: underline;
}
.about-link-icon {
  width: 0.875rem;
  height: 0.875rem;
  flex-shrink: 0;
  opacity: 0.7;
}

.about-update-actions {
  margin-top: 1.25rem;
}
.about-auto-check {
  margin-top: 0.75rem;
  font-size: 0.8125rem;
  font-family: var(--font-ui);
}
.about-update-source {
  margin-top: 0.625rem;
  display: flex;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.8125rem;
  font-family: var(--font-ui);
}
.about-update-source-select {
  width: 11.875rem;
}
</style>