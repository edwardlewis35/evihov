<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { LineChart } from 'echarts/charts'
import { DataZoomComponent, GridComponent, LegendComponent, TooltipComponent } from 'echarts/components'
import { devicesService } from '../services/devices'
import type { AppError } from '../types/domain'
import type { SignalHistoryRange, SignalHistoryResponse, SignalHistorySetting } from '../types/api'

use([CanvasRenderer, LineChart, DataZoomComponent, GridComponent, LegendComponent, TooltipComponent])

const props = defineProps<{
  deviceId: string
  iccid?: string
  profileName?: string
}>()
const range = ref<SignalHistoryRange>('day')
const history = ref<SignalHistoryResponse | null>(null)
const setting = ref<SignalHistorySetting | null>(null)
const retentionDays = ref(30)
const loading = ref(false)
const saving = ref(false)
const error = ref<AppError | null>(null)
let controller: AbortController | null = null
let requestID = 0
let refreshTimer: number | null = null
let lastLoadedAt = 0

const ranges: Array<{ value: SignalHistoryRange; label: string }> = [
  { value: 'day', label: '24小时' },
  { value: 'week', label: '7天' },
  { value: 'month', label: '30天' },
  { value: 'retention', label: '全部' }
]

const points = computed(() => history.value?.points || [])
const hasData = computed(() => points.value.some(point =>
  [point.rssi, point.rsrp, point.rsrq, point.sinr, point.nr5g_sinr].some(value => typeof value === 'number')
))
const profileLabel = computed(() => {
  const name = props.profileName?.trim() || ''
  const iccid = props.iccid?.trim() || ''
  const shortICCID = iccid ? `••••${iccid.slice(-6)}` : ''
  return [name, shortICCID].filter(Boolean).join(' · ')
})

async function loadHistory(showLoading = true) {
  const id = props.deviceId.trim()
  const requestedRange = range.value
  const idNumber = ++requestID
  controller?.abort()
  if (!id) return
  const current = new AbortController()
  controller = current
  if (showLoading) loading.value = true
  error.value = null
  const result = await devicesService.getSignalHistory(id, requestedRange, current.signal)
  if (idNumber !== requestID) return
  loading.value = false
  controller = null
  if (!result.ok) {
    if (result.error.code !== 'ERR_CANCELED') error.value = result.error
    return
  }
  history.value = result.data
  lastLoadedAt = Date.now()
}

async function loadSetting() {
  const result = await devicesService.getSignalHistorySetting()
  if (!result.ok) {
    ElMessage.error(result.error.message || '读取信号历史设置失败')
    return
  }
  setting.value = result.data
  retentionDays.value = result.data.retention_days
}

async function saveSetting() {
  const min = setting.value?.min_days ?? 1
  const max = setting.value?.max_days ?? 3650
  const days = Math.trunc(Number(retentionDays.value))
  if (!Number.isFinite(days) || days < min || days > max) {
    ElMessage.warning(`保留天数必须在 ${min} 到 ${max} 之间`)
    return
  }
  saving.value = true
  const result = await devicesService.updateSignalHistorySetting(days)
  saving.value = false
  if (!result.ok) {
    ElMessage.error(result.error.message || '保存失败')
    return
  }
  setting.value = result.data
  retentionDays.value = result.data.retention_days
  ElMessage.success('保留设置已保存，过期记录已清理')
  if (range.value === 'retention') await loadHistory()
}

function handleRangeChange(value: string | number | boolean | undefined) {
  if (value === 'day' || value === 'week' || value === 'month' || value === 'retention') range.value = value
}

