import { computed, type ComputedRef, type Ref } from 'vue'
import type { ConnectionConfig } from '../../types/session'
import { isSqlDbType } from '../../utils/quickConnect'
import { isMobilePlatform } from '../../utils/platform'
import {
  CATEGORY_META,
  CATEGORY_ORDER,
  CONNECTION_TYPES,
  connectionTypeFormLabel,
  connectionTypeInfo,
} from '../../utils/connectionTypes'

export interface SubTypeInfo {
  type: string
  dbType?: string
  containerRuntime?: string
  label: string
  icon: any
}

export interface UseConnectionCategoriesReturn {
  categories: ComputedRef<{ key: string; label: string; icon: any }[]>
  allSubTypes: ComputedRef<Record<string, SubTypeInfo[]>>
  currentSubTypes: ComputedRef<SubTypeInfo[]>
  category: ComputedRef<string>
  isSubTypeActive: (st: SubTypeInfo) => boolean
  selectType: (st: SubTypeInfo) => void
  onCategorySelect: (catKey: string) => void
}

/**
 * Connection-type category sidebar + sub-type grid logic, extracted from
 * ConnectionForm. Derives everything from the connectionTypes registry and
 * the current platform.
 */
export function useConnectionCategories(
  form: ConnectionConfig,
  isWindows: Ref<boolean>,
  t: (key: string) => string
): UseConnectionCategoriesReturn {
  // Category sidebar and subtype cards derive from the connectionTypes registry.
  const categories = computed(() => CATEGORY_ORDER.map(key => ({
    key,
    label: t(CATEGORY_META[key].labelKey),
    icon: CATEGORY_META[key].icon,
  })))

  const allSubTypes = computed((): Record<string, SubTypeInfo[]> => {
    const groups: Record<string, SubTypeInfo[]> = {}
    for (const cat of CATEGORY_ORDER) groups[cat] = []
    for (const info of CONNECTION_TYPES) {
      if (info.formHidden) continue
      if (info.windowsOnly && !isWindows.value) continue
      // Local terminals spawn a host shell, serial needs host serial-port
      // hardware and X11 needs a local X server — none of which exist on
      // android/ios (same rule as StartTabContent's hidden local-terminal
      // button).
      if (isMobilePlatform() && (info.type === 'local' || info.type === 'serial' || info.type === 'x11-desktop')) continue
      groups[info.category].push({
        type: info.type,
        dbType: info.dbType,
        containerRuntime: info.containerRuntime,
        label: connectionTypeFormLabel(info, t),
        icon: info.icon,
      })
    }
    return groups
  })

  // Category comes from the connectionTypes registry; only the discriminated
  // 'database' type needs the dbType check (SQL vs NoSQL).
  const category = computed(() => {
    if (form.type === 'database') return isSqlDbType(form.dbType) ? 'sql' : 'nosql'
    return connectionTypeInfo(form.type)?.category || 'terminal'
  })

  const currentSubTypes = computed(() => allSubTypes.value[category.value] || allSubTypes.value.terminal)

  function isSubTypeActive(st: SubTypeInfo): boolean {
    if (st.dbType) {
      return form.type === 'database' && form.dbType === st.dbType
    }
    if (st.containerRuntime) {
      return form.type === 'container' && form.containerRuntime === st.containerRuntime
    }
    return form.type === st.type
  }

  function selectType(st: SubTypeInfo) {
    if (st.dbType) {
      form.type = 'database'
      form.dbType = st.dbType
    } else if (st.containerRuntime) {
      form.type = 'container'
      form.containerRuntime = st.containerRuntime as 'docker' | 'podman' | 'nerdctl' | 'wslc'
    } else {
      form.type = st.type as ConnectionConfig['type']
    }
  }

  function onCategorySelect(catKey: string) {
    if (category.value === catKey) return
    const subs = allSubTypes.value[catKey]
    if (subs && subs.length > 0) {
      selectType(subs[0])
    }
  }

  return { categories, allSubTypes, currentSubTypes, category, isSubTypeActive, selectType, onCategorySelect }
}