<template>
  <div v-if="status.serverMode" class="web-account-wrap">
    <button ref="btnRef" class="web-account-btn" :title="status.user" @click.stop="toggle">
      <span class="web-account-avatar">{{ initial }}</span>
    </button>

    <Menu ref="menuRef" align="end" v-model:visible="menuVisible">
      <div class="web-account-head">
        <div class="web-account-name">{{ status.user }}</div>
        <div class="web-account-role">{{ roleLabel }}</div>
      </div>
      <MenuDivider />
      <MenuItem @click="openPassword">{{ t('webAccount.changePassword') }}</MenuItem>
      <MenuItem v-if="status.role === 'admin'" @click="openUsers">{{ t('webAccount.userManagement') }}</MenuItem>
      <MenuDivider />
      <MenuItem @click="logout">{{ t('webAccount.logout') }}</MenuItem>
    </Menu>

    <WebPasswordDialog v-model:visible="passwordVisible" />
    <WebUserAdminDialog
      v-if="status.role === 'admin'"
      v-model:visible="usersVisible"
      :current-user="status.user"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from '../i18n'
import { getWebModeStatus, logoutWebUser, type WebModeStatus } from '../services/webSftpBridge'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import MenuDivider from './MenuDivider.vue'
import WebPasswordDialog from './WebPasswordDialog.vue'
import WebUserAdminDialog from './WebUserAdminDialog.vue'

const { t } = useI18n()
const status = ref<WebModeStatus>({ serverMode: false, user: '', role: 'user', homeDir: '' })
const menuVisible = ref(false)
const passwordVisible = ref(false)
const usersVisible = ref(false)
const btnRef = ref<HTMLElement | null>(null)
const menuRef = ref<InstanceType<typeof Menu> | null>(null)

const initial = computed(() => (status.value.user || '?').slice(0, 1).toUpperCase())
const roleLabel = computed(() =>
  status.value.role === 'admin' ? t('webAccount.roleAdmin') : t('webAccount.roleUser')
)

onMounted(async () => {
  status.value = await getWebModeStatus()
})

function toggle() {
  menuRef.value?.toggle(btnRef.value!)
}

function openPassword() {
  menuVisible.value = false
  passwordVisible.value = true
}

function openUsers() {
  menuVisible.value = false
  usersVisible.value = true
}

async function logout() {
  menuVisible.value = false
  await logoutWebUser()
}
</script>

<style scoped>
.web-account-wrap {
  position: relative;
  flex-shrink: 0;
  --wails-draggable: no-drag;
}

.web-account-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.75rem;
  height: 1.75rem;
  padding: 0;
  border: none;
  border-radius: 50%;
  background: var(--accent-subtle);
  color: var(--accent);
  cursor: pointer;
  font-family: var(--font-ui);
  font-size: 0.6875rem;
  font-weight: 700;
  transition: background 0.15s ease;
}

.web-account-btn:hover {
  background: var(--bg-hover);
}

.web-account-head {
  min-width: 10rem;
  padding: 0.375rem 0.75rem 0.5rem;
}

.web-account-name {
  color: var(--text-primary);
  font-size: 0.8125rem;
  font-weight: 600;
}

.web-account-role {
  margin-top: 0.125rem;
  color: var(--text-secondary);
  font-size: 0.6875rem;
}
</style>