const chartOption = computed(() => {
  const data = (key: 'rssi' | 'rsrp' | 'rsrq' | 'sinr' | 'nr5g_sinr') =>
    points.value.map(point => [point.recorded_at, point[key]])
  return {
    animation: false,
    backgroundColor: 'transparent',
    tooltip: { trigger: 'axis', axisPointer: { type: 'cross' } },
    legend: { type: 'scroll', top: 0, textStyle: { color: '#9ca3af' } },
    grid: { left: 20, right: 20, top: 52, bottom: 42, containLabel: true },
    dataZoom: [{ type: 'inside', filterMode: 'none' }],
    xAxis: { type: 'time', axisLabel: { color: '#9ca3af', hideOverlap: true } },
    yAxis: [
      { type: 'value', name: 'dBm', position: 'left', axisLabel: { color: '#9ca3af' }, splitLine: { lineStyle: { type: 'dashed', opacity: 0.2 } } },
      { type: 'value', name: 'dB', position: 'right', axisLabel: { color: '#9ca3af' }, splitLine: { show: false } }
    ],
    series: [
      { name: 'RSSI (dBm)', type: 'line', yAxisIndex: 0, data: data('rssi'), symbol: 'none', connectNulls: false },
      { name: 'RSRP (dBm)', type: 'line', yAxisIndex: 0, data: data('rsrp'), symbol: 'none', connectNulls: false },
      { name: 'RSRQ (dB)', type: 'line', yAxisIndex: 1, data: data('rsrq'), symbol: 'none', connectNulls: false },
      { name: 'SINR (dB)', type: 'line', yAxisIndex: 1, data: data('sinr'), symbol: 'none', connectNulls: false },
      { name: 'NR5G SINR (dB)', type: 'line', yAxisIndex: 1, data: data('nr5g_sinr'), symbol: 'none', connectNulls: false }
    ]
  }
})

watch(() => [props.deviceId, props.iccid] as const, () => {
  history.value = null
  void loadHistory()
}, { immediate: true })

watch(range, () => void loadHistory())

onMounted(() => {
  void loadSetting()
  refreshTimer = window.setInterval(() => {
    const interval = document.hidden ? 5 * 60_000 : 60_000
    if (Date.now() - lastLoadedAt >= interval) void loadHistory(false)
  }, 60_000)
})

onBeforeUnmount(() => {
  controller?.abort()
  requestID++
  if (refreshTimer !== null) window.clearInterval(refreshTimer)
})
</script>

<template>
  <section class="ui-panel-muted p-4 overflow-hidden">
    <div class="flex flex-col xl:flex-row xl:items-center xl:justify-between gap-3 mb-4">
      <div>
        <div class="text-sm font-bold text-gray-800 dark:text-gray-100">信号强度历史</div>
        <div class="text-xs text-gray-400 mt-1">
          <template v-if="profileLabel">当前 eSIM：{{ profileLabel }}；每分钟保存一次</template>
          <template v-else>等待 SIM/eSIM 身份确认，切换期间暂停记录</template>
        </div>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <el-radio-group :model-value="range" size="small" @change="handleRangeChange">
          <el-radio-button v-for="item in ranges" :key="item.value" :label="item.value">{{ item.label }}</el-radio-button>
        </el-radio-group>
        <el-button size="small" :loading="loading" @click="loadHistory()">刷新</el-button>
        <span class="text-xs text-gray-500">全局保留</span>
        <el-input-number v-model="retentionDays" size="small" :min="setting?.min_days ?? 1" :max="setting?.max_days ?? 3650" :controls="false" class="!w-20" />
        <span class="text-xs text-gray-500">天</span>
        <el-button size="small" :loading="saving" @click="saveSetting">保存</el-button>
      </div>
    </div>
    <div v-if="error" class="h-[260px] flex flex-col items-center justify-center gap-3 text-sm text-red-500">
      <span>{{ error.message }}</span><el-button size="small" @click="loadHistory()">重试</el-button>
    </div>
    <div v-else-if="loading && !history" class="h-[260px]" v-loading="true" />
    <VChart v-else-if="hasData" :option="chartOption" autoresize class="h-[320px] w-full" />
    <div v-else class="h-[220px] rounded-xl border border-dashed border-gray-200 dark:border-white/10 flex items-center justify-center text-sm text-gray-400">
      {{ history && !history.identity_ready ? 'eSIM 切换中，身份确认后将自动显示新 Profile 的历史' : '当前 SIM/eSIM 暂无信号历史数据' }}
    </div>
  </section>
</template>
