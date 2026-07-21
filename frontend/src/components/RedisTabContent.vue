<template>
  <div class="redis-tab-content">
    <div class="redis-main">
      <!-- Left Panel: Key List -->
      <div class="redis-left" :style="{ width: leftWidth + 'px' }">
        <!-- Controls row -->
        <div class="redis-toolbar">
          <el-select v-model="currentDb" size="small" style="width: 90px; flex-shrink: 0" @change="onSwitchDB">
            <el-option v-for="n in 16" :key="n-1" :label="`${n-1} (${dbSizes[n-1] ?? '?'})`" :value="n-1" />
          </el-select>
          <el-input v-model="scanPattern" size="small" placeholder="*" style="flex: 1; min-width: 80px" @keyup.enter="onScan" />
          <button class="btn btn-ghost btn-icon btn-sm" title="Refresh" @click="onScan" style="flex-shrink: 0"><RefreshCw :size="14" /></button>
          <button class="btn btn-ghost btn-icon btn-sm" :title="t('redis.newKey')" @click="onShowNewKeyDialog" style="flex-shrink: 0"><Plus :size="14" /></button>
        </div>

        <!-- Key list -->
        <div class="redis-key-list" v-loading="loading">
          <div v-if="keys.length === 0 && !loading" class="redis-placeholder">{{ t('redis.noKeys') }}</div>
          <div
            v-for="keyInfo in keys"
            :key="keyInfo.name"
            class="key-item"
            :class="{ selected: selectedKey === keyInfo.name }"
            @click="onSelectKey(keyInfo)"
          >
            <span class="key-type-badge">{{ keyInfo.type }}</span>
            <span class="key-name">{{ keyInfo.name }}</span>
          </div>
        </div>

        <!-- Pagination -->
        <div class="redis-pagination">
          <el-select v-model="pageSize" size="small" style="width: 70px" @change="onPageSizeChange">
            <el-option v-for="s in pageSizes" :key="s" :label="String(s)" :value="s" />
          </el-select>
          <span style="flex:1"></span>
          <button class="page-btn" :disabled="cursorStack.length === 0" @click="onPrevPage"><ChevronLeft :size="14" /></button>
          <span class="page-num">{{ currentPage }}</span>
          <button class="page-btn" :disabled="nextCursor === 0 && !hasMore" @click="onNextPage"><ChevronRight :size="14" /></button>
        </div>
      </div>

      <!-- Resizer -->
      <div class="redis-resizer" @mousedown="onResizeStart" />

      <!-- Right Panel: Value Editor -->
      <div class="redis-right">
        <template v-if="selectedKey">
          <div class="key-meta">
            <div class="meta-row"><span class="meta-label">{{ t('redis.keyName') }}</span><span class="meta-value">{{ selectedKey }}</span></div>
            <div class="meta-row"><span class="meta-label">{{ t('redis.type') }}</span><span class="meta-value">{{ selectedKeyInfo?.type || '-' }}</span></div>
            <div class="meta-row">
              <span class="meta-label">{{ t('redis.ttl') }}</span>
              <span class="meta-value ttl-row">
                <el-input-number v-model="editTTL" size="small" style="width: 100px" :min="-1" controls-position="right" />
                <el-button size="small" @click="onSetTTL">{{ t('redis.setTTL') }}</el-button>
                <span style="font-size:11px;color:var(--text-muted)">{{ t('redis.ttlHint') }}</span>
              </span>
            </div>
          </div>

          <div class="redis-right-content" v-loading="keyLoading">
            <!-- String Editor -->
            <template v-if="selectedKeyInfo?.type === 'string'">
              <el-input v-model="stringValue" type="textarea" rows="10" />
            </template>

            <!-- Hash Editor -->
            <template v-else-if="selectedKeyInfo?.type === 'hash'">
              <el-table :data="hashEntries" border size="small">
                <el-table-column type="index" label="#" width="40" />
                <el-table-column prop="field" :label="t('redis.field')">
                  <template #default="{ $index }">
                    <el-input v-model="hashEntries[$index].field" size="small" type="textarea" :rows="1" autosize />
                  </template>
                </el-table-column>
                <el-table-column prop="value" :label="t('redis.value')">
                  <template #default="{ $index }">
                    <el-input v-model="hashEntries[$index].value" size="small" type="textarea" :rows="1" autosize />
                  </template>
                </el-table-column>
                <el-table-column width="50">
                  <template #default="{ $index }">
                    <button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="hashEntries.splice($index, 1)"><Trash2 :size="14" /></button>
                  </template>
                </el-table-column>
              </el-table>
              <el-button size="small" style="margin-top: 4px" @click="hashEntries.push({ field: '', value: '' })"><Plus :size="14" /></el-button>
            </template>

            <!-- List Editor -->
            <template v-else-if="selectedKeyInfo?.type === 'list'">
              <el-table :data="listEntries" border size="small">
                <el-table-column width="26" class-name="drag-col">
                  <template #default="{ $index }">
                    <span class="drag-handle" draggable="true" @dragstart="onListDragStart($index)" @dragover.prevent="onListDragOver($index)" @drop="onListDrop($index)"><GripVertical :size="14" /></span>
                  </template>
                </el-table-column>
                <el-table-column type="index" label="#" width="40" />
                <el-table-column :label="t('redis.value')">
                  <template #default="{ $index }">
                    <el-input v-model="listEntries[$index]" size="small" type="textarea" :rows="1" autosize />
                  </template>
                </el-table-column>
                <el-table-column width="50">
                  <template #default="{ $index }">
                    <button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="listEntries.splice($index, 1)"><Trash2 :size="14" /></button>
                  </template>
                </el-table-column>
              </el-table>
              <el-button size="small" style="margin-top: 4px" @click="addListItem"><Plus :size="14" /></el-button>
            </template>

            <!-- Set Editor -->
            <template v-else-if="selectedKeyInfo?.type === 'set'">
              <el-table :data="setEntries" border size="small">
                <el-table-column type="index" label="#" width="40" />
                <el-table-column :label="t('redis.member')">
                  <template #default="{ $index }">
                    <el-input v-model="setEntries[$index]" size="small" type="textarea" :rows="1" autosize />
                  </template>
                </el-table-column>
                <el-table-column width="50">
                  <template #default="{ $index }">
                    <button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="setEntries.splice($index, 1)"><Trash2 :size="14" /></button>
                  </template>
                </el-table-column>
              </el-table>
              <el-button size="small" style="margin-top: 4px" @click="setEntries.push('')"><Plus :size="14" /></el-button>
            </template>

            <!-- ZSet Editor -->
            <template v-else-if="selectedKeyInfo?.type === 'zset'">
              <el-table :data="zsetEntries" border size="small">
                <el-table-column type="index" label="#" width="40" />
                <el-table-column :label="t('redis.member')">
                  <template #default="{ $index }">
                    <el-input v-model="zsetEntries[$index].member" size="small" type="textarea" :rows="1" autosize />
                  </template>
                </el-table-column>
                <el-table-column :label="t('redis.score')" width="100">
                  <template #default="{ $index }">
                    <el-input-number v-model="zsetEntries[$index].score" size="small" controls-position="right" style="width: 100%" />
                  </template>
                </el-table-column>
                <el-table-column width="50">
                  <template #default="{ $index }">
                    <button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="zsetEntries.splice($index, 1)"><Trash2 :size="14" /></button>
                  </template>
                </el-table-column>
              </el-table>
              <el-button size="small" style="margin-top: 4px" @click="zsetEntries.push({ member: '', score: 0 })"><Plus :size="14" /></el-button>
            </template>

            <!-- Action buttons -->
            <div class="value-actions">
              <el-button type="primary" size="small" @click="onSave" :loading="saving">{{ t('redis.save') }}</el-button>
              <el-button size="small" @click="onRevert">{{ t('redis.revert') }}</el-button>
              <el-button type="danger" size="small" @click="onDeleteKey" :loading="deleting">{{ t('redis.deleteKey') }}</el-button>
            </div>
          </div>
        </template>
        <div v-else class="redis-placeholder full">{{ t('redis.selectKey') }}</div>
      </div>
    </div>

    <!-- New Key Dialog -->
    <el-dialog append-to-body v-model="showNewKeyDialog" :title="t('redis.newKey')" width="500px">
      <el-form label-width="80px">
        <el-form-item :label="t('redis.keyName')">
          <el-input v-model="newKeyName" />
        </el-form-item>
        <el-form-item :label="t('redis.ttl')" v-if="newKeyType">
          <el-input-number v-model="newKeyTTL" :min="-1" controls-position="right" style="width: 120px" />
          <span style="font-size:11px;color:var(--text-muted);margin-left:8px">{{ t('redis.ttlHint') }}</span>
        </el-form-item>
        <el-form-item :label="t('redis.type')">
          <el-radio-group v-model="newKeyType" @change="onNewKeyTypeChange">
            <el-radio-button label="string">String</el-radio-button>
            <el-radio-button label="hash">Hash</el-radio-button>
            <el-radio-button label="list">List</el-radio-button>
            <el-radio-button label="set">Set</el-radio-button>
            <el-radio-button label="zset">ZSet</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <!-- String: value -->
        <el-form-item v-if="newKeyType === 'string'" :label="t('redis.entryValue')">
          <el-input v-model="newStringValue" type="textarea" rows="10" />
        </el-form-item>
        <!-- Hash: field/value table -->
        <template v-if="newKeyType === 'hash'">
          <el-form-item :label="t('redis.entryValue')">
            <el-table :data="newHashEntries" border size="small">
              <el-table-column prop="field" :label="t('redis.field')">
                <template #default="{ $index }"><el-input v-model="newHashEntries[$index].field" size="small" type="textarea" :rows="1" autosize /></template>
              </el-table-column>
              <el-table-column prop="value" :label="t('redis.value')">
                <template #default="{ $index }"><el-input v-model="newHashEntries[$index].value" size="small" type="textarea" :rows="1" autosize /></template>
              </el-table-column>
              <el-table-column width="50">
                <template #default="{ $index }"><button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="newHashEntries.splice($index, 1)"><Trash2 :size="14" /></button></template>
              </el-table-column>
            </el-table>
            <el-button size="small" style="margin-top: 4px" @click="newHashEntries.push({ field: '', value: '' })"><Plus :size="14" /></el-button>
          </el-form-item>
        </template>
        <!-- List: value table -->
        <template v-if="newKeyType === 'list'">
          <el-form-item :label="t('redis.entryValue')">
            <el-table :data="newListEntries" border size="small">
              <el-table-column width="26" class-name="drag-col">
                <template #default="{ $index }">
                  <span class="drag-handle" draggable="true" @dragstart="onNewListDragStart($index)" @dragover.prevent="onNewListDragOver($index)" @drop="onNewListDrop($index)"><GripVertical :size="14" /></span>
                </template>
              </el-table-column>
              <el-table-column type="index" label="#" width="40" />
              <el-table-column :label="t('redis.value')">
                <template #default="{ $index }"><el-input v-model="newListEntries[$index]" size="small" type="textarea" :rows="1" autosize /></template>
              </el-table-column>
              <el-table-column width="50">
                <template #default="{ $index }"><button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="newListEntries.splice($index, 1)"><Trash2 :size="14" /></button></template>
              </el-table-column>
            </el-table>
            <el-button size="small" style="margin-top: 4px" @click="newListEntries.push('')"><Plus :size="14" /></el-button>
          </el-form-item>
        </template>
        <!-- Set: member table -->
        <template v-if="newKeyType === 'set'">
          <el-form-item :label="t('redis.entryValue')">
            <el-table :data="newSetEntries" border size="small">
              <el-table-column type="index" label="#" width="40" />
              <el-table-column :label="t('redis.member')">
                <template #default="{ $index }"><el-input v-model="newSetEntries[$index]" size="small" type="textarea" :rows="1" autosize /></template>
              </el-table-column>
              <el-table-column width="50">
                <template #default="{ $index }"><button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="newSetEntries.splice($index, 1)"><Trash2 :size="14" /></button></template>
              </el-table-column>
            </el-table>
            <el-button size="small" style="margin-top: 4px" @click="newSetEntries.push('')"><Plus :size="14" /></el-button>
          </el-form-item>
        </template>
        <!-- ZSet: member/score table -->
        <template v-if="newKeyType === 'zset'">
          <el-form-item :label="t('redis.entryValue')">
            <el-table :data="newZSetEntries" border size="small">
              <el-table-column prop="member" :label="t('redis.member')">
                <template #default="{ $index }"><el-input v-model="newZSetEntries[$index].member" size="small" type="textarea" :rows="1" autosize /></template>
              </el-table-column>
              <el-table-column prop="score" :label="t('redis.score')" width="100">
                <template #default="{ $index }"><el-input-number v-model="newZSetEntries[$index].score" size="small" controls-position="right" style="width: 100%" /></template>
              </el-table-column>
              <el-table-column width="50">
                <template #default="{ $index }"><button class="btn btn-ghost btn-icon btn-sm danger" title="Delete" @click="newZSetEntries.splice($index, 1)"><Trash2 :size="14" /></button></template>
              </el-table-column>
            </el-table>
            <el-button size="small" style="margin-top: 4px" @click="newZSetEntries.push({ member: '', score: 0 })"><Plus :size="14" /></el-button>
          </el-form-item>
        </template>
      </el-form>
      <template #footer>
        <el-button @click="showNewKeyDialog = false">{{ t('redis.cancel') }}</el-button>
        <el-button type="primary" @click="onCreateKey">{{ t('redis.create') }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onUnmounted } from 'vue'
