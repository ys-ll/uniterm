<template>
  <div
    ref="gridRootEl"
    class="db-result-grid"
    tabindex="0"
    @mousedown="onGridMousedown"
    @keydown="onGridKeydown"
  >
    <el-table
      :key="tableKey"
      ref="tableRef"
      :data="rows"
      border
      size="small"
      height="100%"
      style="width: 100%"
      class="db-result-table"
      :empty-text="emptyText"
      :cell-class-name="cellClassName"
      :row-class-name="rowClassName"
      :header-cell-class-name="headerCellClassName"
      @cell-dblclick="onCellDblClick"
      @cell-contextmenu="onCellContextMenu"
    >
      <!-- 行号列：el-table 原生 index 列；点击选整行、拖动选多行，
           点击表头全选（Excel 角标行为）。 -->
      <el-table-column
        type="index"
        fixed="left"
        :width="uiPx(48)"
        class-name="db-rownum-col"
        :resizable="false"
      >
        <template #header>
          <div class="db-rownum-head" data-db-headall :title="t('db.selectAll')" />
        </template>
        <template #default="{ $index }">
          <div class="db-rownum-cell" :data-db-rownum="true" :data-db-r="$index">{{ $index + 1 }}</div>
        </template>
      </el-table-column>

      <el-table-column
        v-for="(col, colIdx) in columns"
        :key="col.name"
        :prop="col.name"
        :min-width="uiPx(110)"
        show-overflow-tooltip
      >
        <template #header>
          <div class="db-col-head" :data-db-col="colIdx">{{ colTitle(col) }}</div>
        </template>
        <template #default="{ row, $index }">
          <div class="db-cell-box" :data-db-r="$index" :data-db-c="colIdx">
            <div
              v-if="canEdit && editingRowIdx === $index && editingField === col.name"
              class="cell-edit-wrap"
              @mousedown.stop
            >
              <input
                ref="cellInputEl"
                v-model="draftValue"
                class="cell-edit-input"
                :placeholder="isNullable(col.name) ? t('db.null') : ''"
                @input="onDraftInput"
                @keydown.enter.prevent="onCellEditConfirm"
                @keydown.escape.prevent="onCellEditCancel"
                @blur="onCellEditConfirm"
              />
              <button
                v-if="isNullable(col.name)"
                type="button"
                class="null-btn"
                title="N"
                @mousedown.prevent
                @click.prevent="setDraftNull"
              >
                N
              </button>
            </div>
            <span
              v-else-if="row[col.name] === null"
              class="cell-null"
              :class="{ 'cell-pending': isPending(row, col.name) }"
            >{{ t('db.null') }}</span>
            <span
              v-else
              class="cell-value"
              :class="{ 'cell-pending': isPending(row, col.name) }"
            >{{ formatCell(row[col.name]) }}</span>
            <!-- 选中单元格右侧的放大按钮：长文本/多行内容在弹窗编辑器里查看、编辑。 -->
            <button
              v-if="!isEditing($index, col.name) && isActiveCell($index, col.name)"
              type="button"
              class="cell-expand-btn"
              :title="t('db.expandCell')"
              @mousedown.stop
              @click.stop.prevent="openCellEditor($index, col.name)"
            >
              <Maximize2 :size="12" />
            </button>
          </div>
        </template>
      </el-table-column>

      <el-table-column
        v-if="canEdit"
        :label="actionsLabel"
        :width="uiPx(100)"
        fixed="right"
        class-name="db-action-cell"
        :resizable="false"
      >
        <template #default="{ row }">
          <div class="row-actions" @mousedown.stop @click.stop>
            <button
              type="button"
              class="btn btn-ghost btn-icon btn-sm"
              :title="editLabel"
              @click.stop.prevent="emit('edit-row', row)"
            >
              <Pencil :size="'0.875rem'" />
            </button>
            <button
              type="button"
              class="btn btn-ghost btn-icon btn-sm"
              :title="duplicateLabel"
              @click.stop.prevent="emit('duplicate-row', row)"
            >
              <Copy :size="'0.875rem'" />
            </button>
            <button
              type="button"
              class="btn btn-ghost btn-icon btn-sm danger"
              :title="deleteLabel"
              @click.stop.prevent="emit('delete-row', row)"
            >
              <Trash2 :size="'0.875rem'" />
            </button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <!-- 网格右键菜单：复制 / 编辑 / 行 三组平铺，按上下文（格/行/列）禁用 -->
    <Menu ref="cellMenuRef" v-model:visible="cellMenuVisible">
      <MenuItem :class="{ disabled: !selRanges.length }" @click="copySelectionTsv">
        {{ t('db.copySelected') }}
      </MenuItem>
      <MenuItem :class="{ disabled: !activeCell || ctxKind === 'cols' }" @click="copyActiveRow">
        {{ t('db.copyRow') }}
      </MenuItem>
      <MenuItem :class="{ disabled: !activeCell }" @click="copyActiveColumnName">
        {{ t('db.copyColumnName') }}
      </MenuItem>
      <MenuDivider />
      <MenuItem :class="{ disabled: !activeCell || !canEdit || ctxKind === 'cols' }" @click="ctxInlineEdit">
        {{ t('db.inlineEdit') }}
      </MenuItem>
      <MenuItem :class="{ disabled: !activeCell || ctxKind === 'cols' }" @click="ctxOpenEditor">
        {{ t('db.expandCell') }}
      </MenuItem>
      <MenuItem :class="{ disabled: !activeCell || !canEdit || ctxKind !== 'cells' || !isNullable(activeColName) }" @click="ctxSetNull">
        {{ t('db.setNull') }}
      </MenuItem>
      <MenuItem :class="{ disabled: !activeCell || !canEdit || ctxKind !== 'cells' }" @click="ctxClearValue">
        {{ t('db.clearValue') }}
      </MenuItem>
      <MenuDivider />
      <MenuItem :class="{ disabled: !canEdit }" @click="ctxInsertRow">
        {{ t('db.insertRow') }}
      </MenuItem>
      <MenuItem :class="{ disabled: !activeCell || !canEdit || ctxKind === 'cols' }" @click="ctxDuplicateRow">
        {{ t('db.duplicateRow') }}
      </MenuItem>
      <MenuItem class="danger" :class="{ disabled: !selRanges.length || !canEdit }" @click="ctxDeleteRows">
        {{ t('db.deleteSelectedRows') }}
      </MenuItem>
    </Menu>

    <!-- 单元格放大编辑弹窗：多行/长文本用完整编辑器查看与编辑。 -->
    <DBCellEditorDialog
      ref="cellEditorRef"
      :visible="cellEditorVisible"
      @update:visible="(v: boolean) => (cellEditorVisible = v)"
      @save="onCellEditorSave"
    />
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, reactive, shallowReactive, shallowRef, ref, watch } from 'vue'
import { Pencil, Copy, Trash2, Maximize2 } from '@lucide/vue'
import MenuDivider from './MenuDivider.vue'
import type { QueryResultColumn, ColumnInfo } from '../types/database'
import { useI18n } from '../i18n'
import { uiPx } from '../utils/uiScale'
import { writeClipboard } from '../composables/useClipboardWrite'
import Menu from './Menu.vue'
import MenuItem from './MenuItem.vue'
import DBCellEditorDialog from './DBCellEditorDialog.vue'


