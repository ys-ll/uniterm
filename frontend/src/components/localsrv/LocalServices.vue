<template>
  <el-dialog
    append-to-body
    :model-value="visible"
    :title="t('localsrv.title')"
    width="80%"
    :close-on-click-modal="false"
    destroy-on-close
    class="localsrv-dialog"
    style="display:flex; flex-direction:column; height:80vh; max-height:80vh; border-radius:var(--radius-sm);"
    @update:model-value="(v: boolean) => emit('update:visible', v)"
  >
    <div class="localsrv-body">
      <!-- ── Left: server list ── -->
      <div class="server-list">
        <div
          v-for="svc in services"
          :key="svc.key"
          class="server-row"
          :class="{ active: selected === svc.key, running: isRunning(svc.key) }"
          @click="selectService(svc.key)"
        >
          <span class="server-name">{{ t(`localsrv.${svc.key}.name`) }}</span>
          <div class="server-actions">
            <button
              v-if="!isRunning(svc.key)"
              class="action-btn play"
              :title="t('localsrv.start')"
              @click.stop="startServer(svc.key)"
            >
              <el-icon><Play /></el-icon>
            </button>
            <button
              v-else
              class="action-btn stop"
              :title="t('localsrv.stop')"
              @click.stop="stopServer(svc.key)"
            >
              <el-icon><Pause /></el-icon>
            </button>
          </div>
        </div>
      </div>

      <!-- ── Right: settings + log ── -->
      <div class="server-detail">
        <div class="detail-header">
          <span class="detail-title">{{ t(`localsrv.${selected}.name`) }} {{ t('localsrv.settings') }}</span>
        </div>

        <div class="detail-form">
          <!-- Root directory (file-sharing services only) -->
          <div v-if="['http','tftp','ssh','ftp','s3'].includes(selected)" class="form-row">
            <label class="form-label">{{ t('localsrv.rootDir') }}</label>
            <el-input
              v-model="config[selected].rootDir"
              :placeholder="t('localsrv.rootDir')"
              :disabled="isRunning(selected)"
              class="path-input"
            />
            <button class="browse-btn" :disabled="isRunning(selected)" @click="browseDir(selected)">
              <el-icon><FolderOpen /></el-icon>
            </button>
          </div>

          <!-- HTTP settings -->
          <template v-if="selected === 'http'">
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.http.listDir') }}</label>
              <el-checkbox v-model="config.http.listDir" :disabled="isRunning('http')" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.port') }}</label>
              <el-input-number v-model="config.http.port" :min="1" :max="65535" :disabled="isRunning('http')" controls-position="right" />
            </div>
          </template>

          <!-- TFTP settings -->
          <template v-if="selected === 'tftp'">
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.port') }}</label>
              <el-input-number v-model="config.tftp.port" :min="1" :max="65535" :disabled="isRunning('tftp')" controls-position="right" />
              <span class="form-hint">{{ t('localsrv.tftp.blockSize') }}</span>
              <el-input-number v-model="config.tftp.blockSize" :min="512" :max="65464" :disabled="isRunning('tftp')" controls-position="right" />
            </div>
          </template>

          <!-- SSH/SFTP settings -->
          <template v-if="selected === 'ssh'">
            <div class="ssh-hint">{{ t('localsrv.ssh.hint') }}</div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.port') }}</label>
              <el-input-number v-model="config.ssh.port" :min="1" :max="65535" :disabled="isRunning('ssh')" controls-position="right" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.ssh.username') }}</label>
              <el-input v-model="config.ssh.username" :disabled="isRunning('ssh')" class="cred-input" :placeholder="t('localsrv.ssh.userPlaceholder')" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.ssh.password') }}</label>
              <el-input v-model="config.ssh.password" type="password" show-password :disabled="isRunning('ssh')" class="cred-input" :placeholder="t('localsrv.ssh.passPlaceholder')" />
            </div>
          </template>

          <!-- FTP settings -->
          <template v-if="selected === 'ftp'">
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.ftp.username') }}</label>
              <el-input v-model="config.ftp.username" :disabled="isRunning('ftp')" class="cred-input" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.ftp.password') }}</label>
              <el-input v-model="config.ftp.password" type="password" show-password :disabled="isRunning('ftp')" class="cred-input" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.port') }}</label>
              <el-input-number v-model="config.ftp.port" :min="1" :max="65535" :disabled="isRunning('ftp')" controls-position="right" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.ftp.anonymous') }}</label>
              <el-checkbox v-model="config.ftp.anonymous" :disabled="isRunning('ftp')" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.ftp.utf8') }}</label>
              <el-checkbox v-model="config.ftp.utf8" :disabled="isRunning('ftp')" />
            </div>
          </template>

          <!-- Telnet settings -->
          <template v-if="selected === 'telnet'">
            <div class="ssh-hint">{{ t('localsrv.telnet.hint') }}</div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.port') }}</label>
              <el-input-number v-model="config.telnet.port" :min="1" :max="65535" :disabled="isRunning('telnet')" controls-position="right" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.telnet.password') }}</label>
              <el-input v-model="config.telnet.password" type="password" show-password :disabled="isRunning('telnet')" class="cred-input" :placeholder="t('localsrv.telnet.passPlaceholder')" />
            </div>
          </template>

          <!-- NFS settings -->
          <template v-if="selected === 'nfs'">
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.port') }}</label>
              <el-input-number v-model="config.nfs.port" :min="1" :max="65535" :disabled="isRunning('nfs')" controls-position="right" />
            </div>
            <div class="form-row">
              <span class="form-hint">{{ t('localsrv.nfs.hint') }}</span>
            </div>
          </template>

          <!-- S3 settings -->
          <template v-if="selected === 's3'">
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.port') }}</label>
              <el-input-number v-model="config.s3.port" :min="1" :max="65535" :disabled="isRunning('s3')" controls-position="right" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.s3.accessKey') }}</label>
              <el-input v-model="config.s3.accessKey" :disabled="isRunning('s3')" class="cred-input" :placeholder="t('localsrv.s3.keyPlaceholder')" />
            </div>
            <div class="form-row">
              <label class="form-label">{{ t('localsrv.s3.secretKey') }}</label>
              <el-input v-model="config.s3.secretKey" type="password" show-password :disabled="isRunning('s3')" class="cred-input" :placeholder="t('localsrv.s3.keyPlaceholder')" />
            </div>
            <div class="form-row">
              <span class="form-hint">{{ t('localsrv.s3.hint') }}</span>
            </div>
          </template>

          <!-- Timeout auto-stop (all services, shared) -->
          <div class="form-row timeout-row">
            <el-checkbox
              :model-value="config[selected].timeoutSeconds > 0"
              :disabled="isRunning(selected)"
              @update:model-value="(v: boolean) => config[selected].timeoutSeconds = v ? 3600 : 0"
            >{{ t('localsrv.timeout.label') }}</el-checkbox>
            <el-input-number
              v-if="config[selected].timeoutSeconds > 0"
              :model-value="config[selected].timeoutSeconds"
              :min="10"
              :step="60"
              :controls="false"
              class="timeout-input"
              :disabled="isRunning(selected)"
              @update:model-value="(v: number) => config[selected].timeoutSeconds = v || 0"
            />
            <span v-if="config[selected].timeoutSeconds > 0" class="form-hint">{{ t('localsrv.timeout.seconds') }}</span>
          </div>
        </div>

        <!-- Server output log -->
        <div class="log-section">
          <div class="log-header">{{ t('localsrv.output') }}</div>
          <div class="log-viewport" ref="logViewportRef">
            <div
              v-for="(line, i) in logs[selected]"
              :key="i"
              class="log-line"
            >{{ line }}</div>
            <div v-if="logs[selected].length === 0" class="log-empty">{{ t('localsrv.outputEmpty') }}</div>
          </div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, reactive, onMounted, onUnmounted, nextTick } from 'vue'
