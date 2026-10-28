<template>
  <div class="settings-section" :class="{ narrow: isNarrow }">
    <p v-if="isMac" class="kb-hint kb-modifier-note">{{ t('shortcut.modifierNote') }}</p>
    <template v-for="cat in shortcutCategories" :key="cat.key">
      <h2 class="section-title">{{ t(cat.label) }}</h2>
      <table class="kb-table">
        <thead>
          <tr>
            <th>{{ t('shortcut.colFunction') }}</th>
            <th>{{ t('shortcut.colBinding') }}</th>
            <th>{{ t('shortcut.colActions') }}</th>
          </tr>
        </thead>
        <tbody>
          <template v-if="cat.key === 'tabs'">
            <tr>
              <td>{{ t('shortcut.switchToTabByIndex') }}</td>
              <td><kbd class="kb-key">{{ tabSwitchBindingDisplay() }}</kbd></td>
              <td class="kb-actions">
                <el-button
                  :type="rebindingTabSwitch ? 'warning' : 'default'"
                  @click="startRebindTabSwitch()"
                >
                  {{ rebindingTabSwitch ? t('shortcut.pressModifier') : t('shortcut.edit') }}
                </el-button>
                <el-button
                  v-if="rebindingTabSwitch"
                  @click="stopRebind()"
                >
                  {{ t('shortcut.cancel') }}
                </el-button>
                <el-button
                  v-if="rebindingTabSwitch"
                  type="danger"
                  @click="clearTabSwitchModifier()"
                >
                  {{ t('shortcut.clear') }}
                </el-button>
                <el-button
                  v-if="!isTabSwitchModifierUnset() && !rebindingTabSwitch"
                  type="danger"
                  @click="resetTabSwitchModifier()"
                >
                  {{ t('shortcut.reset') }}
                </el-button>
              </td>
            </tr>
            <tr>
              <td>{{ t('shortcut.switchToPanelByIndex') }}</td>
              <td><kbd class="kb-key">{{ panelSwitchBindingDisplay() }}</kbd></td>
              <td class="kb-actions">
                <el-button
                  :type="rebindingPanelSwitch ? 'warning' : 'default'"
                  @click="startRebindPanelSwitch()"
                >
                  {{ rebindingPanelSwitch ? t('shortcut.pressModifier') : t('shortcut.edit') }}
                </el-button>
                <el-button
                  v-if="rebindingPanelSwitch"
                  @click="stopRebind()"
                >
                  {{ t('shortcut.cancel') }}
                </el-button>
                <el-button
                  v-if="rebindingPanelSwitch"
                  type="danger"
                  @click="clearPanelSwitchModifier()"
                >
                  {{ t('shortcut.clear') }}
                </el-button>
                <el-button
                  v-if="!isPanelSwitchModifierUnset() && !rebindingPanelSwitch"
                  type="danger"
                  @click="resetPanelSwitchModifier()"
                >
                  {{ t('shortcut.reset') }}
                </el-button>
              </td>
            </tr>
          </template>
          <tr
            v-for="action in cat.actions"
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
    </template>

    <!-- 系统全局热键（托盘等，系统级生效） -->
    <h2 class="section-title">{{ t('shortcut.globalHotkeys') }}</h2>
    <table class="kb-table">
      <thead>
        <tr>
          <th>{{ t('shortcut.colFunction') }}</th>
          <th>{{ t('shortcut.colBinding') }}</th>
          <th>{{ t('shortcut.colActions') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>{{ t('shortcut.trayHotkeyRow') }}</td>
          <td><kbd class="kb-key">{{ trayHotkeyDisplay() }}</kbd></td>
          <td class="kb-actions">
            <el-button
              :type="rebindingTrayHotkey ? 'warning' : 'default'"
              @click="startRebindTrayHotkey()"
            >
              {{ rebindingTrayHotkey ? t('shortcut.pressKey') : t('shortcut.edit') }}
            </el-button>
            <el-button
              v-if="rebindingTrayHotkey"
              @click="stopRebind()"
            >
              {{ t('shortcut.cancel') }}
            </el-button>
            <el-button
              v-if="rebindingTrayHotkey"
              type="danger"
              @click="clearTrayHotkey()"
            >
              {{ t('shortcut.clear') }}
            </el-button>
            <el-button
              v-if="!isTrayHotkeyDefault() && !rebindingTrayHotkey"
              type="danger"
              @click="resetTrayHotkey()"
            >
              {{ t('shortcut.reset') }}
            </el-button>
          </td>
        </tr>
      </tbody>
    </table>
  </div>
</template>

<script setup lang="ts">
import { ref, onUnmounted, onDeactivated } from 'vue'
import { msg } from '../../services/message'
import { useSettingsStore } from '../../stores/settingsStore'
import { useI18n } from '../../i18n'
import { DEFAULT_KEYBOARD, SHORTCUT_LABELS } from '../../types/settings'
import type { ShortcutAction, KeyBinding, KeyboardSettings } from '../../types/settings'
import { uninstallGlobalListener, installGlobalListener, formatKeyBinding, digitModifierCollides, digitModifierFlagsEqual, TAB_DEFAULT_FLAGS, PANEL_DEFAULT_FLAGS, setRebinding } from '../../composables/useKeyboardShortcuts'
import { usePlatform } from '../../composables/useSettingsForm'

defineProps<{ isNarrow: boolean }>()

const settingsStore = useSettingsStore()
const { t } = useI18n()
const { isMac } = usePlatform()

// ── Keyboard rebinding ──
const rebindingAction = ref<ShortcutAction | null>(null)

// Capture mode for the modifier+digit tab/panel-switch rows: unlike
// per-action bindings, only the modifier flags are stored and `key` stays
// empty.
const rebindingTabSwitch = ref(false)
const rebindingPanelSwitch = ref(false)
const rebindingTrayHotkey = ref(false)

// Display grouping for the keyboard settings table: related shortcuts share
// a sub-heading so the long list stays scannable. The two digit-modifier
// rows are hardcoded in the 'tabs' section (custom edit/clear controls).
// Every ShortcutAction must appear exactly once.
const shortcutCategories: { key: string; label: string; actions: ShortcutAction[] }[] = [
  {
    key: 'tabs',
    label: 'shortcut.catTabs',
    actions: ['newConnection', 'nextTab', 'prevTab', 'navigatePrev', 'navigateNext', 'closePanel', 'duplicateSession', 'maximizePanel', 'lockAI'],
  },
  {
    key: 'sidebar',
    label: 'shortcut.catSidebar',
    actions: ['toggleSidebar', 'openQuickCommands', 'focusAI', 'toggleFilePanel', 'toggleMonitor', 'openSettings'],
  },
  {
    key: 'terminal',
    label: 'shortcut.catTerminal',
    actions: ['focusTerminal', 'copy', 'paste', 'terminalSearch', 'zoomFontIn', 'zoomFontOut', 'toggleLineNumbers', 'toggleTimestamps'],
  },
]

function bindingDisplay(action: ShortcutAction): string {
  const b = settingsStore.settings.keyboard[action]
  if (!b) return ''
  return formatKeyBinding(b, isMac.value)
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
  setRebinding(true)
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
  rebindingTabSwitch.value = false
  rebindingPanelSwitch.value = false
  rebindingTrayHotkey.value = false
  setRebinding(false)
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
  if (rebindingTrayHotkey.value) return onRebindTrayHotkeyKeydown(e)
  if (rebindingTabSwitch.value) return onRebindTabSwitchKeydown(e)
  if (rebindingPanelSwitch.value) return onRebindPanelSwitchKeydown(e)
  if (!rebindingAction.value) return stopRebind()
  e.preventDefault()
  e.stopPropagation()
  const key = e.key
  if (key === 'Escape') return stopRebind()
  if (key === 'Control' || key === 'Shift' || key === 'Alt' || key === 'Meta') return
  if (!e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey) {
    // Bare keys are never valid bindings — they would break normal typing.
    msg.warning(t('shortcut.needsModifier'))
    return
  }

  const binding: KeyBinding = {
    // macOS: Cmd is recorded as ctrl — ctrl combos mirror to Cmd at runtime.
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
  const targetKey = bindingKey(binding)
  const kb = settingsStore.settings.keyboard
  for (const [action, b] of Object.entries(kb) as [ShortcutAction, KeyBinding][]) {
    if (action === rebindingAction.value) continue
    if (!b.key) continue
    if (bindingKey(b) === targetKey) return action
  }
  return null
}

function bindingKey(binding: KeyBinding): string {
  return `${binding.ctrl ? 'ctrl+' : ''}${binding.shift ? 'shift+' : ''}${binding.alt ? 'alt+' : ''}${binding.key.toLowerCase()}`
}

// ── Configurable digit modifiers (tab/panel switching) ──
type DigitModifierKey = 'tabSwitchModifier' | 'panelSwitchModifier'

function digitModifierBindingDisplay(key: DigitModifierKey, defaultMods: string): string {
  const b = settingsStore.settings.keyboard[key]
  if (!b) {
    // Effective platform default, shown so the row never reads as "off".
    return `${defaultMods} + ${t('shortcut.digitKeys')}`
  }
  if (!b.ctrl && !b.shift && !b.alt) {
    // Cleared = the family is disabled; show an empty binding.
    return ''
  }
  // formatKeyBinding pushes the (empty) key too, leaving a trailing '+'
  const mods = formatKeyBinding({ ctrl: !!b.ctrl, shift: !!b.shift, alt: !!b.alt, key: '' }, isMac.value).replace(/\++$/, '')
  return `${mods} + ${t('shortcut.digitKeys')}`
}

function tabSwitchBindingDisplay(): string {
  // Mac default shows the ⌘ symbol (not the word "Cmd") so the row matches
  // the symbol style of formatKeyBinding and every other binding row.
  return digitModifierBindingDisplay('tabSwitchModifier', isMac.value ? '⌘' : 'Ctrl')
}

function panelSwitchBindingDisplay(): string {
  return digitModifierBindingDisplay('panelSwitchModifier', isMac.value ? '⌥' : 'Alt')
}

function isTabSwitchModifierUnset(): boolean {
  return !settingsStore.settings.keyboard.tabSwitchModifier
}

function isPanelSwitchModifierUnset(): boolean {
  return !settingsStore.settings.keyboard.panelSwitchModifier
}

function setDigitModifier(key: DigitModifierKey, binding: KeyBinding | null) {
  const kb: KeyboardSettings = { ...settingsStore.settings.keyboard }
  if (binding) {
    const isTab = key === 'tabSwitchModifier'
    // The two digit families are mutually exclusive: assigning one clears the
    // other when it claims the same combo. An unset other family still counts
    // through its own platform default — otherwise e.g. panels set to Ctrl
    // would silently lose to the default tab switching. This runs BEFORE the
    // own-default check below: recording e.g. Ctrl back onto tabs must clear
    // a conflicting panel binding even though Ctrl is also the tab default.
    const otherKey: DigitModifierKey = isTab ? 'panelSwitchModifier' : 'tabSwitchModifier'
    const otherDefault = isTab ? PANEL_DEFAULT_FLAGS : TAB_DEFAULT_FLAGS
    if (digitModifierCollides(kb[otherKey], otherDefault, binding)) {
      kb[otherKey] = { ctrl: false, shift: false, alt: false, key: '' }
    }
    const ownDefault = isTab ? TAB_DEFAULT_FLAGS : PANEL_DEFAULT_FLAGS
    // Recording a family's own platform default (Ctrl for tabs, Alt for
    // panels) is the same as never having configured it — store nothing so
    // the row keeps showing the default and reset stays hidden.
    if (digitModifierFlagsEqual(binding, ownDefault)) {
      delete kb[key]
    } else {
      kb[key] = binding
      // Mirror the per-action conflict rule: an action explicitly bound to
      // the same combo with a digit key would shadow the family for that
      // digit (the global keybinding listener runs before the platform
      // handler), so clear it like findConflict does for regular rebinds.
      for (const [action, b] of Object.entries(kb) as [ShortcutAction, KeyBinding][]) {
        if ((action as string) === key) continue
        if (!b || !b.key || !/^[1-9]$/.test(b.key)) continue
        if (digitModifierFlagsEqual(b, binding)) {
          kb[action] = { ctrl: false, shift: false, alt: false, key: '' }
        }
      }
    }
  } else {
    delete kb[key]
  }
  settingsStore.settings.keyboard = kb
  settingsStore.save()
}

function setTabSwitchModifier(binding: KeyBinding | null) {
  setDigitModifier('tabSwitchModifier', binding)
}

function setPanelSwitchModifier(binding: KeyBinding | null) {
  setDigitModifier('panelSwitchModifier', binding)
}

function resetTabSwitchModifier() {
  // Back to unset = platform default (Ctrl/Cmd+digit tabs, Alt/Option+digit
  // workspace panels).
  setDigitModifier('tabSwitchModifier', null)
}

function resetPanelSwitchModifier() {
  // Back to unset = platform default (Alt/Option+digit workspace panels).
  setDigitModifier('panelSwitchModifier', null)
}

function clearTabSwitchModifier() {
  // All flags off = digit tab switching disabled; panels keep their digits.
  setDigitModifier('tabSwitchModifier', { ctrl: false, shift: false, alt: false, key: '' })
  stopRebind()
}

function clearPanelSwitchModifier() {
  // All flags off = panel digit switching disabled; tabs keep their digits.
  setDigitModifier('panelSwitchModifier', { ctrl: false, shift: false, alt: false, key: '' })
  stopRebind()
}

function startRebindTabSwitch() {
  rebindingTabSwitch.value = true
  uninstallGlobalListener()
  setRebinding(true)
  if (!rebindListenerActive) {
    rebindListenerActive = true
    document.addEventListener('keydown', onRebindKeydown, true)
    window.addEventListener('blur', onRebindBlur)
  }
}

function startRebindPanelSwitch() {
  rebindingPanelSwitch.value = true
  uninstallGlobalListener()
  setRebinding(true)
  if (!rebindListenerActive) {
    rebindListenerActive = true
    document.addEventListener('keydown', onRebindKeydown, true)
    window.addEventListener('blur', onRebindBlur)
  }
}

// Modifier-only capture: bare modifier presses are ignored so the user can
// build the combo (e.g. Ctrl+Alt); the next non-modifier key press confirms
// the held modifiers — its own key is discarded.
function onRebindModifierKeydown(e: KeyboardEvent, apply: (binding: KeyBinding) => void) {
  e.preventDefault()
  e.stopPropagation()
  if (e.key === 'Escape') return stopRebind()
  if (e.key === 'Control' || e.key === 'Shift' || e.key === 'Alt' || e.key === 'Meta') return
  if (!e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey) {
    // Bare keys are never valid bindings — they would break normal typing.
    msg.warning(t('shortcut.needsModifier'))
    return
  }
  // macOS: Cmd is recorded as ctrl — ctrl combos mirror to Cmd at runtime.
  apply({ ctrl: e.ctrlKey || e.metaKey, shift: e.shiftKey, alt: e.altKey, key: '' })
  stopRebind()
}

function onRebindTabSwitchKeydown(e: KeyboardEvent) {
  onRebindModifierKeydown(e, setTabSwitchModifier)
}

function onRebindPanelSwitchKeydown(e: KeyboardEvent) {
  onRebindModifierKeydown(e, setPanelSwitchModifier)
}

// ── Global show/hide hotkey (tray, system-wide) ──

function trayHotkeyDisplay(): string {
  const b = settingsStore.settings.keyboard.trayShowHide
  return b ? formatKeyBinding(b, isMac.value) : ''
}

function isTrayHotkeyDefault(): boolean {
  const b = settingsStore.settings.keyboard.trayShowHide
  const def = DEFAULT_KEYBOARD.trayShowHide
  if (!b || !def) return b === def
  return b.ctrl === def.ctrl && b.shift === def.shift && b.alt === def.alt && b.key === def.key
}

function startRebindTrayHotkey() {
  rebindingTrayHotkey.value = true
  rebindingAction.value = null
  rebindingTabSwitch.value = false
  rebindingPanelSwitch.value = false
  uninstallGlobalListener()
  setRebinding(true)
  if (!rebindListenerActive) {
    rebindListenerActive = true
    document.addEventListener('keydown', onRebindKeydown, true)
    window.addEventListener('blur', onRebindBlur)
  }
}

function onRebindTrayHotkeyKeydown(e: KeyboardEvent) {
  e.preventDefault()
  e.stopPropagation()
  if (e.key === 'Escape') return stopRebind()
  if (e.key === 'Control' || e.key === 'Shift' || e.key === 'Alt' || e.key === 'Meta') return
  if (!e.ctrlKey && !e.metaKey && !e.shiftKey && !e.altKey) {
    // Bare keys are never valid bindings — they would break normal typing.
    msg.warning(t('shortcut.needsModifier'))
    return
  }
  // macOS: Cmd is recorded as ctrl — ctrl combos mirror to Cmd at runtime.
  settingsStore.settings.keyboard = {
    ...settingsStore.settings.keyboard,
    trayShowHide: {
      ctrl: e.ctrlKey || e.metaKey,
      shift: e.shiftKey,
      alt: e.altKey,
      key: e.key.toLowerCase()
    }
  }
  settingsStore.save()
  stopRebind()
}

function clearTrayHotkey() {
  // All flags off + empty key = binding removed, hotkey unregistered.
  settingsStore.settings.keyboard = {
    ...settingsStore.settings.keyboard,
    trayShowHide: { ctrl: false, shift: false, alt: false, key: '' }
  }
  settingsStore.save()
  stopRebind()
}

function resetTrayHotkey() {
  const def = DEFAULT_KEYBOARD.trayShowHide
  if (!def) return
  settingsStore.settings.keyboard = {
    ...settingsStore.settings.keyboard,
    trayShowHide: { ...def }
  }
  settingsStore.save()
}

function onRebindBlur() {
  stopRebind()
}

// Clean up the global keydown listener if the panel is unmounted or
// deactivated (KeepAlive) mid-rebind.
onUnmounted(() => {
  stopRebind()
})
onDeactivated(() => {
  stopRebind()
})
</script>

<style scoped>
@import './_shared.css';

.kb-key {
  display: inline-block;
  padding: 0.125rem 0.5rem;
  background: var(--bg-overlay);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  /* UI font so macOS modifier symbols (⌘⌥⇧) render with their native
     system-font shapes instead of the thin mono fallback glyphs. */
  font-family: var(--font-ui);
  font-size: 0.75rem;
  color: var(--text-primary);
}

.kb-table {
  width: 100%;
  /* min-width keeps the columns (and the action buttons row) from being
     squeezed on narrow panes; percentages keep the five tables aligned. */
  min-width: 22rem;
  table-layout: fixed;
  border-collapse: collapse;
  font-size: 0.8125rem;
}

.kb-table th:first-child {
  width: 30%;
}

.kb-table th:nth-child(2) {
  width: 38%;
}

.kb-table th:nth-child(3) {
  width: 25%;
}

.kb-table th, .kb-table td {
  padding: 0.625rem 0.75rem;
  text-align: left;
  border-bottom: 1px solid var(--border-subtle);
}

.kb-table th {
  color: var(--text-muted);
  font-weight: 500;
  font-size: 0.75rem;
  text-transform: uppercase;
}

.kb-table tbody tr:hover {
  background: var(--bg-hover);
}

.kb-hint {
  margin-top: 0.125rem;
  font-size: 0.75rem;
  color: var(--text-muted);
}

.kb-modifier-note {
  margin: 0 0 0.625rem;
}

/* Must stay a table-cell: display:flex on the <td> removes it from the row's
   shared-height layout and the column drifts out of line when buttons wrap. */
.kb-actions {
  vertical-align: middle;
  white-space: normal;
}

.kb-actions .el-button {
  margin: 0 0.375rem 0.375rem 0;
}

.kb-actions .el-button + .el-button {
  margin-left: 0;
}
</style>