import { ElMessageBox } from 'element-plus'
import { msg } from '../services/message'
import { Trash2, Plus, GripVertical, RefreshCw, ChevronLeft, ChevronRight } from '@lucide/vue'
import { useI18n } from '../i18n'
import {
  RedisScanKeys,
  RedisGetKeyInfo,
  RedisGetString,
  RedisSetString,
  RedisGetHashAll,
  RedisHashSet,
  RedisHashDel,
  RedisGetListRange,
  RedisListPush,
  RedisListPop,
  RedisGetSetAll,
  RedisSetAdd,
  RedisSetRemove,
  RedisGetSortedSetRange,
  RedisZSetAdd,
  RedisZSetRemove,
  RedisDeleteKey,
  RedisSetKeyTTL,
  RedisSwitchDB,
  RedisKeyspaceInfo,
} from '../../wailsjs/go/main/App'
import type { RedisKeyInfo, FieldEntry, ScoredMember, ScanResult } from '../types/redis'

const props = defineProps<{ sessionId: string }>()
const { t } = useI18n()

// --- Resize ---
const leftWidth = ref(280)
const resizing = ref(false)
let resizeStartX = 0
let resizeStartWidth = 0

function onResizeStart(e: MouseEvent) {
  resizeStartX = e.clientX
  resizeStartWidth = leftWidth.value
  resizing.value = true
  document.addEventListener('mousemove', onResizeMove)
  document.addEventListener('mouseup', onResizeEnd)
}
function onResizeMove(e: MouseEvent) {
  const dx = e.clientX - resizeStartX
  leftWidth.value = Math.max(180, Math.min(500, resizeStartWidth + dx))
}
function onResizeEnd() {
  resizing.value = false
  document.removeEventListener('mousemove', onResizeMove)
  document.removeEventListener('mouseup', onResizeEnd)
}
onUnmounted(() => {
  if (resizing.value) {
    document.removeEventListener('mousemove', onResizeMove)
    document.removeEventListener('mouseup', onResizeEnd)
  }
})