import { Events } from '@wailsio/runtime'
import { FolderOpen, Play, Pause } from '@lucide/vue'
import {
  StartHTTPServer, StartTFTPServer, StartSSHServer, StartFTPServer,
  StartTelnetServer, StartNFSServer, StartS3Server,
  StopLocalServer, OpenDirectoryDialog
} from '../../../bindings/github.com/ys-ll/uniterm/app'
import { useI18n } from '../../i18n'
import { msg } from '../../services/message'

defineOptions({ name: 'LocalServices' })

const props = defineProps<{ visible?: boolean }>()
void props
const emit = defineEmits<{
  (e: 'update:visible', v: boolean): void
}>()

const { t } = useI18n()

type ServiceKey = 'http' | 'tftp' | 'ssh' | 'ftp' | 'telnet' | 'nfs' | 's3'

const services: { key: ServiceKey }[] = [
  { key: 'tftp' },
  { key: 'ftp' },
  { key: 'http' },
  { key: 'ssh' },
  { key: 'telnet' },
  { key: 'nfs' },
  { key: 's3' }
]

const selected = ref<ServiceKey>('tftp')

// Per-service configuration (defaults mirror MobaXterm).
const config = reactive({
  http:   { rootDir: '', port: 80, listDir: true, timeoutSeconds: 0 },
  tftp:   { rootDir: '', port: 69, blockSize: 8192, timeoutSeconds: 0 },
  ssh:    { rootDir: '', port: 22, username: '', password: '', timeoutSeconds: 0 },
  ftp:    { rootDir: '', port: 21, username: '', password: '', anonymous: true, utf8: true, timeoutSeconds: 0 },
  telnet: { port: 23, password: '', timeoutSeconds: 0 },
  nfs:    { port: 2049, timeoutSeconds: 0 },
  s3:     { rootDir: '', port: 9000, accessKey: '', secretKey: '', timeoutSeconds: 0 }
})