const { t } = useI18n()

const props = defineProps<{
  rows: Record<string, any>[]
  columns: QueryResultColumn[]
  canEdit: boolean
  primaryKeys?: string[]
  tableColumns?: ColumnInfo[]
  emptyText?: string
  actionsLabel?: string
  editLabel?: string
  duplicateLabel?: string
  deleteLabel?: string
}>()

const emit = defineEmits<{
  /** A staged, unsaved cell edit changed; payload = number of pending edits. */
  'pending-change': [count: number]
  'edit-row': [row: Record<string, any>]
  'duplicate-row': [row: Record<string, any>]
  'delete-row': [row: Record<string, any>]
  'editing-change': [editing: boolean]
  'insert-row': []
  'delete-rows': [rows: Record<string, any>[]]
}>()

const tableRef = ref<any>(null)
const cellInputEl = ref<HTMLInputElement | HTMLInputElement[] | null>(null)


const tableKey = computed(() => `edit-${props.canEdit ? 1 : 0}-cols-${props.columns.map(c => c.name).join('|')}`)

// ── 选择模型（Excel 风格多选）──
// 单击选格、拖动画矩形；行号列点击选整行、拖动选多行；列头点击选整列、
// 拖动选多列；序号列表头全选。Ctrl 加选（跳跃多块）、Shift 从锚点扩选。
// 选区以 (rowIndex, colIndex) 归一化矩形存储，整行/整列用标记区分
// （行号/列头高亮与拖动扩展方向依赖该标记）。
interface SelRange { r1: number; c1: number; r2: number; c2: number; fullRow: boolean; fullCol: boolean }
type HitKind = 'cells' | 'rows' | 'cols' | 'all'
interface Anchor { r: number; c: number; kind: HitKind }

const gridRootEl = ref<HTMLElement | null>(null)
const selRanges = shallowRef<SelRange[]>([])
const activeCell = shallowRef<{ r: number; c: number } | null>(null)
const selAnchor = shallowRef<Anchor | null>(null)

const colIndexByName = computed(() => {
  const m = new Map<string, number>()
  props.columns.forEach((c, i) => m.set(c.name, i))
  return m
})

function clearSelection() {
  selRanges.value = []
  activeCell.value = null
  selAnchor.value = null
}

/** 归一化矩形（r1<=r2、c1<=c2）。空列/空行时保持原样以产生空区间。 */
function rectRange(r1: number, c1: number, r2: number, c2: number): SelRange {
  if (r2 < 0 || c2 < 0) return { r1, c1, r2, c2, fullRow: false, fullCol: false }
  return normRange({ r1, c1, r2, c2, fullRow: false, fullCol: false })
}
function normRange(r: SelRange): SelRange {
  return {
    r1: Math.min(r.r1, r.r2), r2: Math.max(r.r1, r.r2),
    c1: Math.min(r.c1, r.c2), c2: Math.max(r.c1, r.c2),
    fullRow: r.fullRow, fullCol: r.fullCol,
  }
}
/** 整行选区（覆盖当前所有列）。 */
function fullRowSpan(r1: number, r2: number): SelRange {
  const last = props.columns.length - 1
  const rng = { r1, c1: 0, r2, c2: last, fullRow: true, fullCol: false }
  return last >= 0 ? normRange(rng) : rng
}
/** 整列选区（覆盖当前所有行）。 */
function fullColSpan(c1: number, c2: number): SelRange {
  const last = props.rows.length - 1
  const rng = { r1: 0, c1, r2: last, c2, fullRow: false, fullCol: true }
  return last >= 0 ? normRange(rng) : rng
}

function inSelection(r: number, c: number): boolean {
  for (const s of selRanges.value) {
    if (r >= s.r1 && r <= s.r2 && c >= s.c1 && c <= s.c2) return true
  }
  return false
}