// --- Key list state ---
const keys = ref<RedisKeyInfo[]>([])
const selectedKey = ref('')
const selectedKeyInfo = ref<RedisKeyInfo | null>(null)
const loading = ref(false)
const scanPattern = ref('*')
const currentDb = ref(0)
const dbSizes = ref<number[]>(new Array(16).fill(-1))

// Pagination
const pageSize = ref(100)
const pageSizes = [50, 100, 200, 500]
const currentPage = ref(1)
const cursorStack = ref<number[]>([])
const nextCursor = ref(0)
const hasMore = ref(true)

// Value editing
const editTTL = ref(-1)
const stringValue = ref('')
const hashEntries = ref<FieldEntry[]>([])
const listEntries = ref<string[]>([])
const setEntries = ref<string[]>([])
const zsetEntries = ref<ScoredMember[]>([])
const keyLoading = ref(false)
const saving = ref(false)
const deleting = ref(false)

// New key dialog
const showNewKeyDialog = ref(false)
const newKeyName = ref('')
const newKeyType = ref('string')
const newKeyTTL = ref(-1)
const newStringValue = ref('')
const newHashEntries = ref<FieldEntry[]>([])
const newListEntries = ref<string[]>([])
const newSetEntries = ref<string[]>([])
const newZSetEntries = ref<ScoredMember[]>([])

