<template>
  <div v-if="phase" class="progress-card card" :class="'phase-' + phase">
    <div class="card-body">
      <div class="status-row">
        <div class="status-info">
          <div class="status-dot"></div>
          <div>
            <div class="status-label">Current Scan</div>
            <div class="status-phase">{{ phaseLabel }}</div>
          </div>
        </div>
        <div style="text-align: right">
          <div class="pct">{{ percentDisplay }}</div>
        </div>
      </div>

      <div class="progress-track">
        <div class="progress-fill" :class="{ indeterminate: isIndeterminate }" :style="{ width: barWidth }"></div>
      </div>

      <div class="progress-stats">
        <span v-if="phase === 'discovery'">Discovered: {{ total.toLocaleString() }} files</span>
        <span v-else-if="phase === 'scanning'">Scanned: {{ current.toLocaleString() }} / {{ total.toLocaleString() }}
          files</span>
        <span v-else-if="phase === 'comparing'">Searching for possible duplicates...</span>
        <span v-else>Done: {{ current.toLocaleString() }} out of {{ total.toLocaleString() }}</span>

        <span v-if="eta">ETA: {{ eta }}</span>
        <span v-else-if="durationSeconds > 0">Elapsed: {{ formatDuration(durationSeconds) }}</span>
      </div>

      <div v-if="lastFile" class="last-file">
        {{ lastFile }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, watch } from 'vue'
import { EventsOn } from '../api'

const props = defineProps({
  scanning: Boolean,
  initialState: { type: Object, default: () => ({ current: 0, total: 0, phase: '', last_file: '', duration_seconds: 0 }) }
})
defineEmits(['stop'])

const phase = ref(props.initialState.phase || '')
const current = ref(props.initialState.current || 0)
const total = ref(props.initialState.total || 0)
const lastFile = ref(props.initialState.last_file || '')
const durationSeconds = ref(props.initialState.duration_seconds || 0)

// Update internal state when initialState prop changes (e.g. after async fetch)
watch(() => props.initialState, (newVal) => {
  if (newVal) {
    phase.value = newVal.phase
    current.value = newVal.current
    total.value = newVal.total
    lastFile.value = newVal.last_file
    durationSeconds.value = newVal.duration_seconds || 0
  }
}, { deep: true })

const phaseLabel = computed(() => {
  if (phase.value === 'discovery') return '🔍 Discovering files...'
  if (phase.value === 'scanning') return '⚙️ Processing files...'
  if (phase.value === 'comparing') return '🍏🍊 Comparing and grouping...'
  if (phase.value === 'completed') return '✅ Scan Complete'
  return phase.value || 'Starting...'
})

const isIndeterminate = computed(() => {
  return phase.value === 'discovery'
})

const percent = computed(() => {
  if (phase.value === 'completed') return 100
  if (total.value <= 1 || current.value <= 0) return 0
  return Math.min(100, (current.value / total.value) * 100)
})

const percentDisplay = computed(() => {
  if (isIndeterminate.value) return ''
  return Math.round(percent.value) + '%'
})

const barWidth = computed(() => {
  if (isIndeterminate.value) return '100%'
  return percent.value + '%'
})

const estimatedRemainingSeconds = ref(0)
const eta = computed(() => {
  if ((phase.value !== 'scanning' && phase.value !== 'comparing') || estimatedRemainingSeconds.value <= 0) return ''
  return formatDuration(estimatedRemainingSeconds.value)
})

const formatDuration = (secs) => {
  const totalSeconds = Math.round(secs)
  if (totalSeconds < 60) return `${totalSeconds}s`
  if (totalSeconds < 3600) {
    const m = Math.floor(totalSeconds / 60)
    const s = totalSeconds % 60
    return `${m}m ${s}s`
  }
  const h = Math.floor(totalSeconds / 3600)
  const m = Math.floor((totalSeconds % 3600) / 60)
  return `${h}h ${m}m`
}

onMounted(() => {
  EventsOn('scan_progress', (data) => {
    phase.value = data.phase
    current.value = data.current
    total.value = data.total
    lastFile.value = data.last_file || ''
    durationSeconds.value = data.duration_seconds || 0
    estimatedRemainingSeconds.value = data.estimated_remaining_seconds || 0
  })
})
</script>

<style scoped>
.progress-card {
  min-width: 0;
  width: 100%;
  overflow: hidden;
}

.last-file {
  margin-top: 12px;
  font-size: 11px;
  color: var(--text-muted);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  background: var(--surface-alt);
  padding: 4px 8px;
  border-radius: 4px;
  max-width: 100%;
}

/* Phase-specific colors */
.phase-discovery .progress-fill {
  background: var(--text-muted) !important;
}

.phase-discovery .status-dot {
  background: var(--text-muted) !important;
}

.phase-comparing .pct {
  color: var(--success) !important;
}

.phase-comparing .progress-fill {
  background: var(--success) !important;
}

.phase-comparing .status-dot {
  background: var(--success) !important;
}

.progress-fill.indeterminate {
  animation: indeterminate 1.5s ease-in-out infinite;
  background: linear-gradient(90deg, transparent 0%, var(--accent) 50%, transparent 100%);
  background-size: 200% 100%;
}

@keyframes indeterminate {
  0% {
    background-position: 200% 0;
  }

  100% {
    background-position: -200% 0;
  }
}
</style>
