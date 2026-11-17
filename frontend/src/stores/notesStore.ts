import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import { SaveNotes, LoadNotes } from '../../bindings/github.com/ys-ll/uniterm/app'
import { Events } from '@wailsio/runtime'

export interface Note {
  id: string
  title?: string
  category?: string
  content: string
  createdAt: string
  updatedAt: string
}

export interface NotesData {
  version: number
  notes: Note[]
}

let idCounter = 0
function genId(): string {
  return `note-${Date.now()}-${++idCounter}`
}

// Un-subscriber for the sync-pull store:notes:changed listener (module-level,
// mirroring skillStore).
let unsubNotesChanged: (() => void) | null = null

export const useNotesStore = defineStore('notes', () => {
  const notes = ref<Note[]>([])
  const loaded = ref(false)

  async function load() {
    if (loaded.value) return
    try {
      const data: NotesData = await LoadNotes()
      notes.value = data.notes || []
    } catch (e) {
      console.error('Failed to load notes:', e)
      notes.value = []
    }
    loaded.value = true
  }

  // A sync pull may have rewritten notes.json wholesale — force a reload
  // bypassing the load-once guard.
  async function reload() {
    loaded.value = false
    await load()
  }

  unsubNotesChanged?.()
  unsubNotesChanged = Events.On('store:notes:changed', () => { void reload() })

  async function save() {
    try {
      await SaveNotes({
        version: 1,
        notes: JSON.parse(JSON.stringify(notes.value)),
      })
    } catch (e) {
      console.error('Failed to save notes:', e)
    }
  }

  function add(category?: string): Note {
    const now = new Date().toISOString()
    const note: Note = {
      id: genId(),
      title: undefined,
      category: category || undefined,
      content: '',
      createdAt: now,
      updatedAt: now,
    }
    notes.value.push(note)
    save()
    return note
  }

  function update(id: string, patch: Partial<Pick<Note, 'title' | 'category' | 'content'>>) {
    const n = notes.value.find(x => x.id === id)
    if (!n) return
    if (patch.title !== undefined) n.title = patch.title || undefined
    if (patch.category !== undefined) n.category = patch.category || undefined
    if (patch.content !== undefined) n.content = patch.content
    n.updatedAt = new Date().toISOString()
    save()
  }

  function remove(id: string) {
    notes.value = notes.value.filter(n => n.id !== id)
    save()
  }

  // Newest-edited first: the list panel reads this.
  const sortedNotes = computed(() =>
    [...notes.value].sort((a, b) => (b.updatedAt || '').localeCompare(a.updatedAt || ''))
  )

  // All categories in use, alphabetical, for the filter dropdown.
  const categories = computed(() =>
    [...new Set(notes.value.map(n => n.category).filter((c): c is string => !!c))].sort()
  )

  function get(id: string): Note | undefined {
    return notes.value.find(n => n.id === id)
  }

  // Category-level ops touch every note carrying the tag but do not bump
  // updatedAt — re-tagging is not an edit of the note content.
  function renameCategory(oldName: string, newName: string) {
    const name = newName.trim()
    if (!name || name === oldName) return
    notes.value.forEach(n => { if (n.category === oldName) n.category = name })
    save()
  }

  // Deletes the category only: its notes stay, becoming uncategorized.
  function removeCategory(name: string) {
    notes.value.forEach(n => { if (n.category === name) n.category = undefined })
    save()
  }

  return {
    notes, loaded, sortedNotes, categories,
    load, reload, save,
    add, update, remove, get,
    renameCategory, removeCategory,
  }
})
