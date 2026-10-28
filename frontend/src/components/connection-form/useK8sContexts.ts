import { ref, computed, watch, type Ref, type ComputedRef } from 'vue'
import type { ConnectionConfig } from '../../types/session'
import type { K8sContextInfo } from '../../types/k8s'
import { listContexts } from '../../services/k8sClient'

export interface UseK8sContextsReturn {
  k8sSourceMode: Ref<'file' | 'inline'>
  k8sContexts: Ref<K8sContextInfo[]>
  k8sContextsLoading: Ref<boolean>
  k8sContextsError: Ref<string>
  k8sNamespaceTouched: Ref<boolean>
  k8sNamespaceOptions: ComputedRef<string[]>
  reloadK8sContexts: () => Promise<void>
  onK8sNamespaceInput: () => void
  /** Reset all k8s UI state back to defaults (called by the form reset). */
  resetK8s: () => void
}

/**
 * Kubernetes context/namespace picker state, extracted from ConnectionForm.
 * Owns the contexts list, loading/error flags, namespace suggestions and the
 * watchers that keep them in sync with the config source / selected context.
 *
 * `hydrating` is owned by the parent (it also gates the edit-config hydration)
 * and passed in so the watchers can skip while a stored config is being
 * restored.
 */
export function useK8sContexts(
  form: ConnectionConfig,
  hydrating: Ref<boolean>
): UseK8sContextsReturn {
  const k8sSourceMode = ref<'file' | 'inline'>('inline')
  const k8sContexts = ref<K8sContextInfo[]>([])
  const k8sContextsLoading = ref(false)
  const k8sContextsError = ref('')

  // True once the user hand-edits the namespace field, so that switching the
  // k8s context later doesn't silently overwrite a value they typed.
  const k8sNamespaceTouched = ref(false)
  // Namespace suggestions drawn from the contexts' declared defaults plus the
  // current field value. allow-create lets the user type any namespace.
  const k8sNamespaceOptions = computed(() => {
    const set = new Set<string>()
    if (form.k8sNamespace) set.add(form.k8sNamespace)
    for (const c of k8sContexts.value) if (c.namespace) set.add(c.namespace)
    return Array.from(set)
  })

  async function reloadK8sContexts() {
    k8sContextsLoading.value = true
    k8sContextsError.value = ''
    try {
      const src = k8sSourceMode.value === 'file' ? form.k8sConfigPath : form.k8sConfigInline
      if (!src) return
      const list = await listContexts(src, k8sSourceMode.value === 'file')
      k8sContexts.value = list
      if (!form.k8sContext) {
        const current = list.find(c => c.current)
        if (current) form.k8sContext = current.name
      }
    } catch (e: any) {
      k8sContextsError.value = String(e?.message || e)
    } finally {
      k8sContextsLoading.value = false
    }
  }

  function onK8sNamespaceInput() {
    k8sNamespaceTouched.value = true
  }

  // Wiping the context list when the config source changes (skip during
  // edit-hydration so restored state isn't cleared).
  watch(() => [k8sSourceMode.value, form.k8sConfigPath, form.k8sConfigInline], () => {
    if (hydrating.value) return
    form.k8sContext = ''
    k8sContexts.value = []
  })

  // Load contexts when entering k8s type with an existing config.
  watch(() => form.type, (t) => {
    if (t === 'k8s' && (form.k8sConfigPath || form.k8sConfigInline)) {
      reloadK8sContexts()
    }
  })

  // Adopt the selected context's declared default namespace, unless the user has
  // already hand-typed a value. Editing an existing connection runs under
  // hydrating, where the stored namespace must win.
  watch(() => form.k8sContext, (ctxName) => {
    if (hydrating.value) return
    if (k8sNamespaceTouched.value) return
    if (!ctxName) return
    const ctx = k8sContexts.value.find(c => c.name === ctxName)
    if (ctx && ctx.namespace) form.k8sNamespace = ctx.namespace
  })

  function resetK8s() {
    k8sSourceMode.value = 'inline'
    k8sContexts.value = []
    k8sContextsLoading.value = false
    k8sContextsError.value = ''
    k8sNamespaceTouched.value = false
  }

  return {
    k8sSourceMode,
    k8sContexts,
    k8sContextsLoading,
    k8sContextsError,
    k8sNamespaceTouched,
    k8sNamespaceOptions,
    reloadK8sContexts,
    onK8sNamespaceInput,
    resetK8s,
  }
}