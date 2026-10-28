<template>
  <div v-show="searchVisible" class="terminal-search-bar">
    <button class="search-btn" :class="{ active: searchCaseSensitive }" :title="t('terminal.searchMatchCase')" @click="toggleSearchOption('caseSensitive')">
      <CaseSensitive :size="'0.875rem'" />
    </button>
    <button class="search-btn" :class="{ active: searchRegex }" :title="t('terminal.searchRegexp')" @click="toggleSearchOption('regex')">
      <Regex :size="'0.875rem'" />
    </button>
    <button class="search-btn" :class="{ active: searchWholeWord }" :title="t('terminal.searchWholeWord')" @click="toggleSearchOption('wholeWord')">
      <WholeWord :size="'0.875rem'" />
    </button>
    <input
      ref="searchInputRef"
      v-model="searchText"
      class="search-input"
      :placeholder="t('terminal.searchPlaceholder')"
      @input="onSearchInput"
      @keydown.enter.prevent="onSearchEnter"
      @keydown.escape="closeSearch"
      @blur="getSearchAddon()?.clearActiveDecoration()"
    />
    <span class="search-count" v-if="searchText">{{ searchResultIndex + 1 }}/{{ searchResultCount || 0 }}</span>
    <button class="search-btn" @click="onSearchPrev" :title="`${t('terminal.searchPrev')} (Enter)`">
      <ChevronUp :size="'0.875rem'" />
    </button>
    <button class="search-btn" @click="onSearchNext" :title="`${t('terminal.searchNext')} (Shift+Enter)`">
      <ChevronDown :size="'0.875rem'" />
    </button>
    <button class="search-btn" @click="closeSearch" :title="t('terminal.searchClose')">
      <X :size="'0.875rem'" />
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, nextTick } from 'vue'
import type { ISearchOptions } from '@xterm/addon-search'
import { getManagedTerminal } from '../../services/terminalManager'
import { useI18n } from '../../i18n'
import { CaseSensitive, ChevronUp, ChevronDown, Regex, WholeWord, X } from '@lucide/vue'

const props = defineProps<{
  sessionId: string | null | undefined
}>()

const { t } = useI18n()

const searchInputRef = ref<HTMLInputElement>()
const searchVisible = ref(false)
const searchText = ref('')
const searchResultIndex = ref(0)
const searchResultCount = ref(0)
// Toggle states persist across search open/close within the session
const searchCaseSensitive = ref(false)
const searchRegex = ref(false)
const searchWholeWord = ref(false)

const searchDecoOptions = {
  matchBackground: '#515c6e',
  matchBorder: '#22d3ee',
  matchOverviewRuler: '#22d3ee',
  activeMatchBackground: '#22d3ee44',
  activeMatchBorder: '#22d3ee',
  activeMatchColorOverviewRuler: '#22d3ee',
}

function getSearchAddon() {
  return props.sessionId ? getManagedTerminal(props.sessionId)?.searchAddon : undefined
}

function openSearch() {
  searchVisible.value = true
  nextTick(() => {
    searchInputRef.value?.focus()
    if (searchText.value) {
      searchInputRef.value?.select()
      onSearchInput()
    }
  })
}

function closeSearch() {
  searchVisible.value = false
  searchText.value = ''
  // Toggle states (case / regex / whole-word) persist across opens
  searchResultIndex.value = 0
  searchResultCount.value = 0
  getSearchAddon()?.clearDecorations()
}

function searchOpts(): ISearchOptions {
  return {
    caseSensitive: searchCaseSensitive.value,
    regex: searchRegex.value,
    wholeWord: searchWholeWord.value,
    decorations: searchDecoOptions,
  }
}

function invalidRegex() {
  // The addon constructs RegExp directly without try/catch (SearchEngine.ts:289),
  // so guard here; an invalid pattern must not throw.
  if (!searchRegex.value) return false
  try {
    new RegExp(searchText.value)
    return false
  } catch {
    return true
  }
}

function clearSearchMatches() {
  searchResultIndex.value = 0
  searchResultCount.value = 0
  getSearchAddon()?.clearDecorations()
}

function onSearchInput() {
  if (!searchText.value || invalidRegex()) {
    clearSearchMatches()
    return
  }
  // Anchor the match at the bottom of the buffer (closest to the prompt);
  // Enter continues upward, Shift+Enter downward.
  getSearchAddon()?.findPrevious(searchText.value, searchOpts())
}

function onSearchEnter(e: KeyboardEvent) {
  if (e.shiftKey) {
    onSearchNext()
  } else {
    onSearchPrev()
  }
}

function onSearchNext() {
  if (!searchText.value || invalidRegex()) return
  getSearchAddon()?.findNext(searchText.value, searchOpts())
}

function onSearchPrev() {
  if (!searchText.value || invalidRegex()) return
  getSearchAddon()?.findPrevious(searchText.value, searchOpts())
}

function toggleSearchOption(key: 'caseSensitive' | 'regex' | 'wholeWord') {
  if (key === 'caseSensitive') searchCaseSensitive.value = !searchCaseSensitive.value
  else if (key === 'regex') searchRegex.value = !searchRegex.value
  else searchWholeWord.value = !searchWholeWord.value
  // Re-anchor at the bottom with the new options
  onSearchInput()
}

// Set up search results listener from shared SearchAddon. Runs after the parent's
// onMounted (which acquires the managed terminal), so getManagedTerminal is ready.
onMounted(() => {
  const managed = getManagedTerminal(props.sessionId || '')
  if (managed) {
    managed.searchAddon.onDidChangeResults((e) => {
      searchResultIndex.value = e.resultIndex
      searchResultCount.value = e.resultCount
    })
  }
})

defineExpose({
  openSearch,
  closeSearch,
})
</script>

<style scoped>
.terminal-search-bar {
  position: absolute;
  top: 0.5rem;
  right: 0.5rem;
  display: flex;
  align-items: center;
  gap: 0.25rem;
  /* 跟应用主题走（与 Zmodem 面板、终端建议弹窗等悬浮控件一致）；
     保留 88% 不透明度，让 blur 透出一点终端内容。 */
  background: color-mix(in srgb, var(--bg-surface) 88%, transparent);
  backdrop-filter: blur(0.5rem);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: 0.25rem 0.375rem;
  z-index: 50;
}
.search-input {
  width: 10rem;
  background: transparent;
  border: none;
  outline: none;
  color: var(--text-primary);
  font-family: var(--font-ui);
  font-size: 0.75rem;
  padding: 0.125rem 0.25rem;
}
.search-input::placeholder {
  color: var(--text-muted);
}
.search-count {
  font-family: var(--font-mono);
  font-size: 0.6875rem;
  color: var(--text-muted);
  white-space: nowrap;
  min-width: 2rem;
  text-align: center;
}
.search-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.375rem;
  height: 1.375rem;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  transition: all 0.15s;
}
.search-btn:hover {
  background: var(--bg-hover);
  color: var(--text-primary);
}
.search-btn.active {
  color: var(--accent);
  background: var(--accent-subtle);
}
</style>