function resetNewKeyForm() {
  newKeyName.value = ''
  newKeyType.value = 'string'
  newKeyTTL.value = -1
  newStringValue.value = ''
  newHashEntries.value = [{ field: '', value: '' }]
  newListEntries.value = ['']
  newSetEntries.value = ['']
  newZSetEntries.value = [{ member: '', score: 0 }]
}
function onNewKeyTypeChange() {
  newKeyTTL.value = -1
  newStringValue.value = ''
  newHashEntries.value = [{ field: '', value: '' }]
  newListEntries.value = ['']
  newSetEntries.value = ['']
  newZSetEntries.value = [{ member: '', score: 0 }]
}

// --- Key scanning ---
async function doScan(cursor: number) {
  loading.value = true
  try {
    const result: ScanResult = await RedisScanKeys(props.sessionId, scanPattern.value, cursor, pageSize.value)
    keys.value = (result.keys || []).sort((a, b) => a.name.localeCompare(b.name))
    nextCursor.value = result.cursor
    hasMore.value = result.cursor !== 0
  } catch (e: any) {
    msg.error(`Scan failed: ${e?.message || e}`)
    keys.value = []
  } finally {
    loading.value = false
  }
}
async function onScan() {
  cursorStack.value = []
  currentPage.value = 1
  await doScan(0)
}
async function onNextPage() {
  if (nextCursor.value === 0) return
  cursorStack.value.push(nextCursor.value)
  currentPage.value++
  await doScan(nextCursor.value)
}
async function onPrevPage() {
  const prev = cursorStack.value.pop()
  currentPage.value = Math.max(1, currentPage.value - 1)
  await doScan(prev ?? 0)
}
async function onPageSizeChange() {
  cursorStack.value = []
  currentPage.value = 1
  await doScan(0)
}
async function fetchDBSize() {
  try {
    const info = await RedisKeyspaceInfo(props.sessionId)
    for (let i = 0; i < 16; i++) {
      dbSizes.value[i] = info[i] ?? 0
    }
  } catch (_) { /* ignore */ }
}