function isActiveCell(rowIndex: number, field: string): boolean {
  const a = activeCell.value
  // 行匹配用下标：el-table 内部 store 的 data 是 ref 深层响应式代理，
  // 插槽 row 是代理对象，与 props.rows 的原始对象 !==。
  return !!a && a.r === rowIndex && props.columns[a.c]?.name === field
}

/**
 * 命中检测：把指针位置换算成网格坐标。
 * 单元格内容带 data-db-r/data-db-c、行号带 data-db-rownum、列头带
 * data-db-col / data-db-headall。用 elementsFromPoint 逐层找：悬浮层
 * （溢出提示 popper 等）不在网格内，跳过继续；点击落在 th/td 的 padding
 * 死区时 closest 向上找不到标记，需向内部 querySelector 兜底；操作列等
 * 无标记的 td 到此为止，避免穿透到 sticky 固定列遮住的下一层行。
 */
function hitTest(x: number, y: number): { kind: HitKind; r?: number; c?: number } | null {
  const root = gridRootEl.value
  if (!root) return null
  for (const el of document.elementsFromPoint(x, y)) {
    if (!root.contains(el)) continue
    const headAll = el.closest('[data-db-headall]')
      ?? (el.tagName === 'TH' ? el.querySelector('[data-db-headall]') : null)
    if (headAll) return { kind: 'all' }
    const head = el.closest('[data-db-col]')
      ?? (el.tagName === 'TH' ? el.querySelector('[data-db-col]') : null)
    if (head) return { kind: 'cols', c: Number(head.getAttribute('data-db-col')) }
    const num = el.closest('[data-db-rownum]')
      ?? (el.tagName === 'TD' ? el.querySelector('[data-db-rownum]') : null)
    if (num) return { kind: 'rows', r: Number(num.getAttribute('data-db-r')) }
    const cell = el.closest('[data-db-c]')
      ?? (el.tagName === 'TD' ? el.querySelector('[data-db-c]') : null)
    if (cell) {
      return {
        kind: 'cells',
        r: Number(cell.getAttribute('data-db-r')),
        c: Number(cell.getAttribute('data-db-c')),
      }
    }
    if (el.tagName === 'TD' || el.tagName === 'TH') return null
  }
  return null
}

function applySelect(kind: HitKind, r: number, c: number, extend: boolean, additive: boolean) {
  const rows = props.rows
  const cols = props.columns
  const rc = Math.min(Math.max(r, 0), rows.length - 1)
  const cc = Math.min(Math.max(c, 0), cols.length - 1)

  // 全选（序号列表头）。
  if (kind === 'all') {
    if (!rows.length || !cols.length) return
    selRanges.value = [normRange({ r1: 0, c1: 0, r2: rows.length - 1, c2: cols.length - 1, fullRow: true, fullCol: true })]
    activeCell.value = { r: 0, c: 0 }
    selAnchor.value = { r: 0, c: 0, kind: 'cells' }
    return
  }

  // Shift 扩选：从锚点画到当前格，保持锚点的选择类型（整行/整列/矩形）。
  if (extend && selAnchor.value) {
    const a = selAnchor.value
    const range =
      a.kind === 'rows' ? fullRowSpan(Math.min(a.r, rc), Math.max(a.r, rc))
        : a.kind === 'cols' ? fullColSpan(Math.min(a.c, cc), Math.max(a.c, cc))
          : rectRange(Math.min(a.r, rc), Math.min(a.c, cc), Math.max(a.r, rc), Math.max(a.c, cc))
    const ranges = selRanges.value.length ? [...selRanges.value] : []
    ranges[ranges.length - 1] = range
    selRanges.value = ranges
    activeCell.value = { r: rc, c: cc }
    return
  }

  // Ctrl 加选保留旧选区；普通点击替换为单块新选区。
  const ranges = additive ? [...selRanges.value] : []
  ranges.push(
    kind === 'rows' ? fullRowSpan(rc, rc)
      : kind === 'cols' ? fullColSpan(cc, cc)
        : rectRange(rc, cc, rc, cc),
  )
  selRanges.value = ranges
  // 整列选择的主动格落在首行，整行选择落在首列（与 Excel 一致）。
  const ar = kind === 'cols' ? 0 : rc
  const ac = kind === 'rows' ? 0 : cc
  activeCell.value = { r: ar, c: ac }
  selAnchor.value = { r: rc, c: cc, kind }
}

// 拖动扩展：mousemove 期间持续把最后一块选区的终点改到指针位置。
// 行/列拖动允许扫回数据区（cells 命中同样提供 r/c），方向不丢。
// mousemove 每秒可触发几十次：坐标暂存、requestAnimationFrame 合帧，
// 且选区范围没变就不写状态，避免大表被无谓重渲染拖出卡顿。
let dragKind: HitKind | null = null
let dragFrame = 0
let dragPending: { x: number; y: number } | null = null

