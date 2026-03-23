<template>
  <div class="status-line" :class="{ 'is-scanning': scanning }">
    <div class="status-container">
      <!-- Left: Library Vitals -->
      <div class="status-group vitals">
        <div class="status-item" title="Total unique video files found">
          <span class="status-icon">🎞</span>
          <span class="status-value">{{ stats.total_files.toLocaleString() }}</span>
          <span class="status-label">Files</span>
        </div>
        <div class="status-divider"></div>
        <div class="status-item" title="Total library size">
          <span class="status-icon">🎞</span>
          <span class="status-value">{{ formatSize(stats.total_size) }}</span>
        </div>
        <div class="status-divider"></div>
        <div class="status-item" title="Total duplicates and groups">
          <span class="status-icon">👯</span>
          <span class="status-value">{{ resultsSummary.groups }}</span>
          <span class="status-label">Groups</span>
          <span class="status-val-sm">({{ resultsSummary.files }} files)</span>
        </div>
      </div>

      <!-- Center: Progress Indicator (only during scan) -->
      <div class="status-group progress-center" v-if="scanning">
        <div class="progress-pill">
          <div class="spinner"></div>
          <div class="progress-info">
            <span class="phase">{{ scanState.phase }}</span>
            <span class="count" v-if="scanState.total > 0">
              {{ scanState.current }} / {{ scanState.total }}
              <span class="eta" v-if="formattedETA">&middot; ETA: {{ formattedETA }}</span>
            </span>
          </div>
          <div class="mini-progress-track">
            <div class="mini-progress-bar" :style="{ width: progressPercent + '%' }"></div>
          </div>
        </div>
      </div>

      <!-- Right: Selection & Mode -->
      <div class="status-group meta-right">
        <button class="status-item btn-clear" @click="toggleLogs" :class="{ active: showLogs }">
          <span class="status-icon">📝</span>
          Activity Log <span class="status-val-sm" v-if="appLogs.length > 0">({{ appLogs.length }})</span>
        </button>
        <div v-if="selectionCount > 0" class="selection-badge">
          <span class="badge-dot"></span>
          {{ selectionCount }} selected · {{ formatSize(selectionSize) }}
        </div>
        <button class="status-item btn-clear bug-btn" @click="reportBug" title="Report a bug">
          <span class="status-icon">🐞</span>
          Report Bug
        </button>
        <div class="status-item wails-indicator" v-if="isWails">
          <span class="indicator-dot online"></span>
          App <span class="version-tag">{{ appVersion }}</span>
          <a v-if="updateAvailable" :href="updateAvailable.url" target="_blank" class="update-link" title="A new version is available!">
            🚀 New Version
          </a>
        </div>
        <div class="status-item wails-indicator" v-else>
          <span class="indicator-dot web"></span>
          Web UI <span class="version-tag">{{ appVersion }}</span>
          <a v-if="updateAvailable" :href="updateAvailable.url" target="_blank" class="update-link" title="A new version is available!">
            🚀 New Version
          </a>
        </div>
        <!-- Mobile Scanning Pulse -->
        <div v-if="scanning" class="scanning-pulse-mobile" title="Scan in progress">
          <div class="pulse-dot"></div>
        </div>
      </div>
    </div>

    <!-- Activity Log Drawer -->
    <transition name="slide-up">
      <div class="log-drawer" v-if="showLogs">
        <div class="log-header">
          <span class="log-title">Application Activity</span>
          <div class="log-actions">
            <button class="log-btn" @click="clearLogs">Clear</button>
            <button class="log-btn close-btn" @click="toggleLogs">×</button>
          </div>
        </div>
        <div class="log-content">
          <div v-if="appLogs.length === 0" class="log-empty">No activity recorded yet.</div>
          <div v-else class="log-entry" v-for="(log, i) in appLogs" :key="i" :class="'log-' + log.severity">
            <span class="log-time">{{ log.time }}</span>
            <span class="log-msg">{{ log.message }}</span>
          </div>
          <div ref="logsEndRef"></div>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted, nextTick } from 'vue'