// --- DB switching ---
async function onSwitchDB(idx: number) {
  try {
    await RedisSwitchDB(props.sessionId, idx)
    currentDb.value = idx
    cursorStack.value = []
    currentPage.value = 1
    await fetchDBSize()
    await doScan(0)
  } catch (e: any) {
    msg.error(`Switch DB failed: ${e?.message || e}`)
  }
}

// --- Key selection ---
async function onSelectKey(info: RedisKeyInfo) {
  selectedKey.value = info.name
  keyLoading.value = true
  try {
    const fresh = await RedisGetKeyInfo(props.sessionId, info.name)
    if (fresh) { selectedKeyInfo.value = fresh; editTTL.value = fresh.ttl }
    else { selectedKeyInfo.value = info; editTTL.value = info.ttl }
  } catch (_) { selectedKeyInfo.value = info; editTTL.value = info.ttl }
  try {
    switch (info.type) {
      case 'string': { const val = await RedisGetString(props.sessionId, info.name); stringValue.value = val || ''; break }
      case 'hash': { const entries = await RedisGetHashAll(props.sessionId, info.name); hashEntries.value = entries || []; break }
      case 'list': { const items = await RedisGetListRange(props.sessionId, info.name, 0, -1); listEntries.value = items || []; break }
      case 'set': { const members = await RedisGetSetAll(props.sessionId, info.name); setEntries.value = members || []; break }
      case 'zset': { const members = await RedisGetSortedSetRange(props.sessionId, info.name, '-inf', '+inf'); zsetEntries.value = members || []; break }
    }
  } catch (e: any) { msg.error(`Load key failed: ${e?.message || e}`) }
  finally { keyLoading.value = false }
}