function startDrag(kind: HitKind) {
  dragKind = kind
  document.addEventListener('mousemove', onDragMove)
  document.addEventListener('mouseup', onDragEnd)
}
function onDragMove(e: MouseEvent) {
  dragPending = { x: e.clientX, y: e.clientY }
  if (!dragFrame) dragFrame = requestAnimationFrame(applyDragMove)
}
function applyDragMove() {
  dragFrame = 0
  const pos = dragPending
  dragPending = null
  if (!pos || !dragKind || !selAnchor.value) return
  const hit = hitTest(pos.x, pos.y)
  if (!hit || hit.kind === 'all') return
  const a = selAnchor.value
  const rows = props.rows
  const cols = props.columns
  const rc = Math.min(Math.max(hit.r ?? 0, 0), rows.length - 1)
  const cc = Math.min(Math.max(hit.c ?? 0, 0), cols.length - 1)
  let range: SelRange
  if (dragKind === 'rows') {
    if (hit.kind !== 'rows' && hit.kind !== 'cells') return
    range = fullRowSpan(Math.min(a.r, rc), Math.max(a.r, rc))
  } else if (dragKind === 'cols') {
    if (hit.kind !== 'cols' && hit.kind !== 'cells') return
    range = fullColSpan(Math.min(a.c, cc), Math.max(a.c, cc))
  } else {
    if (hit.kind !== 'cells') return
    range = rectRange(Math.min(a.r, rc), Math.min(a.c, cc), Math.max(a.r, rc), Math.max(a.c, cc))
  }
  const ranges = selRanges.value.length ? [...selRanges.value] : []
  const last = ranges[ranges.length - 1]
  if (last && last.r1 === range.r1 && last.r2 === range.r2 && last.c1 === range.c1 && last.c2 === range.c2) return
  ranges[ranges.length - 1] = range
  selRanges.value = ranges
}
function onDragEnd() {
  dragKind = null
  if (dragFrame) { cancelAnimationFrame(dragFrame); dragFrame = 0 }
  dragPending = null
  document.removeEventListener('mousemove', onDragMove)
  document.removeEventListener('mouseup', onDragEnd)
}

/** 网格级 mousedown 委托：单元格/行号/列头统一在这里起选、起拖。 */
function onGridMousedown(e: MouseEvent) {
  if (e.button !== 0) return
  // 行内编辑中不抢事件：全局 outside-mousedown 会先确认草稿。
  if (editingRowIdx.value !== null) return
  const hit = hitTest(e.clientX, e.clientY)
  if (!hit) return
  e.preventDefault()
  // 容器获得焦点，keydown（Ctrl+C）才能路由到这里；不抢编辑态 input 的焦点。
  gridRootEl.value?.focus({ preventScroll: true })
  applySelect(hit.kind, hit.r ?? 0, hit.c ?? 0, e.shiftKey, e.ctrlKey || e.metaKey)
  if (hit.kind !== 'all') startDrag(hit.kind)
}

/** 网格右键：格/行/列/全选统一处理——先按 Excel 语义选中目标，再打开菜单。
 *  坐标用 hitTest 解析——事件参数里的 row 是 el-table 的代理对象，不能
 *  用 props.rows.indexOf 找下标（永远 -1，右键菜单会静默失效）。 */
function onCellContextMenu(
  _row: Record<string, any>,
  _column: { property?: string },
  _cell: HTMLElement,
  event: MouseEvent,
) {
  const hit = hitTest(event.clientX, event.clientY)
  if (!hit) return
  event.preventDefault()
  event.stopPropagation()
  const r = hit.r ?? 0
  const c = hit.c ?? 0
  ctxKind.value = hit.kind
  if (hit.kind === 'all') {
    applySelect('all', 0, 0, false, false)
  } else if (!inSelection(r, c)) {
    // 不在选区内：按目标类型单独选中（格/整行/整列）。
    selRanges.value = [
      hit.kind === 'rows' ? fullRowSpan(r, r)
        : hit.kind === 'cols' ? fullColSpan(c, c)
          : rectRange(r, c, r, c),
    ]
    activeCell.value = { r: hit.kind === 'cols' ? 0 : r, c: hit.kind === 'rows' ? 0 : c }
    selAnchor.value = { r, c, kind: hit.kind }
  }
  openCellMenu(event.clientX, event.clientY)
}

// 右键目标类型：菜单项按它禁用（列上下文里行级操作无意义）。
const ctxKind = ref<HitKind | null>(null)

const activeColName = computed(() => {
  const a = activeCell.value
  return a ? props.columns[a.c]?.name ?? '' : ''
})

/** 当前选区覆盖的所有行下标（去重升序）。 */
function selectedRowIndices(): number[] {
  const set = new Set<number>()
  for (const s of selRanges.value) {
    for (let r = s.r1; r <= s.r2; r++) {
      if (r >= 0 && r < props.rows.length) set.add(r)
    }
  }
  return [...set].sort((a, b) => a - b)
}

function ctxInlineEdit() {
  cellMenuVisible.value = false
  const a = activeCell.value
  if (!a || !props.canEdit || ctxKind.value === 'cols') return
  const field = props.columns[a.c]?.name
  if (field) startEdit(a.r, field)
}

function ctxOpenEditor() {
  cellMenuVisible.value = false
  const a = activeCell.value
  if (!a || ctxKind.value === 'cols') return
  const field = props.columns[a.c]?.name
  if (field) openCellEditor(a.r, field)
}

/** 右键「设为 NULL / 清空」：与行内编辑同一条暂存通路（stageEdit 会
 *  保留首次暂存的原始值，重复操作可回退）。 */
function ctxSetNull() {
  cellMenuVisible.value = false
  const a = activeCell.value
  if (!a || !props.canEdit || ctxKind.value !== 'cells') return
  const field = props.columns[a.c]?.name
  if (!field || !isNullable(field)) return
  const row = props.rows[a.r]
  stageEdit(row, field, row[field], null)
}

function ctxClearValue() {
  cellMenuVisible.value = false
  const a = activeCell.value
  if (!a || !props.canEdit || ctxKind.value !== 'cells') return
  const field = props.columns[a.c]?.name
  if (!field) return
  const row = props.rows[a.r]
  stageEdit(row, field, row[field], '')
}