import { EventsOn, GetDebugInfo, GetActivityHistory, ClearPersistedLogs, CheckForUpdates } from '../api'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'

const props = defineProps({
  stats: { type: Object, default: () => ({ total_files: 0, total_size: 0 }) },
  scanning: { type: Boolean, default: false },
  scanState: { type: Object, default: () => ({ current: 0, total: 0, phase: '' }) },
  resultsSummary: { type: Object, default: () => ({ groups: 0, files: 0 }) },
  selectionCount: { type: Number, default: 0 },
  selectionSize: { type: Number, default: 0 }
})

const isWails = !!window.go
const showLogs = ref(false)
const appLogs = ref([])
const appVersion = ref('...')
const updateAvailable = ref(null)
const logsEndRef = ref(null)
let unlistenLog = null

const toggleLogs = () => {
  showLogs.value = !showLogs.value
  if (showLogs.value) {
    nextTick(() => {
      if (logsEndRef.value) {
        logsEndRef.value.scrollIntoView()
      }
    })
  }
}

const clearLogs = async () => {
  appLogs.value = []
  await ClearPersistedLogs()
}

onMounted(async () => {
  // Fetch version info
  try {
    const debug = await GetDebugInfo()
    appVersion.value = debug.version || 'unknown'
  } catch (e) {
    console.warn('StatusLine: Failed to fetch version info', e)
  }

  // Load history from RxDB
  try {
    const history = await GetActivityHistory()
    if (history && history.length > 0) {
      appLogs.value = history
    }
  } catch (e) {
    console.warn('StatusLine: Failed to load log history', e)
  }

  // Check for updates
  try {
    const res = await CheckForUpdates()
    if (res.update_available) {
      updateAvailable.value = res
    }
  } catch (e) {
    console.warn('StatusLine: Failed to check for updates', e)
  }

  unlistenLog = EventsOn('app_log', (data) => {
    appLogs.value.push(data)
    if (appLogs.value.length > 500) {
      appLogs.value.shift()
    }
    if (showLogs.value) {
      nextTick(() => {
        if (logsEndRef.value) {
          logsEndRef.value.scrollIntoView({ behavior: 'smooth' })
        }
      })
    }
  })
})

onUnmounted(() => {
  if (unlistenLog) unlistenLog()
})

const progressPercent = computed(() => {
  if (!props.scanState.total) return 0
  return Math.min(100, Math.round((props.scanState.current / props.scanState.total) * 100))
})

const formattedETA = computed(() => {
  if (props.scanState.phase !== 'scanning' || !props.scanState.estimated_remaining_seconds) return ''
  const secs = props.scanState.estimated_remaining_seconds
  if (secs < 60) return `${Math.round(secs)}s`
  if (secs < 3600) {
    const m = Math.floor(secs / 60)
    const s = Math.round(secs % 60)
    return `${m}m ${s}s`
  }
  const h = Math.floor(secs / 3600)
  const m = Math.round((secs % 3600) / 60)
  return `${h}h ${m}m`
})

const formatSize = (bytes) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i]
}

// Report Bug
const FORM_URL = 'https://docs.google.com/forms/d/e/1FAIpQLSeQlwMLSDiS3paYLno2NpaLSpci_rTJnvhXLw2Wou5ixw2Y0A/viewform'
const FIELD_DESCRIPTION = 'entry.1527808167'
const FIELD_DEBUG = 'entry.1243639100'

const openExternal = (url) => {
  if (isWails) {
    BrowserOpenURL(url)
  } else {
    window.open(url, '_blank')
  }
}

const reportBug = async () => {
  let debugStr = ''
  try {
    const debug = await GetDebugInfo()
    debugStr = JSON.stringify(debug, null, 2)
  } catch (e) {
    console.error('Failed to get debug info:', e)
    debugStr = `Error gathering debug info: ${e}`
  }
  const url = `${FORM_URL}?${FIELD_DEBUG}=${encodeURIComponent(debugStr)}`
  openExternal(url)
}
</script>

