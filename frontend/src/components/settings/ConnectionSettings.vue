<template>
  <div v-if="settingsStore.activeCategory === 'identities'" class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.identities') }}</h2>
    <p class="section-desc">{{ t('settings.identitiesDesc') }}</p>
    <el-button type="primary" @click="openIdentityDialog()">{{ t('settings.addIdentity') }}</el-button>
    <el-table :data="identityStore.identities" size="small" style="margin-top: 0.75rem">
      <el-table-column prop="name" :label="t('conn.name')" />
      <el-table-column prop="username" :label="t('conn.user')" />
      <el-table-column :label="t('conn.authType')">
        <template #default="{ row }">{{ row.authType === 'password' ? t('conn.password') : row.authType === 'keyText' ? t('conn.keyText') : t('conn.keyPath') }}</template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" :width="uiPx(160)">
        <template #default="{ row }">
          <el-button size="small" @click="openIdentityDialog(row)">{{ t('common.edit') }}</el-button>
          <el-button size="small" type="danger" @click="removeIdentity(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <IdentityEditDialog v-model:visible="identityDialogVisible" :identity="editingIdentity" @saved="identityStore.load()" />
  </div>

  <div v-else-if="settingsStore.activeCategory === 'proxies'" class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.proxies') }}</h2>
    <p class="section-desc">{{ t('settings.proxiesDesc') }}</p>
    <el-button type="primary" @click="openProxyDialog()">{{ t('settings.addProxy') }}</el-button>
    <el-table :data="proxyStore.proxies" size="small" style="margin-top: 0.75rem">
      <el-table-column prop="name" :label="t('conn.name')" />
      <el-table-column prop="kind" :label="t('conn.proxyType')" :width="uiPx(100)" />
      <el-table-column :label="t('settings.proxyHost')">
        <template #default="{ row }">{{ row.host }}:{{ row.port }}</template>
      </el-table-column>
      <el-table-column :label="t('settings.enabled')" :width="uiPx(80)">
        <template #default="{ row }">
          <el-switch
            :model-value="row.enabled !== false"
            size="small"
            @change="(v: boolean) => toggleProxy(row, v)"
          />
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" :width="uiPx(160)">
        <template #default="{ row }">
          <el-button size="small" @click="openProxyDialog(row)">{{ t('common.edit') }}</el-button>
          <el-button size="small" type="danger" @click="removeProxy(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <ProxyEditDialog v-model:visible="proxyDialogVisible" :proxy="editingProxy" @saved="proxyStore.load()" />
  </div>

  <div v-else-if="settingsStore.activeCategory === 'tunnels'" class="settings-section" :class="{ narrow: isNarrow }">
    <h2 class="section-title">{{ t('settings.tunnels') }}</h2>
    <p class="section-desc">{{ t('settings.tunnelsDesc') }}</p>
    <el-button type="primary" @click="openTunnelDialog()">{{ t('tunnels.addTunnel') }}</el-button>
    <el-table :data="tunnelStore.tunnels" size="small" style="margin-top: 0.75rem">
      <el-table-column prop="name" :label="t('tunnels.name')" />
      <el-table-column :label="t('tunnels.modeCol')" :width="uiPx(150)">
        <template #default="{ row }">{{ modeName(row.mode) }}</template>
      </el-table-column>
      <el-table-column :label="t('tunnels.listenCol')" :width="uiPx(120)">
        <template #default="{ row }">:{{ effPort(row) }}</template>
      </el-table-column>
      <el-table-column :label="t('tunnels.statusCol')" :width="uiPx(160)">
        <template #default="{ row }">
          <el-switch
            :model-value="tunnelStore.isActive(row.id)"
            :title="tunnelStore.isActive(row.id) ? t('tunnels.stop') : t('tunnels.start')"
            @change="toggleRun(row)"
          />
          <div :title="tunnelStore.statusTitle(row.id)">{{ tunnelStore.statusLabel(row.id) }}</div>
        </template>
      </el-table-column>
      <el-table-column :label="t('common.actions')" :width="uiPx(160)">
        <template #default="{ row }">
          <!-- Connecting and reconnecting tunnels must also stop before edits. -->
          <el-button
            size="small"
            :disabled="tunnelStore.isActive(row.id)"
            :title="tunnelStore.isActive(row.id) ? t('tunnels.editDisabledActive') : undefined"
            @click="openTunnelDialog(row)"
          >{{ t('common.edit') }}</el-button>
          <el-button size="small" type="danger" @click="removeTunnel(row)">{{ t('common.delete') }}</el-button>
        </template>
      </el-table-column>
    </el-table>
    <TunnelEditDialog v-model="tunnelDialogVisible" :editing-id="editingTunnelId" />
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useSettingsStore } from '../../stores/settingsStore'
import { useIdentityStore } from '../../stores/identityStore'
import { useProxyStore } from '../../stores/proxyStore'
import { useTunnelStore } from '../../stores/tunnelStore'
import { useTunnelCredentials } from '../../composables/useTunnelCredentials'
import { useI18n } from '../../i18n'
import { ElMessage } from 'element-plus'
import type { Tunnel, TunnelMode } from '../../stores/tunnelStore'
import type { Identity } from '../../types/identity'
import type { Proxy } from '../../types/proxy'
import { uiPx } from '../../utils/uiScale'
import IdentityEditDialog from '../IdentityEditDialog.vue'
import ProxyEditDialog from '../ProxyEditDialog.vue'
import TunnelEditDialog from '../TunnelEditDialog.vue'