function ctxInsertRow() {
  cellMenuVisible.value = false
  emit('insert-row')
}

function ctxDuplicateRow() {
  cellMenuVisible.value = false
  const a = activeCell.value
  if (!a || ctxKind.value === 'cols') return
  emit('duplicate-row', props.rows[a.r])
}

/** 删除选区覆盖的所有行（去重）；二次确认与后端删除由父组件处理。 */
function ctxDeleteRows() {
  cellMenuVisible.value = false
  const idxs = selectedRowIndices()
  if (!idxs.length) return
  emit('delete-rows', idxs.map(i => props.rows[i]))
}

const cellMenuRef = ref<InstanceType<typeof Menu> | null>(null)
const cellMenuVisible = ref(false)
function openCellMenu(x: number, y: number) {
  cellMenuVisible.value = true
  cellMenuRef.value?.openAt(x, y)
}

/** 单元格类名：奇偶行斑马纹 + 选区/主动格高亮（cellClassName 挂在 el-table 的 td 上）。
 *  斑马纹走自定义类而不是 el-table 的 stripe 属性，保证选中高亮可覆盖。
 *  斑马色落在奇数显示行（第 1、3、5…行，rowIndex 从 0 起算）。 */
function cellClassName({ rowIndex, column }: { rowIndex: number; column: { property?: string } }) {
  let cls = ''
  if (rowIndex % 2 === 0) cls = 'db-cell-striped'
  const idx = column?.property ? colIndexByName.value.get(column.property) : undefined
  if (idx !== undefined) {
    if (inSelection(rowIndex, idx)) cls = cls ? `${cls} db-cell-in-sel` : 'db-cell-in-sel'
    if (isActiveCell(rowIndex, column.property as string)) cls = `${cls} db-cell-selected`
  }
  return cls
}

/** 行高亮：仅被整行选区覆盖的行（挂在 el-table 的 tr 上）。
 *  单格/矩形选择不铺整行——只有点行号选整行时才亮整行。 */
function rowClassName({ rowIndex }: { rowIndex: number }) {
  for (const s of selRanges.value) {
    if (s.fullRow && rowIndex >= s.r1 && rowIndex <= s.r2) return 'db-row-selected'
  }
  return ''
}

/** 列头高亮：仅被整列选区覆盖的列（挂在 el-table 的 th 上）。
 *  单格/矩形选择不亮列头——只有点列头选整列时才亮列头。 */
function headerCellClassName({ column }: { column: { property?: string } }) {
  const idx = column?.property ? colIndexByName.value.get(column.property) : undefined
  if (idx === undefined) return ''
  for (const s of selRanges.value) {
    if (s.fullCol && idx >= s.c1 && idx <= s.c2) return 'db-col-selected'
  }
  return ''
}

/** 选区复制为 TSV（多选区按块拼接），可直接粘贴进 Excel/Numbers 分列。 */
function copySelectionTsv() {
  const blocks: string[] = []
  for (const s of selRanges.value) {
    const lines: string[] = []
    for (let r = s.r1; r <= s.r2; r++) {
      const row = props.rows[r]
      if (!row) continue
      const cells: string[] = []
      for (let c = s.c1; c <= s.c2; c++) cells.push(formatCell(row[props.columns[c]?.name]))
      lines.push(cells.join('\t'))
    }
    if (lines.length) blocks.push(lines.join('\n'))
  }
  if (blocks.length) writeClipboard(blocks.join('\n'))
  cellMenuVisible.value = false
}

/** 活动格所在整行复制为 TSV。 */
function copyActiveRow() {
  const a = activeCell.value
  const row = a ? props.rows[a.r] : null
  if (row) writeClipboard(props.columns.map(c => formatCell(row[c.name])).join('\t'))
  cellMenuVisible.value = false
}

function copyActiveColumnName() {
  const a = activeCell.value
  if (a) writeClipboard(props.columns[a.c]?.name ?? '')
  cellMenuVisible.value = false
}

/**
 * 网格容器 keydown：Ctrl/Cmd+C 复制当前选区。用户用鼠标拖选了真实文本时
 * 放行默认复制（尊重手动选区），与终端"有选区才复制"的语义一致。
 */
function onGridKeydown(e: KeyboardEvent) {
  if (e.type !== 'keydown' || (e.key !== 'c' && e.key !== 'C')) return
  if (!(e.ctrlKey || e.metaKey) || e.shiftKey || e.altKey) return
  // 正在编辑单元格时，input 的默认复制优先。
  if (editingRowIdx.value !== null) return
  const text = window.getSelection()?.toString() ?? ''
  if (text) return // 用户拖选了文本，走浏览器默认复制
  if (!selRanges.value.length) return
  e.preventDefault()
  copySelectionTsv()
}

/** 行内编辑状态：按行下标（el-table 插槽 row 是代理对象，不能做身份比较）。 */
const editingRowIdx = shallowRef<number | null>(null)
const editingField = ref<string | null>(null)
const draftValue = ref('')
const draftIsNull = ref(false)
let snapOldValue: any = undefined

interface StagedCell { old: any; newVal: any }
interface StagedRow { pk: Record<string, any>; fields: Map<string, StagedCell> }

/** Unsaved edits, keyed by the row object they belong to. Reactive for cell highlight. */
const pending = shallowReactive(new Map<Record<string, any>, StagedRow>())