// Per-service runtime state.
const serverIds = reactive<Record<ServiceKey, string>>({
  http: '', tftp: '', ssh: '', ftp: '', telnet: '', nfs: '', s3: ''
})
const running = reactive<Record<ServiceKey, boolean>>({
  http: false, tftp: false, ssh: false, ftp: false, telnet: false, nfs: false, s3: false
})
const logs = reactive<Record<ServiceKey, string[]>>({
  http: [], tftp: [], ssh: [], ftp: [], telnet: [], nfs: [], s3: []
})

const logViewportRef = ref<HTMLElement | null>(null)

function isRunning(key: ServiceKey): boolean {
  return running[key]
}

function selectService(key: ServiceKey) {
  selected.value = key
}

async function browseDir(key: ServiceKey) {
  try {
    const dir = await OpenDirectoryDialog(config[key].rootDir)
    if (dir) config[key].rootDir = dir
  } catch (e) {
    msg.error(`${t('localsrv.browseFailed')} ${(e as Error)?.message ?? ''}`)
  }
}

const serviceLabel: Record<ServiceKey, string> = {
  http: 'HTTP', tftp: 'TFTP', ssh: 'SSH/SFTP', ftp: 'FTP', telnet: 'Telnet', nfs: 'NFS', s3: 'S3'
}