defineProps<{ isNarrow: boolean }>()

const settingsStore = useSettingsStore()
const { t } = useI18n()
const { resolveTunnelCredentials } = useTunnelCredentials()

// ── Identities (密钥库) ──
const identityStore = useIdentityStore()
const identityDialogVisible = ref(false)
const editingIdentity = ref<Identity | null>(null)
function openIdentityDialog(id?: Identity) {
  editingIdentity.value = id ?? null
  identityDialogVisible.value = true
}
async function removeIdentity(row: Identity) {
  await identityStore.remove(row.id)
  ElMessage.success(t('settings.deleted'))
}

// ── Proxies (代理) ──
const proxyStore = useProxyStore()
const proxyDialogVisible = ref(false)
const editingProxy = ref<Proxy | null>(null)
function openProxyDialog(p?: Proxy) {
  editingProxy.value = p ?? null
  proxyDialogVisible.value = true
}
async function removeProxy(row: Proxy) {
  await proxyStore.remove(row.id)
}

// Enable toggle (issue #749): a disabled proxy is skipped on connect and the
// referencing connection dials directly.
async function toggleProxy(row: Proxy, v: boolean) {
  await proxyStore.update({ ...row, enabled: v })
}

// ── Tunnels (隧道) ──
const tunnelStore = useTunnelStore()
const tunnelDialogVisible = ref(false)
const editingTunnelId = ref<string | undefined>(undefined)

function openTunnelDialog(t?: Tunnel) {
  if (t && tunnelStore.isActive(t.id)) return
  editingTunnelId.value = t?.id
  tunnelDialogVisible.value = true
}
async function removeTunnel(row: Tunnel) {
  await tunnelStore.deleteTunnel(row.id)
}
const modeName = (m: TunnelMode) => t(`tunnels.mode.${m}`)
const effPort = (t: Tunnel) => tunnelStore.states[t.id]?.localPort || t.listenPort
async function toggleRun(row: Tunnel) {
  if (tunnelStore.isActive(row.id)) {
    await tunnelStore.stop(row.id)
  } else {
    // Exit connection without saved credentials: prompt for user/password and
    // pass them inline (backend fills only empty fields). Cancel aborts.
    const creds = await resolveTunnelCredentials(row.sshConnId)
    if (!creds) return
    await tunnelStore.start(row.id, creds.user, creds.password)
  }
}

onMounted(async () => {
  try {
    await identityStore.load()
  } catch {
    // Ignore identity load errors
  }
  try {
    await proxyStore.load()
  } catch {
    // Ignore proxy load errors
  }
  try {
    await tunnelStore.load()
  } catch {
    // Ignore tunnel load errors
  }
})
</script>

<style scoped>
@import './_shared.css';
</style>