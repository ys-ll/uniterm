<template>
  <!-- Dock host for a sidebar view body. Chooses between rendering the slot
       in place (left sidebar), teleporting it into the bottom panel's body
       or into the right rail's body. The choice is a plain v-if (not
       <Teleport :disabled>, which has known anchor-corruption bugs when
       toggled) — flipping the dock remounts the slot, matching the old
       per-view v-if behavior.
       Each teleport waits for its host body to exist: Sidebar mounts before
       the dock hosts, and a Teleport whose target is missing at mount stays
       broken for its whole lifetime. No per-panel title bar is rendered
       here — every dock's strip carries the active panel's more menu. -->
  <Teleport v-if="location === 'bottom' && uiLayout.bottomPanelBodyReady" to="#bottom-panel-body">
    <slot />
  </Teleport>
  <Teleport v-else-if="location === 'right' && uiLayout.rightPanelBodyReady" to="#right-panel-body">
    <slot />
  </Teleport>
  <template v-else>
    <slot />
  </template>
</template>

<script setup lang="ts">
import { useUiLayoutStore } from '../stores/uiLayoutStore'

defineProps<{
  location: string
}>()

const uiLayout = useUiLayoutStore()
</script>
