<template>
  <el-dialog
    append-to-body
    :model-value="visible"
    @update:model-value="(v: boolean) => !v && close()"
    :title="t('webUsers.title')"
    width="42rem"
    :close-on-click-modal="false"
  >
    <div class="web-users-create">
      <el-input
        v-model="newUsername"
        :placeholder="t('webUsers.usernamePlaceholder')"
        @keyup.enter="create"
      />
      <el-input
        v-model="newPassword"
        type="password"
        show-password
        :placeholder="t('webUsers.passwordPlaceholder')"
        @keyup.enter="create"
      />
      <el-button type="primary" :loading="creating" @click="create">{{ t('webUsers.add') }}</el-button>
    </div>

    <el-table v-loading="loading" :data="users" size="small" class="web-users-table">
      <el-table-column prop="username" :label="t('webUsers.username')" min-width="140" />
      <el-table-column :label="t('webUsers.role')" width="110">
        <template #default="{ row }">{{ roleLabel(row.role) }}</template>
      </el-table-column>
      <el-table-column :label="t('webUsers.createdAt')" min-width="170">
        <template #default="{ row }">{{ formatDate(row.createdAt) }}</template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" width="90" align="right">
        <template #default="{ row }">
          <el-button
            v-if="row.username !== currentUser"
            link
            type="danger"
            size="small"
            :loading="deleting === row.username"
            @click="remove(row.username)"
          >{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>

    <template #footer>
      <el-button @click="close">{{ t('common.done') }}</el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { ElMessageBox } from 'element-plus'
import { useI18n } from '../i18n'
import { msg } from '../services/message'
import { createGatewayUser, deleteGatewayUser, listGatewayUsers } from '../services/webSftpBridge'

interface GatewayUser {
  username: string
  role: string
  createdAt: string
  disabled?: boolean
}

const props = defineProps<{ visible: boolean; currentUser: string }>()
const emit = defineEmits<{ (e: 'update:visible', v: boolean): void }>()

const { t } = useI18n()
const users = ref<GatewayUser[]>([])
const loading = ref(false)
const creating = ref(false)
const deleting = ref('')
const newUsername = ref('')
const newPassword = ref('')

watch(
  () => props.visible,
  (visible) => {
    if (visible) void load()
  }
)

function close() {
  emit('update:visible', false)
}

function roleLabel(role: string) {
  return role === 'admin' ? t('webAccount.roleAdmin') : t('webAccount.roleUser')
}

function formatDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? value : date.toLocaleString()
}

async function load() {
  loading.value = true
  try {
    users.value = await listGatewayUsers()
  } catch (e: any) {
    msg.error(e?.message || String(e))
  } finally {
    loading.value = false
  }
}

async function create() {
  const username = newUsername.value.trim()
  if (!username || newPassword.value.length < 6) return
  creating.value = true
  try {
    await createGatewayUser(username, newPassword.value, 'user')
    newUsername.value = ''
    newPassword.value = ''
    msg.success(t('webUsers.created'))
    await load()
  } catch (e: any) {
    msg.error(e?.message || String(e))
  } finally {
    creating.value = false
  }
}

async function remove(username: string) {
  try {
    await ElMessageBox.confirm(
      t('webUsers.deleteConfirm', { name: username }),
      t('webUsers.title'),
      { type: 'warning', confirmButtonText: t('common.delete'), cancelButtonText: t('common.cancel') }
    )
  } catch {
    return
  }
  deleting.value = username
  try {
    await deleteGatewayUser(username)
    msg.success(t('webUsers.deleted'))
    await load()
  } catch (e: any) {
    msg.error(e?.message || String(e))
  } finally {
    deleting.value = ''
  }
}
</script>

<style scoped>
.web-users-create {
  display: flex;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}
.web-users-create :deep(.el-input) {
  flex: 1;
}
.web-users-table {
  width: 100%;
}
</style>