async function startServer(key: ServiceKey) {
  const rootDir = config[key].rootDir
  // Some services (telnet, nfs) don't need a root directory.
  const needsRootDir = ['http', 'tftp', 'ssh', 'ftp', 's3'].includes(key)
  if (needsRootDir && !rootDir) {
    msg.warning(t('localsrv.missingRootDir'))
    return
  }
  // Credentials are mandatory for the services that expose a local shell.
  if (key === 'ssh' && (!config.ssh.username || !config.ssh.password)) {
    msg.warning(t('localsrv.ssh.missingCreds'))
    return
  }
  if (key === 'telnet' && !config.telnet.password) {
    msg.warning(t('localsrv.telnet.missingPassword'))
    return
  }
  try {
    let id: string
    switch (key) {
      case 'http':
        id = await StartHTTPServer(rootDir, config.http.port, config.http.listDir, config.http.timeoutSeconds)
        break
      case 'tftp':
        id = await StartTFTPServer(rootDir, config.tftp.port, config.tftp.blockSize, config.tftp.timeoutSeconds)
        break
      case 'ssh':
        id = await StartSSHServer(rootDir, config.ssh.port, config.ssh.username, config.ssh.password, config.ssh.timeoutSeconds)
        break
      case 'ftp':
        id = await StartFTPServer(rootDir, config.ftp.port, config.ftp.username, config.ftp.password, config.ftp.anonymous, config.ftp.utf8, config.ftp.timeoutSeconds)
        break
      case 'telnet':
        id = await StartTelnetServer(config.telnet.port, config.telnet.password, config.telnet.timeoutSeconds)
        break
      case 'nfs':
        id = await StartNFSServer(config.nfs.port, config.nfs.timeoutSeconds)
        break
      case 's3':
        id = await StartS3Server(config.s3.rootDir, config.s3.port, config.s3.accessKey, config.s3.secretKey, config.s3.timeoutSeconds)
        break
    }
    serverIds[key] = id
    running[key] = true
    appendLog(key, `Started ${serviceLabel[key]} server ... listening on port ${config[key].port}`)
  } catch (e: any) {
    const detail = e?.message || ''
    msg.error(detail || t('localsrv.startFailed'))
    appendLog(key, `Failed to start ${serviceLabel[key]} server: ${detail || 'unknown error'}`)
  }
}

function stopServer(key: ServiceKey) {
  if (serverIds[key]) {
    StopLocalServer(serverIds[key])
  }
  // Update the UI immediately; the state event also reconciles afterwards.
  running[key] = false
  appendLog(key, `Stopped ${serviceLabel[key]} server`)
}

function appendLog(key: ServiceKey, message: string) {
  const now = new Date()
  const stamp = now.toLocaleTimeString('en-GB', { hour12: false }) + '.' + String(now.getMilliseconds()).padStart(3, '0')
  logs[key].push(`[${stamp}] ${message}`)
  // Cap log size to avoid unbounded growth.
  if (logs[key].length > 500) logs[key].shift()
  if (selected.value === key) {
    nextTick(() => {
      if (logViewportRef.value) {
        logViewportRef.value.scrollTop = logViewportRef.value.scrollHeight
      }
    })
  }
}

// Filter events to the server we started.
function ownedService(id: string): ServiceKey | null {
  for (const { key } of services) {
    if (serverIds[key] === id) return key
  }
  return null
}

function handleLog(ev: any) {
  const d = ev?.data
  if (!d) return
  const key = ownedService(d.id)
  if (!key) return
  appendLog(key, d.message)
}

function handleState(ev: any) {
  const d = ev?.data
  if (!d) return
  const key = ownedService(d.id)
  if (!key) return
  running[key] = !!d.running
  if (!d.running) {
    serverIds[key] = ''
  }
}

let unsubs: (() => void)[] = []

onMounted(() => {
  unsubs = [
    Events.On('localsrv:log', handleLog),
    Events.On('localsrv:state', handleState)
  ]
})

onUnmounted(() => {
  // Stop any servers this component started.
  for (const { key } of services) {
    if (serverIds[key]) StopLocalServer(serverIds[key])
  }
  unsubs.forEach(u => u())
})
</script>

<style scoped>
.localsrv-dialog {
  display: flex;
  flex-direction: column;
  height: 80vh;
  max-height: 80vh;
  background: var(--bg-base);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
}
.localsrv-dialog :deep(.el-dialog__header) {
  padding: 0.625rem 1rem;
  border-bottom: 1px solid var(--border-subtle);
  margin: 0;
  background: var(--bg-elevated);
  flex-shrink: 0;
}
.localsrv-dialog :deep(.el-dialog__title) {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
}
.localsrv-dialog :deep(.el-dialog__body) {
  flex: 1;
  min-height: 0;
  padding: 0;
  overflow: hidden;
}