// --- Save ---
async function onSave() {
  saving.value = true
  try {
    const key = selectedKey.value
    const t = selectedKeyInfo.value?.type
    switch (t) {
      case 'string': await RedisSetString(props.sessionId, key, stringValue.value); break
      case 'hash': {
        const current = await RedisGetHashAll(props.sessionId, key)
        const toDelete = current.filter(e => !hashEntries.value.find(h => h.field === e.field))
        if (toDelete.length > 0) await RedisHashDel(props.sessionId, key, toDelete.map(e => e.field))
        for (const e of hashEntries.value) { if (e.field) await RedisHashSet(props.sessionId, key, e.field, e.value) }
        break
      }
      case 'list': {
        const existing = await RedisGetListRange(props.sessionId, key, 0, -1)
        for (let i = 0; i < existing.length; i++) { await RedisListPop(props.sessionId, key, 'right') }
        if (listEntries.value.length > 0) await RedisListPush(props.sessionId, key, 'right', listEntries.value)
        break
      }
      case 'set': {
        const current = await RedisGetSetAll(props.sessionId, key)
        const r = current.filter(m => !setEntries.value.includes(m))
        const a = setEntries.value.filter(m => !current.includes(m))
        if (r.length > 0) await RedisSetRemove(props.sessionId, key, r)
        if (a.length > 0) await RedisSetAdd(props.sessionId, key, a)
        break
      }
      case 'zset': {
        const current = await RedisGetSortedSetRange(props.sessionId, key, '-inf', '+inf')
        const r = current.filter(m => !zsetEntries.value.find(z => z.member === m.member))
        if (r.length > 0) await RedisZSetRemove(props.sessionId, key, r.map(m => m.member))
        if (zsetEntries.value.length > 0) await RedisZSetAdd(props.sessionId, key, zsetEntries.value)
        break
      }
    }
    msg.success(t('redis.saved'))
    const info = await RedisGetKeyInfo(props.sessionId, key)
    if (info) { selectedKeyInfo.value = info; editTTL.value = info.ttl }
  } catch (e: any) { msg.error(`Save failed: ${e?.message || e}`) }
  finally { saving.value = false }
}

// --- TTL ---
async function onSetTTL() {
  if (!selectedKey.value) return
  try {
    await RedisSetKeyTTL(props.sessionId, selectedKey.value, editTTL.value)
    msg.success(t('redis.ttlUpdated'))
    const info = await RedisGetKeyInfo(props.sessionId, selectedKey.value)
    if (info) selectedKeyInfo.value = info
  } catch (e: any) { msg.error(`Set TTL failed: ${e?.message || e}`) }
}

