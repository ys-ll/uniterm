import { defineStore } from 'pinia'
import { ref } from 'vue'
import { GetAppInfo } from '../../bindings/github.com/ys-ll/uniterm/app'

// Feature availability as reported by the backend's server policy
// (AppInfo.features, keyed by serverpolicy group name).
//
// The backend never binds methods of a denied group, so the frontend must
// treat "unknown group / not yet loaded" as disabled: panels gated on this
// store stay unmounted and never call an unbound method (which would
// surface as "unknown bound method id" errors). Desktop builds report
// every group enabled, so nothing is hidden there.
export const useFeaturesStore = defineStore('features', () => {
  const features = ref<Record<string, boolean>>({})
  const loaded = ref(false)

  async function load() {
    try {
      const info = await GetAppInfo()
      features.value = (info.features as Record<string, boolean>) || {}
    } catch (e) {
      console.error('Failed to load feature flags:', e)
      features.value = {}
    } finally {
      loaded.value = true
    }
  }

  function enabled(group: string): boolean {
    return features.value[group] === true
  }

  return { features, loaded, load, enabled }
})