/* ── Two-pane layout: server list (left) + detail (right) ── */
.localsrv-body {
  display: flex;
  height: 100%;
  min-height: 0;
}

/* ── Left: server list ── */
.server-list {
  width: 13rem;
  flex-shrink: 0;
  border-right: 1px solid var(--border-subtle);
  background: var(--bg-elevated);
  padding: 0.5rem;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  overflow-y: auto;
}
.server-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.5rem 0.625rem;
  border-radius: var(--radius-sm);
  cursor: pointer;
  transition: background 0.12s ease;
  border: 1px solid transparent;
}
.server-row:hover {
  background: var(--bg-hover);
}
.server-row.active {
  background: var(--accent-subtle);
  border-color: var(--border-subtle);
}
.server-row.running .server-name {
  color: var(--success);
  font-weight: 600;
}
.server-name {
  font-size: 0.8125rem;
  font-family: var(--font-ui);
  color: var(--text-primary);
}
.server-actions {
  display: flex;
  gap: 0.25rem;
}
.action-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.5rem;
  height: 1.5rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-base);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.12s ease;
  padding: 0;
}
.action-btn:hover {
  border-color: var(--accent);
  color: var(--accent);
}
.action-btn.play:hover {
  background: var(--success-subtle);
  border-color: var(--success);
  color: var(--success);
}
.action-btn.stop:hover {
  background: color-mix(in srgb, var(--el-color-danger) 12%, transparent);
  border-color: var(--el-color-danger);
  color: var(--el-color-danger);
}
.action-btn .el-icon {
  font-size: 0.75rem;
}

/* ── Right: detail panel ── */
.server-detail {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  padding: 0.75rem 1rem;
  gap: 0.75rem;
  overflow: hidden;
}
.detail-header {
  flex-shrink: 0;
}
.detail-title {
  font-size: 0.875rem;
  font-weight: 600;
  color: var(--text-primary);
  font-family: var(--font-ui);
}

.detail-form {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.form-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.form-label {
  font-size: 0.8125rem;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  min-width: 7rem;
  flex-shrink: 0;
}
.form-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-family: var(--font-ui);
}
.path-input {
  flex: 1;
  min-width: 10rem;
}
.cred-input {
  width: 12rem;
}
.browse-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 2rem;
  height: 2rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  color: var(--text-secondary);
  cursor: pointer;
  flex-shrink: 0;
  transition: all 0.12s ease;
}
.browse-btn:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
}
.browse-btn:disabled {
  opacity: 0.4;
  cursor: not-allowed;
}
.ssh-hint {
  font-size: 0.75rem;
  color: var(--text-muted);
  font-family: var(--font-ui);
  line-height: 1.5;
  padding: 0.5rem 0.75rem;
  background: var(--bg-elevated);
  border-radius: var(--radius-sm);
}

.timeout-row {
  padding-top: 0.25rem;
  border-top: 1px dashed var(--border-subtle);
  margin-top: 0.25rem;
}
.timeout-input {
  width: 6rem;
}

/* ── Server output log ── */
.log-section {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated);
  overflow: hidden;
}
.log-header {
  padding: 0.375rem 0.625rem;
  border-bottom: 1px solid var(--border-subtle);
  font-size: 0.75rem;
  font-weight: 600;
  color: var(--text-secondary);
  font-family: var(--font-ui);
  text-transform: uppercase;
  letter-spacing: 0.03em;
  flex-shrink: 0;
}
.log-viewport {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: 0.375rem 0.625rem;
  font-family: var(--font-mono);
  font-size: 0.75rem;
  line-height: 1.5;
  /* Allow selecting/copying log lines despite the global user-select: none. */
  user-select: text;
  -webkit-user-select: text;
  cursor: text;
}
.log-line {
  color: var(--text-primary);
  white-space: pre-wrap;
  word-break: break-all;
}
.log-empty {
  color: var(--text-muted);
  font-style: italic;
}
</style>