// --- Revert ---
async function onRevert() {
  if (!selectedKeyInfo.value) return
  await onSelectKey(selectedKeyInfo.value)
}

// --- Delete key ---
async function onDeleteKey() {
  try {
    await ElMessageBox.confirm(t('redis.confirmDelete', { key: selectedKey.value }), t('redis.delete'), { confirmButtonText: t('redis.delete'), cancelButtonText: t('redis.cancel'), type: 'warning' })
  } catch (_) { return }
  deleting.value = true
  try {
    await RedisDeleteKey(props.sessionId, selectedKey.value)
    msg.success('Key deleted')
    selectedKey.value = ''
    selectedKeyInfo.value = null
    await doScan(cursorStack.value.length > 0 ? cursorStack.value[cursorStack.value.length - 1] : 0)
  } catch (e: any) { msg.error(`Delete failed: ${e?.message || e}`) }
  finally { deleting.value = false }
}

// --- New key ---
function onShowNewKeyDialog() { resetNewKeyForm(); showNewKeyDialog.value = true }
async function onCreateKey() {
  if (!newKeyName.value.trim()) { msg.warning(t('redis.keyNameRequired')); return }
  try {
    switch (newKeyType.value) {
      case 'string': await RedisSetString(props.sessionId, newKeyName.value, newStringValue.value); break
      case 'hash': {
        for (const e of newHashEntries.value) { if (e.field) await RedisHashSet(props.sessionId, newKeyName.value, e.field, e.value) }
        if (!newHashEntries.value.some(e => e.field)) await RedisHashSet(props.sessionId, newKeyName.value, 'field', '')
        break
      }
      case 'list': {
        const items = newListEntries.value.filter(s => s.trim())
        if (items.length > 0) await RedisListPush(props.sessionId, newKeyName.value, 'right', items)
        else await RedisListPush(props.sessionId, newKeyName.value, 'right', [''])
        break
      }
      case 'set': {
        const members = newSetEntries.value.map(s => s.trim()).filter(s => s)
        if (members.length > 0) await RedisSetAdd(props.sessionId, newKeyName.value, members)
        else await RedisSetAdd(props.sessionId, newKeyName.value, ['member'])
        break
      }
      case 'zset': {
        const entries = newZSetEntries.value.filter(e => e.member)
        if (entries.length > 0) await RedisZSetAdd(props.sessionId, newKeyName.value, entries)
        else await RedisZSetAdd(props.sessionId, newKeyName.value, [{ member: 'member', score: 0 }])
        break
      }
    }
    if (newKeyTTL.value >= 0) { await RedisSetKeyTTL(props.sessionId, newKeyName.value, newKeyTTL.value) }
    showNewKeyDialog.value = false
    msg.success(t('redis.keyCreated'))
    await doScan(cursorStack.value.length > 0 ? cursorStack.value[cursorStack.value.length - 1] : 0)
  } catch (e: any) { msg.error(`Create key failed: ${e?.message || e}`) }
}

// --- List drag reorder ---
let dragIdx = -1
function onListDragStart(idx: number) { dragIdx = idx }
function onListDragOver(idx: number) {
  if (dragIdx === -1 || dragIdx === idx) return
  const item = listEntries.value.splice(dragIdx, 1)[0]
  listEntries.value.splice(idx, 0, item)
  dragIdx = idx
}
function onListDrop(_idx: number) { dragIdx = -1 }
function addListItem() { dragIdx = -1; listEntries.value.push('') }

let newDragIdx = -1
function onNewListDragStart(idx: number) { newDragIdx = idx }
function onNewListDragOver(idx: number) {
  if (newDragIdx === -1 || newDragIdx === idx) return
  const item = newListEntries.value.splice(newDragIdx, 1)[0]
  newListEntries.value.splice(idx, 0, item)
  newDragIdx = idx
}
function onNewListDrop(_idx: number) { newDragIdx = -1 }

