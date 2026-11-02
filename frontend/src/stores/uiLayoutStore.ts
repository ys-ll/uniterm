import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { useLocalStateStore } from './localStateStore'
import { useSettingsStore } from './settingsStore'
import { PANEL_LOCATION_DEFAULTS, SIDEBAR_TAB_ORDER } from '../types/settings'

// Dock position of a sidebar view: left sidebar or the bottom panel.
export type PanelLocation = 'sidebar' | 'bottom'

// Views that can live in either the left sidebar or the bottom panel. The
// view set mirrors SIDEBAR_TAB_ORDER in types/settings.ts.
export type SidebarView =
  | 'connections'
  | 'quickCommands'
  | 'history'
  | 'personalization'
  | 'files'
  | 'monitor'
  | 'tunnels'

export const BOTTOM_PANEL_MIN_HEIGHT = 120

// Below this window width the layout switches to "narrow" mode: the bottom
// panel is hidden and its views display inside the left sidebar instead
// (their configured docks stay saved and take effect again when the window
// widens). Matches the app's existing 640px stacking breakpoint.
export const NARROW_LAYOUT_MAX_WIDTH = 640

// UI layout state shared by the left sidebar and the bottom panel: which
// view is active, where each view is docked and in what tab order (persisted
// in AppSettings.panelLocations/panelOrder, next to the sidebarTabs
// visibility config), plus the bottom panel's size/visibility (device-local
// state in localStateStore, like the sidebar visibility flags).
export const useUiLayoutStore = defineStore('uiLayout', () => {
  const settingsStore = useSettingsStore()
  const localState = useLocalStateStore()

  // Each dock keeps its OWN active view, so the left sidebar and the bottom
  // panel can show content simultaneously (like VS Code's sidebar + panel).
  const sidebarActiveView = ref<SidebarView>('connections')
  const bottomActiveView = ref<SidebarView | null>(null)
  // View id currently being dragged between the two strips (null otherwise).
  // Lets a hidden dock reveal itself as a drop target while dragging.
  const draggingPanel = ref<string | null>(null)
  // True once BottomPanel has mounted and #bottom-panel-body exists in the
  // DOM. Teleports into it must wait for this — Sidebar mounts BEFORE the
  // bottom panel, and a Teleport whose target is missing at mount stays
  // broken for its whole lifetime (null-anchor crashes on later patches).
  const bottomPanelBodyReady = ref(false)

  // Bottom panel visibility/height: device-local (never synced).
  const bottomPanelVisible = computed(() => localState.state.bottomPanelVisible ?? true)
  const bottomPanelHeight = computed(() => {
    const h = localState.state.bottomPanelHeight
    return h > 0 ? h : 220
  })

  // Dock positions and tab order: synced settings (AppSettings).
  const panelLocations = computed<Record<string, PanelLocation>>(() => ({
    ...PANEL_LOCATION_DEFAULTS,
    ...(settingsStore.settings.panelLocations || {}),
  }))
  const panelOrder = computed<string[]>(() => settingsStore.settings.panelOrder || [])

  // Number of views currently docked to the bottom panel (header button
  // active state; the panel itself hides when its last tab leaves).
  const bottomTabCount = computed(() =>
    Object.values(panelLocations.value).filter(loc => loc === 'bottom').length
  )

  // All views in persisted order (missing ids append in SIDEBAR_TAB_ORDER
  // order). Each strip filters this by dock location.
  const orderedViews = computed<string[]>(() => {
    const known = SIDEBAR_TAB_ORDER.map(tab => tab.key)
    const saved = panelOrder.value.filter(v => known.includes(v))
    const missing = known.filter(k => !saved.includes(k))
    return [...saved, ...missing]
  })

  function panelLocation(view: string): PanelLocation {
    return panelLocations.value[view] ?? PANEL_LOCATION_DEFAULTS[view] ?? 'sidebar'
  }

  // Narrow mode: everything renders in the left sidebar regardless of the
  // configured dock. Follows window resizes (app-lifetime store, so the
  // listener is never removed).
  const narrowLayout = ref(window.innerWidth <= NARROW_LAYOUT_MAX_WIDTH)
  window.addEventListener('resize', () => {
    narrowLayout.value = window.innerWidth <= NARROW_LAYOUT_MAX_WIDTH
  })

  // The dock a view renders in right now (config, overridden by narrow mode).
  function effectiveLocation(view: string): PanelLocation {
    return narrowLayout.value ? 'sidebar' : panelLocation(view)
  }

  // Whether a view's body should render right now: hidden views never
  // render; otherwise the body shows in the dock it lives in, when that
  // dock's active view is this view.
  function isViewActive(view: SidebarView): boolean {
    if (settingsStore.settings.sidebarTabs?.[view] === false) return false
    return effectiveLocation(view) === 'bottom'
      ? bottomActiveView.value === view
      : sidebarActiveView.value === view
  }

  // Move a view to a dock at the given index within that dock's tab subset
  // (used by both the context menu and tab drag & drop). Moving to the
  // bottom opens the panel; moving the last bottom tab away hides it. When
  // the dock actually changes, the view becomes the active view of its new
  // dock (pure reorders within a dock leave both active views alone).
  // In narrow mode docks are display-merged into the sidebar: only the tab
  // order changes, every view's configured dock stays saved.
  function movePanel(view: string, location: PanelLocation, index: number) {
    if (narrowLayout.value) {
      const views = orderedViews.value.filter(v => v !== view)
      const beforeId = views[Math.max(0, Math.min(index, views.length))]
      const order = panelOrder.value.length > 0 ? [...panelOrder.value] : [...orderedViews.value]
      const from = order.indexOf(view)
      if (from !== -1) order.splice(from, 1)
      const at = beforeId !== undefined ? order.indexOf(beforeId) : -1
      if (at === -1) order.push(view)
      else order.splice(at, 0, view)
      settingsStore.settings.panelOrder = order
      settingsStore.save()
      return
    }
    const prevLocation = panelLocation(view)
    const dockViews = orderedViews.value.filter(v => panelLocation(v) === location && v !== view)
    const beforeId = dockViews[Math.max(0, Math.min(index, dockViews.length))]
    // Global order: remove the view, then re-insert before its new neighbor
    // (subset order follows global order, so this lands it at `index`).
    const order = panelOrder.value.length > 0 ? [...panelOrder.value] : [...orderedViews.value]
    const from = order.indexOf(view)
    if (from !== -1) order.splice(from, 1)
    const at = beforeId !== undefined ? order.indexOf(beforeId) : -1
    if (at === -1) order.push(view)
    else order.splice(at, 0, view)
    settingsStore.settings.panelOrder = order

    settingsStore.settings.panelLocations = { ...panelLocations.value, [view]: location }
    if (location !== prevLocation) {
      if (location === 'bottom') {
        bottomActiveView.value = view as SidebarView
        localState.update({ bottomPanelVisible: true })
        if (sidebarActiveView.value === view) sidebarActiveView.value = 'connections'
      } else {
        sidebarActiveView.value = view as SidebarView
        if (bottomActiveView.value === view) bottomActiveView.value = null
        if (bottomTabCount.value === 0) localState.update({ bottomPanelVisible: false })
      }
    }
    settingsStore.save()
  }

  // Context-menu toggle: move a view to the other dock (appending at the
  // end of the target dock) and show it there.
  function togglePanelLocation(view: string) {
    movePanel(view, panelLocation(view) === 'bottom' ? 'sidebar' : 'bottom', Number.MAX_SAFE_INTEGER)
  }

  // Show/hide a view's tab everywhere (AppSettings.sidebarTabs). Hiding the
  // active view falls back to another visible view in the same dock (left
  // dock keeps "connections" as its last resort — if everything is hidden
  // the sidebar body is simply empty).
  function setViewVisible(view: string, v: boolean) {
    settingsStore.settings.sidebarTabs = { ...(settingsStore.settings.sidebarTabs || {}), [view]: v }
    settingsStore.save()
    if (!v) {
      const fallback = (loc: PanelLocation): SidebarView | null => {
        const found = orderedViews.value.find(
          x => x !== view && (settingsStore.settings.sidebarTabs?.[x] ?? true) && effectiveLocation(x) === loc
        )
        return (found as SidebarView) ?? null
      }
      if (sidebarActiveView.value === view) sidebarActiveView.value = fallback('sidebar') ?? 'connections'
      if (bottomActiveView.value === view) bottomActiveView.value = fallback('bottom')
    }
  }

  function setBottomPanelVisible(v: boolean) {
    localState.update({ bottomPanelVisible: v })
  }

  function setBottomPanelHeight(h: number) {
    localState.update({ bottomPanelHeight: Math.round(Math.max(BOTTOM_PANEL_MIN_HEIGHT, h)) })
  }

  async function init() {
    // Hydrate the device-local bottom-panel fields (settings load on their
    // own through settingsStore).
    await localState.init()
  }

  return {
    sidebarActiveView,
    bottomActiveView,
    draggingPanel,
    bottomPanelBodyReady,
    bottomPanelVisible,
    bottomPanelHeight,
    narrowLayout,
    panelLocations,
    panelOrder,
    bottomTabCount,
    orderedViews,
    init,
    panelLocation,
    effectiveLocation,
    isViewActive,
    movePanel,
    togglePanelLocation,
    setViewVisible,
    setBottomPanelVisible,
    setBottomPanelHeight,
  }
})