function changedCellCount() {
  let n = 0
  for (const rec of pending.values()) n += rec.fields.size
  return n
}
function bumpPending() {
  emit('pending-change', changedCellCount())
}

function findMeta(colName: string) {
  const lower = colName.toLowerCase()
  return props.tableColumns?.find(c => c.name === colName || c.name.toLowerCase() === lower)
}
function colTitle(col: QueryResultColumn) {
  const meta = findMeta(col.name)
  if (meta?.isPrimary) return `${col.name} 🔑`
  return col.name
}
function isNullable(colName: string) {
  return findMeta(colName)?.nullable ?? true
}
function formatCell(v: any) {
  if (v === null || v === undefined) return ''
  if (typeof v === 'object') {
    try { return JSON.stringify(v) } catch { return String(v) }
  }
  return String(v)
}
function valuesEqual(a: any, b: any) {
  if (a === b) return true
  if (a === null || b === null || a === undefined || b === undefined) return false
  return String(a) === String(b)
}
function snapshotPk(row: Record<string, any>) {
  const pk: Record<string, any> = {}
  for (const k of props.primaryKeys || []) pk[k] = row[k] ?? null
  return pk
}
function isPending(row: Record<string, any>, field: string) {
  return pending.get(row)?.fields.has(field) ?? false
}

function startEdit(rowIndex: number, field: string) {
  if (editingRowIdx.value === rowIndex && editingField.value === field) return
  if (editingRowIdx.value !== null) onCellEditConfirm() // stage pending edit first
  editingRowIdx.value = rowIndex
  editingField.value = field
  emit('editing-change', true)
  const value = props.rows[rowIndex]?.[field]
  draftIsNull.value = value === null
  draftValue.value = value === null || value === undefined ? '' : String(value)
  snapOldValue = value
  document.addEventListener('mousedown', onEditOutsideMousedown)
  document.addEventListener('keydown', onEditKeydown)
  nextTick(() => {
    // 字符串 ref 在 v-for 里是数组；同一时刻只有一个编辑输入框存在。
    const input = Array.isArray(cellInputEl.value) ? cellInputEl.value[0] : cellInputEl.value
    input?.focus()
    input?.select()
  })
}

// Dismissal must not depend on the input's focus/blur: focus via the el-table
// slot ref is fragile, so listen globally while editing instead.
function onEditOutsideMousedown(e: MouseEvent) {
  const target = e.target as HTMLElement | null
  if (target?.closest('.cell-edit-wrap')) return
  onCellEditConfirm()
}
function onEditKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape') onCellEditCancel()
}
function onEditDismissCleanup() {
  document.removeEventListener('mousedown', onEditOutsideMousedown)
  document.removeEventListener('keydown', onEditKeydown)
}

// el-table's @cell-dblclick passes positional args (row, column, cell, event)
function onCellDblClick(_row: Record<string, any>, column?: { property?: string }, _cell?: HTMLElement, event?: MouseEvent) {
  if (!props.canEdit || !event) return
  const field = column?.property
  if (!field) return
  const hit = hitTest(event.clientX, event.clientY)
  if (!hit || hit.kind !== 'cells') return
  startEdit(hit.r ?? 0, field)
}

function onDraftInput() {
  draftIsNull.value = false
}
function setDraftNull() {
  draftIsNull.value = true
  draftValue.value = ''
}

function onCellEditConfirm() {
  const idx = editingRowIdx.value
  const field = editingField.value
  if (idx === null || !field) return
  stopEditing()
  const newVal: any = draftIsNull.value ? null : draftValue.value
  const oldVal = snapOldValue
  snapOldValue = undefined
  draftIsNull.value = false
  stageEdit(props.rows[idx], field, oldVal, newVal)
}

/** Stage an edit (shared by the inline editor and the cell editor dialog). */
function stageEdit(row: Record<string, any>, field: string, oldVal: any, newVal: any) {
  if (valuesEqual(oldVal, newVal)) {
    row[field] = oldVal
    return
  }
  // Stage instead of saving: reflect the change live, mark dirty, wait for explicit save.
  let rec = pending.get(row)
  if (!rec) {
    rec = { pk: snapshotPk(row), fields: reactive(new Map<string, StagedCell>()) }
    pending.set(row, rec)
  }
  row[field] = newVal
  const existing = rec.fields.get(field)
  const original = existing ? existing.old : oldVal
  if (valuesEqual(original, newVal)) {
    // Edited back to the committed value — drop the staged edit for this field.
    rec.fields.delete(field)
    if (rec.fields.size === 0) pending.delete(row)
    bumpPending()
    return
  }
  rec.fields.set(field, { old: original, newVal })
  bumpPending()
}

// ── 单元格放大编辑器弹窗 ──
const cellEditorRef = ref<InstanceType<typeof DBCellEditorDialog> | null>(null)
const cellEditorVisible = ref(false)
let cellEditorTarget: { rowIndex: number; field: string; value: any } | null = null

function isEditing(rowIndex: number, field: string) {
  return editingRowIdx.value === rowIndex && editingField.value === field
}

/** Open the long-text editor for one cell. Read-only grids get view/copy only. */
function openCellEditor(rowIndex: number, field: string) {
  if (isEditing(rowIndex, field)) onCellEditConfirm() // flush any inline draft first
  const value = props.rows[rowIndex]?.[field]
  cellEditorTarget = { rowIndex, field, value }
  cellEditorRef.value?.open({
    value,
    field,
    nullable: isNullable(field),
    readonly: !props.canEdit,
  })
}