<style scoped>
.status-line {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  height: 32px;
  background: #11111a;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  color: #94a3b8;
  font-size: 11px;
  z-index: 1000;
  display: flex;
  align-items: center;
  transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
  font-family: 'Inter', -apple-system, system-ui, sans-serif;
  box-shadow: 0 -4px 12px rgba(0, 0, 0, 0.05);
}

.status-line.is-scanning {
  background: #09090e;
  border-top-color: var(--accent);
  box-shadow: 0 -2px 10px rgba(0, 0, 0, 0.3);
}

.status-container {
  width: 100%;
  max-width: 1400px;
  margin: 0 auto;
  padding: 0 16px;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.status-group {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-shrink: 0;
}

.status-item {
  display: flex;
  align-items: center;
  gap: 6px;
  white-space: nowrap;
}

.status-icon {
  font-size: 12px;
  opacity: 0.8;
}

.status-value {
  color: #e2e8f0;
  font-weight: 600;
}

.status-val-sm {
  font-size: 10px;
  opacity: 0.6;
}

.status-label {
  opacity: 0.6;
  text-transform: lowercase;
}

.status-divider {
  width: 1px;
  height: 12px;
  background: rgba(255, 255, 255, 0.1);
}

/* Progress indicator */
.progress-center {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
}

.progress-pill {
  display: flex;
  align-items: center;
  gap: 8px;
  background: #09090e;
  padding: 4px 12px;
  border-radius: 20px;
  border: 1px solid rgba(255, 255, 255, 0.1);
  min-width: 180px;
  height: 20px;
}

.progress-info {
  display: flex;
  gap: 6px;
  font-weight: 500;
}

.phase {
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: #60a5fa;
  font-size: 10px;
}

.count {
  color: #cbd5e1;
  font-size: 10px;
}

.mini-progress-track {
  flex-grow: 1;
  height: 3px;
  background: rgba(255, 255, 255, 0.05);
  border-radius: 2px;
  overflow: hidden;
  position: relative;
  width: 60px;
}

.mini-progress-bar {
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  background: var(--accent);
  box-shadow: 0 0 4px var(--accent);
  transition: width 0.3s ease;
}

.spinner {
  width: 10px;
  height: 10px;
  border: 2px solid rgba(255, 255, 255, 0.1);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 0.8s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

/* Right side */
.selection-badge {
  background: rgba(248, 113, 113, 0.15);
  color: #f87171;
  padding: 2px 8px;
  border-radius: 10px;
  border: 1px solid rgba(248, 113, 113, 0.2);
  font-weight: 600;
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: 4px;
}

.badge-dot {
  width: 6px;
  height: 6px;
  background: #f87171;
  border-radius: 50%;
  box-shadow: 0 0 4px #f87171;
}

.indicator-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.indicator-dot.online {
  background: #4ade80;
  box-shadow: 0 0 6px #4ade80;
}

.indicator-dot.web {
  background: #60a5fa;
  box-shadow: 0 0 6px #60a5fa;
}

.wails-indicator {
  font-size: 11px;
  padding-left: 8px;
  border-left: 1px solid var(--border);
  display: flex;
  align-items: center;
  gap: 8px;
}

.version-tag {
  opacity: 0.5;
  font-family: ui-monospace, monospace;
  font-size: 10px;
}

.update-link {
  color: #fbbf24;
  text-decoration: none;
  font-weight: 600;
  font-size: 10px;
  animation: pulse-update 2s infinite;
  display: flex;
  align-items: center;
  gap: 4px;
}

.update-link:hover {
  text-decoration: underline;
  color: #fff;
}

@keyframes pulse-update {
  0% { opacity: 0.7; transform: scale(1); }
  50% { opacity: 1; transform: scale(1.05); }
  100% { opacity: 0.7; transform: scale(1); }
}

.bug-btn {
  font-size: 11px;
  transition: all 0.2s;
}

.bug-btn:hover {
  color: #fbbf24;
}

/* Mobile Scanning Pulse */
.scanning-pulse-mobile {
  display: none;
  padding-left: 12px;
  border-left: 1px solid var(--border);
}

.pulse-dot {
  width: 10px;
  height: 10px;
  background: #4ade80;
  border-radius: 50%;
  box-shadow: 0 0 0 0 rgba(74, 222, 128, 0.7);
  animation: pulse 1.5s infinite;
}

@keyframes pulse {
  0% {
    box-shadow: 0 0 0 0 rgba(74, 222, 128, 0.7);
  }

  70% {
    box-shadow: 0 0 0 10px rgba(74, 222, 128, 0);
  }

  100% {
    box-shadow: 0 0 0 0 rgba(74, 222, 128, 0);
  }
}

@media (max-width: 1250px) {
  .mini-progress-track {
    display: none;
  }
}

@media (max-width: 1100px) {
  .progress-center {
    display: none;
  }
}

@media (max-width: 768px) {

  .vitals .status-item:not(:first-child),
  .vitals .status-divider {
    display: none;
  }

  .progress-center {
    display: none;
  }

  .scanning-pulse-mobile {
    display: block;
  }

  .status-label {
    display: none;
  }

  .log-drawer {
    width: 100vw !important;
    max-width: 100% !important;
    right: 0 !important;
    left: 0 !important;
    bottom: 40px !important;
    /* Height of status line */
    border-radius: 0 !important;
    max-height: 80vh !important;
  }
}

/* Activity Log Drawer */
.btn-clear {
  background: transparent;
  border: none;
  color: inherit;
  cursor: pointer;
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 11px;
  transition: all 0.2s;
}

.btn-clear:hover {
  background: var(--surface);
}

.btn-clear.active {
  background: var(--accent);
  color: var(--on-primary, #fff);
}

.log-drawer {
  position: absolute;
  bottom: 100%;
  right: 16px;
  width: 450px;
  max-height: 400px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-bottom: none;
  border-radius: 8px 8px 0 0;
  display: flex;
  flex-direction: column;
  box-shadow: 0 -4px 20px rgba(0, 0, 0, 0.5);
  overflow: hidden;
  z-index: 1000;
}

.log-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 16px;
  background: #11111a;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.log-title {
  font-weight: 600;
  color: var(--text);
}

.log-actions {
  display: flex;
  gap: 8px;
}

.log-btn {
  background: var(--surface);
  border: 1px solid var(--border);
  color: var(--text-secondary);
  padding: 4px 10px;
  border-radius: 4px;
  cursor: pointer;
  font-size: 10px;
  transition: all 0.2s;
}

.log-btn:hover {
  background: rgba(255, 255, 255, 0.2);
  color: #fff;
}

.close-btn {
  font-size: 14px;
  padding: 2px 8px;
}

.log-content {
  padding: 12px;
  overflow-y: auto;
  flex: 1;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.log-empty {
  text-align: center;
  color: #6b7280;
  font-style: italic;
  padding: 20px 0;
}

.log-entry {
  display: flex;
  gap: 12px;
  padding: 4px 0;
  border-bottom: 1px solid var(--border);
  font-size: 11px;
}

.log-entry:last-child {
  border-bottom: none;
}

.log-time {
  color: #6b7280;
  flex-shrink: 0;
}

.log-msg {
  color: #d1d5db;
  word-break: break-word;
}

.log-info .log-msg {
  color: #60a5fa;
}

.log-warning .log-msg {
  color: #fbbf24;
}

.log-error .log-msg {
  color: #f87171;
}

.log-success .log-msg {
  color: #4ade80;
}

.slide-up-enter-active,
.slide-up-leave-active {
  transition: all 0.3s cubic-bezier(0.16, 1, 0.3, 1);
}

.slide-up-enter-from,
.slide-up-leave-to {
  transform: translateY(100%);
  opacity: 0;
}
</style>
