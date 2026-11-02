<template>
  <!-- Dock host for a sidebar view body. Chooses between rendering the slot
       in place (left sidebar) and teleporting it into the bottom panel's
       body. The choice is a plain v-if (not <Teleport :disabled>, which has
       known anchor-corruption bugs when toggled) — flipping the dock
       remounts the slot, matching the old per-view v-if behavior.
       The teleport also waits for the bottom panel body to exist: Sidebar
       mounts before BottomPanel, and a Teleport whose target is missing at
       mount stays broken for its whole lifetime.
       The #title slot (per-panel title bar) renders only in the sidebar
       branch — the bottom panel's tab strip already labels the view. -->
  <Teleport v-if="docked && uiLayout.bottomPanelBodyReady" to="#bottom-panel-body">
    <slot />
  </Teleport>
  <template v-else>
    <slot name="title" />
    <slot />
  </template>
</template>

<script setup lang="ts">
import { useUiLayoutStore } from '../stores/uiLayoutStore'

defineProps<{
  docked: boolean
}>()

const uiLayout = useUiLayoutStore()
</script>