/** Dialog save: stage through the same pending pipeline as inline editing. */
function onCellEditorSave(newVal: string | null) {
  const target = cellEditorTarget
  cellEditorTarget = null
  cellEditorVisible.value = false
  if (!target) return
  stageEdit(props.rows[target.rowIndex], target.field, target.value, newVal)
}

function onCellEditCancel() {
  const idx = editingRowIdx.value
  const field = editingField.value
  if (idx !== null && field) {
    const row = props.rows[idx]
    const rec = pending.get(row)
    const staged = rec?.fields.get(field)
    row[field] = staged ? staged.newVal : snapOldValue
  }
  stopEditing()
  snapOldValue = undefined
  draftIsNull.value = false
}

function stopEditing() {
  editingRowIdx.value = null
  editingField.value = null
  emit('editing-change', false)
  onEditDismissCleanup()
}

/** Discard all staged edits for a row and restore its committed values. */
function revertRow(row: Record<string, any>) {
  const rec = pending.get(row)
  if (!rec) return
  for (const [field, cell] of rec.fields) row[field] = cell.old
  pending.delete(row)
  bumpPending()
}
/** Discard every staged edit. */
function revertAll() {
  for (const [row, rec] of pending) {
    for (const [field, cell] of rec.fields) row[field] = cell.old
  }
  pending.clear()
  bumpPending()
}
/** Mark a staged edit as persisted (after a successful save). */
function markCommitted(row: Record<string, any>, field: string) {
  const rec = pending.get(row)
  if (!rec) return
  rec.fields.delete(field)
  if (rec.fields.size === 0) pending.delete(row)
  bumpPending()
}

/** One UPDATE per row keeps a changed primary key and other fields together. */
function getPendingEdits(): Array<{
  row: Record<string, any>
  set: Record<string, any>
  where: Record<string, any>
}> {
  const out: ReturnType<typeof getPendingEdits> = []
  for (const [row, rec] of pending) {
    const set: Record<string, any> = {}
    for (const [field, cell] of rec.fields) set[field] = cell.newVal
    out.push({ row, set, where: { ...rec.pk } })
  }
  return out
}

function markCommittedRow(row: Record<string, any>) {
  pending.delete(row)
  bumpPending()
}

function clearEdit() {
  onCellEditConfirm()
}

onBeforeUnmount(() => {
  onEditDismissCleanup()
  onDragEnd()
})

watch(() => props.rows, (rows, previous = []) => {
  // 选择按行列下标存储，行集合一变（新结果/过滤/翻页）即失效。
  clearSelection()
  if (editingRowIdx.value !== null) stopEditing()
  // Filtering creates new arrays containing the same row objects. Keep their
  // staged changes until a different result arrives.
  if (rows.every(row => previous.includes(row))) return
  if (pending.size) {
    pending.clear()
    bumpPending()
  }
})

defineExpose({
  getPendingEdits,
  markCommittedRow,
  revertRow,
  revertAll,
  markCommitted,
  clearEdit,
  getSelectedRows: () => [],
})
</script>

<style scoped>
.db-result-grid {
  flex: 1;
  min-height: 0;
  height: 100%;
  width: 100%;
  overflow: hidden;
  outline: none;
}
.db-result-table {
  --el-table-header-bg-color: var(--bg-elevated, var(--bg-surface));
  /* 悬停不高亮整行（Excel 语义：选择才高亮）。 */
  --el-table-row-hover-bg-color: transparent;
  --el-table-tr-bg-color: var(--bg-surface);
  --el-table-border-color: var(--border-subtle);
  --el-table-header-text-color: var(--text-secondary);
  --el-table-text-color: var(--text-primary);
  --el-table-bg-color: var(--bg-surface);
  font-size: 0.75rem;
}
/* 外框沿用 el-table 原生四条伪元素线（四边各 1px，行数不满一屏时底边也
   有框）。原先「底/右 2px」是因为伪元素线与末行/末列单元格边框重合
   ——去掉重合的单元格边框，统一 1px。 */
.db-result-table :deep(.el-table__body tr td:last-child),
.db-result-table :deep(.el-table__header-wrapper tr th:last-child) {
  border-right: none;
}
.db-result-table :deep(.el-table__body tr:last-child td) {
  border-bottom: none;
}
.row-actions {
  display: inline-flex;
  align-items: center;
  gap: 0.125rem;
}
.row-actions :deep(svg) { pointer-events: none; }
.cell-null {
  color: var(--text-muted);
  font-style: italic;
}
.cell-value {
  font-family: var(--font-mono, monospace);
  font-size: 0.75rem;
}
/* 行号列 / 列头 / 单元格的交互层：自带坐标属性供命中检测与拖动选择。 */
.db-rownum-cell,
.db-rownum-head,
.db-col-head {
  height: 100%;
  display: flex;
  align-items: center;
  user-select: none;
}
/* 序号列收紧左内边距：el-table .cell 自带 8px padding，去掉并给数字
   一点缩进即可。 */
.db-result-table :deep(td.db-rownum-col .cell),
.db-result-table :deep(th.db-rownum-col .cell) {
  padding: 0 0.25rem;
}
.db-rownum-cell {
  justify-content: flex-start;
  padding-left: 0.25rem;
  color: var(--text-muted);
  font-size: 0.75rem;
}
.db-rownum-head,
.db-rownum-cell,
.db-col-head {
  cursor: default;
}
/* 斑马纹：奇数显示行（1、3、5…）铺极淡蓝（Excel 风格），偶数行用数据库
   tab 内容的底色（--bg-base），与周边面板连成一体。悬停/选中/暂存高亮的
   规则在同文件更后面定义，可正常覆盖。 */
