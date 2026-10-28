<template>
  <div
    class="connection-item"
    :class="{
      indented,
      active: isSelected,
      'has-session': hasSession,
      'drop-before': dropBefore,
      'drop-after': dropAfter,
    }"
    :data-conn-id="conn.id"
    draggable="true"
    @dragstart="emit('dragstart', $event, conn)"
    @dragend="emit('dragend')"
    @dragover.prevent="emit('dragover', $event, conn)"
    @drop.prevent="emit('drop', $event, conn)"
    @click="emit('click', $event, conn)"
    @dblclick="emit('dblclick', conn)"
    @contextmenu.prevent="emit('contextmenu', $event, conn)"
  >
    <span class="conn-icon"><component :is="icon" :size="'0.875rem'" /></span>
    <div class="conn-details">
      <span class="name">{{ conn.name }}</span>
      <span class="conn-meta">
        <span class="host">{{ subtitle }}</span>
      </span>
    </div>
    <!-- Inside the favorites group the star is hover-only (everything
         here is favorited; a lit star would be redundant noise) -->
    <button
      v-if="showHostRowButtons && favoriteMode === 'lit'"
      class="conn-fav-btn lit"
      :title="t('sidebar.removeFromFavorites')"
      @click.stop="emit('toggle-favorite', conn)"
    >
      <Star :size="'0.75rem'" />
    </button>
    <button
      v-else-if="showHostRowButtons && favoriteMode === 'toggle'"
      class="conn-fav-btn"
      :class="{ on: isFavorite }"
      :title="isFavorite ? t('sidebar.removeFromFavorites') : t('sidebar.addToFavorites')"
      @click.stop="emit('toggle-favorite', conn)"
    >
      <Star :size="'0.75rem'" />
    </button>
    <button
      v-if="showHostRowButtons"
      class="conn-more-btn"
      @click.stop="emit('more', $event, conn)"
      :title="t('terminal.more')"
    >
      <MoreHorizontal :size="'0.875rem'" />
    </button>
  </div>
</template>

<script setup lang="ts">
import type { Component } from 'vue'
import { MoreHorizontal, Star } from '@lucide/vue'
import type { ConnectionConfig } from '../../types/session'
import { useI18n } from '../../i18n'

defineProps<{
  conn: ConnectionConfig
  icon: Component
  subtitle: string
  isSelected: boolean
  hasSession: boolean
  dropBefore: boolean
  dropAfter: boolean
  indented?: boolean
  showHostRowButtons: boolean
  /** 'toggle' = star toggles favorite state; 'lit' = always-lit (inside favorites group); 'none' = no star */
  favoriteMode?: 'toggle' | 'lit' | 'none'
  isFavorite?: boolean
}>()

const emit = defineEmits<{
  (e: 'dragstart', event: DragEvent, conn: ConnectionConfig): void
  (e: 'dragend'): void
  (e: 'dragover', event: DragEvent, conn: ConnectionConfig): void
  (e: 'drop', event: DragEvent, conn: ConnectionConfig): void
  (e: 'click', event: MouseEvent, conn: ConnectionConfig): void
  (e: 'dblclick', conn: ConnectionConfig): void
  (e: 'contextmenu', event: MouseEvent, conn: ConnectionConfig): void
  (e: 'more', event: MouseEvent, conn: ConnectionConfig): void
  (e: 'toggle-favorite', conn: ConnectionConfig): void
}>()

const { t } = useI18n()
</script>

<style scoped>
.connection-item {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  padding: 0.5rem 0.625rem;
  border-radius: var(--radius-sm);
  transition: all 0.12s ease;
  margin-bottom: 0.125rem;
  user-select: none;
  position: relative;
}

.connection-item.drop-before::before,
.connection-item.drop-after::after {
  content: '';
  position: absolute;
  left: 0.375rem;
  right: 0.375rem;
  height: 0.125rem;
  background: var(--accent);
  border-radius: var(--radius-sm);
  z-index: 2;
  pointer-events: none;
}
.connection-item.drop-before::before { top: -1px; }
.connection-item.drop-after::after { bottom: -1px; }

.connection-item.indented {
  padding-left: 1.5rem;
}

.connection-item:hover {
  background: var(--bg-hover);
}

.connection-item.active {
  background: var(--accent-subtle);
  box-shadow: inset 0 0 0 1px var(--accent);
}

.connection-item.active .name {
  color: var(--accent);
}

/* Connection has an open session → highlight its icon with the AI-lock amber */
.connection-item.has-session .conn-icon {
  color: var(--warning);
}

.conn-more-btn {
  display: none;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border: none;
  background: transparent;
  color: var(--text-muted);
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  margin-left: 0;
  padding: 0;
}
.connection-item:hover .conn-more-btn {
  display: flex;
}
.conn-more-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

/* Favorite toggle: always reserves its slot and sits at the right edge so
   rows align; revealed on hover and while favorited */
.conn-fav-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border: none;
  background: transparent;
  color: var(--text-muted);
  border-radius: var(--radius-sm);
  flex-shrink: 0;
  margin-left: auto;
  padding: 0;
  opacity: 0;
  pointer-events: none;
}
.connection-item:hover .conn-fav-btn,
.conn-fav-btn.on {
  opacity: 1;
  pointer-events: auto;
}
.conn-fav-btn.on {
  color: var(--warning);
}
/* Inside the favorites group: hover reveals the star already lit */
.conn-fav-btn.lit,
.conn-fav-btn.lit:hover {
  color: var(--warning);
}
.conn-fav-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}

.conn-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 1rem;
  flex-shrink: 0;
  color: var(--text-muted);
}

.conn-details {
  display: flex;
  flex-direction: column;
  flex: 1;
  gap: 0.125rem;
  min-width: 0;
}

.name {
  font-family: var(--font-ui);
  font-size: 0.75rem;
  font-weight: 500;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

.host {
  font-family: var(--font-ui);
  font-size: 0.6875rem;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  line-height: 1.4;
}

.conn-meta {
  display: flex;
  align-items: center;
  gap: 0.375rem;
  min-width: 0;
}
</style>