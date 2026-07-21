<template>
  <el-dialog append-to-body v-model="visible" :title="t('serial.title')" width="420px">
    <el-form label-width="100px" @submit.prevent="onConnect">
      <el-form-item :label="t('serial.portLabel')">
        <el-select
          v-model="selectedPort"
          :placeholder="portPlaceholder"
          :disabled="ports.length === 0 || scanning"
          :loading="scanning"
        >
          <el-option
            v-for="port in ports"
            :key="port"
            :label="port"
            :value="port"
          />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('serial.baudRate')">
        <el-autocomplete
          v-model="baudRateInput"
          :fetch-suggestions="queryBaudSuggestions"
          :placeholder="t('serial.baudRate')"
          clearable
          style="width:100%"
        />
      </el-form-item>
      <el-form-item :label="t('serial.dataBits')">
        <el-select v-model="dataBits">
          <el-option
            v-for="bits in dataBitsOptions"
            :key="bits"
            :label="String(bits)"
            :value="bits"
          />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('serial.stopBits')">
        <el-select v-model="stopBits">
          <el-option
            v-for="bits in stopBitsOptions"
            :key="bits"
            :label="String(bits)"
            :value="bits"
          />
        </el-select>
      </el-form-item>
      <el-form-item :label="t('serial.parity')">
        <el-select v-model="parity">
          <el-option :label="t('serial.parityNone')" value="none" />
          <el-option :label="t('serial.parityOdd')" value="odd" />
          <el-option :label="t('serial.parityEven')" value="even" />
          <el-option :label="t('serial.parityMark')" value="mark" />
          <el-option :label="t('serial.paritySpace')" value="space" />
        </el-select>
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="visible = false">{{ t('conn.cancel') }}</el-button>
      <el-button
        type="primary"
        :disabled="!selectedPort"
        :loading="connecting"
        @click="onConnect"
      >
        {{ t('serial.connect') }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from '../i18n'
import { ListSerialPorts, ConnectSerial } from '../../wailsjs/go/main/App'

const { t } = useI18n()

const props = defineProps<{
  modelValue: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: boolean]
  connect: [sessionId: string, portName: string, baudRate: number]
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v)
})

// Serial port scanning
const ports = ref<string[]>([])
const scanning = ref(false)

// Form state
const selectedPort = ref('')
const baudRateInput = ref('')
const dataBits = ref<number>(8)
const stopBits = ref<number>(1)
const parity = ref<string>('none')
const connecting = ref(false)

// Options
const baudRatePresets = [300, 1200, 2400, 4800, 9600, 14400, 19200, 38400, 57600, 115200, 230400, 460800, 921600]
const dataBitsOptions = [5, 6, 7, 8]
const stopBitsOptions = [1, 1.5, 2]

function queryBaudSuggestions(queryString: string, cb: (results: { value: string }[]) => void) {
  const suggestions = baudRatePresets
    .filter(r => String(r).includes(queryString))
    .map(r => ({ value: String(r) }))
  cb(suggestions)
}

const portPlaceholder = computed(() => {
  if (scanning.value) return t('serial.scanning')
  if (ports.value.length === 0) return t('serial.noPorts')
  return t('serial.portLabel')
})

async function scanPorts() {
  scanning.value = true
  try {
    ports.value = await ListSerialPorts()
  } catch {
    ports.value = []
  } finally {
    scanning.value = false
  }
}

async function onConnect() {
  if (!selectedPort.value || connecting.value) return
  connecting.value = true
  try {
    const baud = parseInt(baudRateInput.value, 10) || 115200
    const session = await ConnectSerial(
      selectedPort.value,
      baud,
      dataBits.value,
      stopBits.value,
      parity.value
    )
    emit('connect', session.id, selectedPort.value, baud)
    visible.value = false
  } catch {
    // Connection failed, keep dialog open so user can retry
  } finally {
    connecting.value = false
  }
}

// Auto-scan when dialog opens
watch(visible, (val) => {
  if (val) {
    selectedPort.value = ''
    baudRateInput.value = ''
    dataBits.value = 8
    stopBits.value = 1
    parity.value = 'none'
    scanPorts()
  }
})
</script>