.db-result-table {
  --el-table-tr-bg-color: var(--bg-base);
}
/* 注意：行号列/操作列是 sticky 固定列，td 自身滚动悬停、下层内容会从
   半透明底色透出来。所以斑马纹/选区统一用「不透明底色 + 半透明色
   background-image 叠加」绘制，视觉相同但不透底。 */
.db-result-table :deep(td.db-cell-striped) {
  background-color: var(--bg-base);
  background-image: linear-gradient(var(--accent-subtle), var(--accent-subtle));
}
/* 选区覆盖的单元格：铺选中底色。带 td 元素选择器，与斑马纹规则同权重，
   且定义在后，奇数行（斑马行）上也能正常覆盖。 */
.db-result-table :deep(td.db-cell-in-sel) {
  background-color: var(--bg-base);
  background-image: linear-gradient(var(--accent-glow), var(--accent-glow));
}
/* 选中单元格所在整行 / 对应列表头：与选中格一起构成十字定位。
   同样用不透明底色 + 半透明色叠加（sticky 固定列不透底）；
   规则顺序在斑马纹之后，可覆盖；选中格自身还有 inset 描边区分。 */
.db-result-table :deep(tr.db-row-selected) td.el-table__cell {
  background-color: var(--bg-base);
  background-image: linear-gradient(var(--accent-glow), var(--accent-glow));
}
.db-result-table :deep(th.el-table__cell.db-col-selected) {
  background-color: var(--bg-elevated, var(--bg-surface));
  background-image: linear-gradient(var(--accent-glow), var(--accent-glow));
}
/* 悬停不高亮整行：element-plus 除了 :hover 规则还会用 JS 给行加
   hover-row / hover-cell 类，逐条以更高优先级压掉（置透明）。
   注意：本规则权重 (0,5,2) 高于下方行号/操作列/主动格以外的所有带色
   规则；带色单元格靠 background-image 叠加层维持视觉，不受影响。 */
.db-result-table :deep(.el-table__body tr.hover-row > td.el-table__cell),
.db-result-table :deep(.el-table__body tr > td.hover-cell),
.db-result-table :deep(.el-table--enable-row-hover .el-table__body tr:hover > td.el-table__cell) {
  background-color: transparent;
}
/* 行号列是固定导轨列：底色与标题行一致、不参与斑马纹。权重 (0,5,2) 与
   hover 透明规则同权重且定义在后，rAF 摘类窗口期也不透底；整行选中
   高亮 (0,6,2) 定义更后，仍可覆盖到行号格。 */
.db-result-table :deep(.el-table__body tr td.el-table__cell.db-rownum-col) {
  background-color: var(--el-table-header-bg-color);
}
/* 主动格（最后点击的选中单元格）：双层 accent-glow 叠加出更深的蓝，
   比选区/行高亮（单层 glow）明显更深，配 1px accent 描边，一眼可辨。
   权重与 hover 透明规则同级且在后，hover 下也能维持底色。 */
.db-result-table :deep(.el-table__body tr td.el-table__cell.db-cell-selected) {
  background-color: var(--bg-base);
  background-image: linear-gradient(var(--accent-glow), var(--accent-glow)), linear-gradient(var(--accent-glow), var(--accent-glow));
  outline: none;
}
/* 操作列同为 sticky 固定列：底色权重高于 hover 透明规则（rAF 摘类窗口
   期不透底）。显式清掉 background-image——整行选中（row-selected）的
   蓝色叠加层不会渗到操作列；斑马纹用叠加层保留在奇数行。 */
.db-result-table :deep(.el-table__body tr td.el-table__cell.db-action-cell) {
  background-color: var(--bg-base);
  background-image: none;
}
.db-result-table :deep(.el-table__body tr td.el-table__cell.db-action-cell.db-cell-striped) {
  background-image: linear-gradient(var(--accent-subtle), var(--accent-subtle));
}
/* 放大按钮锚在单元格内容盒上（盒子铺满 .cell 内容区），不依赖 td 的
   定位上下文，避免被裁剪链路影响。 */
.db-cell-box {
  position: relative;
}
.cell-expand-btn {
  position: absolute;
  right: 0.125rem;
  top: 50%;
  transform: translateY(-50%);
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 1.125rem;
  height: 1.125rem;
  padding: 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-surface);
  color: var(--text-muted);
}
.cell-expand-btn:hover {
  color: var(--accent);
  border-color: var(--accent);
}
.cell-expand-btn :deep(svg) { pointer-events: none; }
.cell-pending {
  background-color: var(--accent-subtle, rgba(64, 128, 255, 0.12));
  box-shadow: inset 0 -0.125rem 0 var(--accent);
}
.cell-edit-wrap {
  display: flex;
  align-items: stretch;
  width: 100%;
  gap: 0.125rem;
}
.cell-edit-input {
  flex: 1;
  min-width: 0;
  height: 1.5rem;
  padding: 0 0.375rem;
  border: 1px solid var(--accent);
  border-radius: var(--radius-sm);
  background: var(--bg-base);
  color: var(--text-primary);
  font-family: var(--font-mono, monospace);
  font-size: 0.75rem;
  outline: none;
}
.null-btn {
  flex-shrink: 0;
  width: 1.375rem;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  background: var(--bg-elevated, var(--bg-hover));
  color: var(--text-muted);
  font-size: 0.625rem;
  font-weight: 700;
  font-style: italic;
}
.null-btn:hover {
  color: var(--text-primary);
  border-color: var(--accent);
}
</style>
