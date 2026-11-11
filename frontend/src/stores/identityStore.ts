import { defineStore } from 'pinia'
import { ref } from 'vue'
import { LoadIdentities, SaveIdentities } from '../../bindings/github.com/ys-ll/uniterm/app'
import { Events } from '@wailsio/runtime'
import type { Identity, IdentityStoreData } from '../types/identity'

// Module-level un-subscriber for the cross-window store:identities:changed listener.
let unsubIdentitiesChanged: (() => void) | null = null

export const useIdentityStore = defineStore('identity', () => {
  const identities = ref<Identity[]>([])
  const loading = ref(false)

  async function load() {
    loading.value = true
    try {
      const data = (await LoadIdentities()) as IdentityStoreData
      identities.value = data.identities ?? []
    } catch (e) {
      console.error('Failed to load identities:', e)
    } finally {
      loading.value = false
    }
  }

  async function save() {
    await SaveIdentities({ identities: identities.value } as any)
  }

  // Reload from disk when the backend pushes updated identity data (e.g. after
  // a cloud sync pull), so the keychain isn't stale until restart.
  unsubIdentitiesChanged?.()
  unsubIdentitiesChanged =Events.On('store:identities:changed', (ev) => { const data: IdentityStoreData = ev.data; 
    if (data?.identities) identities.value = data.identities
   })

  function dispose() {
    unsubIdentitiesChanged?.()
    unsubIdentitiesChanged = null
  }

  // add/update/remove persist BEFORE mutating the in-memory list: the save
  // can fail closed on the backend (locked vault, unwired store), and a
  // push-then-save used to leave a phantom entry that vanished on reload
  // (issue #1423). The list only updates once the save succeeded.
  async function add(id: Identity) {
    const next = [...identities.value, id]
    await SaveIdentities({ identities: next } as any)
    identities.value = next
  }

  async function update(id: Identity) {
    const i = identities.value.findIndex((x) => x.id === id.id)
    if (i < 0) return
    const next = identities.value.slice()
    next[i] = id
    await SaveIdentities({ identities: next } as any)
    identities.value = next
  }

  async function remove(id: string) {
    const next = identities.value.filter((x) => x.id !== id)
    await SaveIdentities({ identities: next } as any)
    identities.value = next
  }

  return { identities, loading, load, save, add, update, remove, dispose }
})
