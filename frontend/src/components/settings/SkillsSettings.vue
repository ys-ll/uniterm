<template>
  <div class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.skills') }}</h2>
    <p class="section-desc">{{ t('settings.skillsDesc') }}</p>
    <SkillsManager />
    <h2 class="section-title">{{ t('settings.commands') }}</h2>
    <p class="section-desc">{{ t('settings.commandsDesc') }}</p>
    <CommandsManager />
  </div>
</template>

<script setup lang="ts">
import { useI18n } from '../../i18n'
import SkillsManager from '../SkillsManager.vue'
import CommandsManager from '../CommandsManager.vue'

defineProps<{ isNarrow: boolean }>()

const { t } = useI18n()
</script>

<style scoped>
@import './_shared.css';
</style>