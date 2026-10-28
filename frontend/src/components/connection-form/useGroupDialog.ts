import { ref, computed, type Ref, type ComputedRef } from 'vue'
import { useConnectionStore } from '../../stores/connectionStore'
import type { ConnectionConfig } from '../../types/session'

interface TreeOption {
  value: string
  label: string
  children?: TreeOption[]
}

export interface UseGroupDialogReturn {
  groupTreeData: ComputedRef<TreeOption[]>
  showNewGroupDialog: Ref<boolean>
  newGroupName: Ref<string>
  newGroupParentId: Ref<string | undefined>
  onGroupSelect: (value: string | undefined) => void
  confirmNewGroup: () => Promise<void>
}

/**
 * Group tree-select + new-group dialog state, extracted from ConnectionForm.
 * Builds the tree from the connection store and handles the "create group"
 * inline dialog.
 */
export function useGroupDialog(
  form: ConnectionConfig,
  selectedGroupId: Ref<string | undefined>,
  t: (key: string) => string
): UseGroupDialogReturn {
  const connectionStore = useConnectionStore()

  const groupTreeData = computed<TreeOption[]>(() => {
    function buildTree(nodes: any[]): TreeOption[] {
      return nodes.map(node => ({
        value: node.group.id,
        label: node.group.name,
        children: node.children.length > 0 ? buildTree(node.children) : undefined,
      }))
    }
    return [
      { value: '__none__', label: t('conn.noGroup') },
      ...buildTree(connectionStore.groupedConnections.roots),
    ]
  })

  const showNewGroupDialog = ref(false)
  const newGroupName = ref('')
  const newGroupParentId = ref<string | undefined>(undefined)

  function onGroupSelect(value: string | undefined) {
    if (value === '__new__') {
      showNewGroupDialog.value = true
      newGroupName.value = ''
      newGroupParentId.value = undefined
      return
    }
  }

  async function confirmNewGroup() {
    const name = newGroupName.value.trim()
    if (!name) return
    showNewGroupDialog.value = false
    const group = await connectionStore.addGroup(name, newGroupParentId.value)
    newGroupParentId.value = undefined
    newGroupName.value = ''
    form.groupId = group.id
    selectedGroupId.value = group.id
  }

  return { groupTreeData, showNewGroupDialog, newGroupName, newGroupParentId, onGroupSelect, confirmNewGroup }
}