// --- Init ---
let initialised = false
watch(() => props.sessionId, async (newId) => {
  if (newId && !initialised) { initialised = true; await fetchDBSize(); await doScan(0) }
})
</script>

<style scoped>
.redis-tab-content {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.redis-main {
  flex: 1;
  display: flex;
  overflow: hidden;
}
.redis-left {
  flex-shrink: 0;
  display: flex;
  flex-direction: column;
  border-right: 1px solid var(--border-subtle);
  overflow: hidden;
}
.redis-resizer {
  width: 4px;
  cursor: col-resize;
  background: transparent;
  flex-shrink: 0;
  transition: background 0.15s ease;
}
.redis-resizer:hover {
  background: var(--border-subtle);
}
.redis-right {
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.redis-toolbar {
  display: flex;
  gap: 4px;
  align-items: center;
  padding: 8px;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
}
.redis-key-list {
  flex: 1;
  overflow-y: auto;
}
.key-item {
  padding: 6px 10px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 6px;
  font-family: var(--font-ui);
  font-size: 12px;
  color: var(--text-primary);
  transition: background 0.12s ease;
  user-select: none;
}
.key-item:hover { background: var(--bg-hover); }
.key-item.selected { background: var(--bg-hover); color: var(--accent); }
.key-type-badge {
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
  background: var(--bg-hover);
  padding: 1px 4px;
  border-radius: var(--radius-sm);
  min-width: 42px;
  text-align: center;
  flex-shrink: 0;
}
.key-name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: none;
}
.redis-pagination {
  display: flex;
  align-items: center;
  gap: 4px;
  padding: 6px 8px;
  border-top: 1px solid var(--border-subtle);
  flex-shrink: 0;
  font-family: var(--font-ui);
  font-size: 12px;
  color: var(--text-secondary);
}
.page-btn {
  border: 1px solid var(--border-subtle);
  background: var(--bg-base);
  color: var(--text-secondary);
  cursor: pointer;
  padding: 2px 4px;
  border-radius: var(--radius-sm);
  font-size: 12px;
  font-family: var(--font-ui);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
.page-btn:hover:not(:disabled) { background: var(--bg-hover); color: var(--text-primary); }
.page-btn:disabled { opacity: 0.4; cursor: default; }
.page-num {
  min-width: 20px;
  text-align: center;
}
.key-meta {
  padding: 8px 12px 12px;
  border-bottom: 1px solid var(--border-subtle);
  flex-shrink: 0;
  font-family: var(--font-ui);
  font-size: 12px;
}
.meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 8px 0;
}
.meta-label {
  color: var(--text-muted);
  min-width: 32px;
}
.meta-value {
  color: var(--text-primary);
  font-family: var(--font-mono);
  user-select: text;
  word-break: break-all;
  overflow-wrap: break-word;
}
.ttl-row {
  display: flex;
  align-items: center;
  gap: 6px;
}
.redis-right-content {
  flex: 1;
  overflow-y: auto;
  padding: 8px;
}
.value-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
.redis-placeholder {
  color: var(--text-secondary);
  font-family: var(--font-ui);
  font-size: 14px;
  padding: 16px;
}
.redis-placeholder.full {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
}
/* .btn.btn-ghost.btn-icon(.danger) now supplies the base look; keep only
   Redis-specific overrides here (font-size for icon glyphs, table-cell reset). */
.btn-icon.is-disabled,
.btn-icon:disabled { opacity: 0.3; cursor: default; }
.btn-icon:disabled:hover { color: var(--text-secondary); background: none; }
:deep(.el-table .btn-icon) { background: none; }
.drag-handle {
  cursor: grab;
  color: var(--text-muted);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
}
.drag-handle:active { cursor: grabbing; }
.drag-handle:hover { color: var(--text-primary); }
:deep(.drag-col) { overflow: visible !important; text-align: center !important; vertical-align: middle !important; }
:deep(.drag-col .cell) { overflow: visible !important; padding: 0 !important; }
</style>
