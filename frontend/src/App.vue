<template>
  <div class="app-shell">
    <!-- Top Navigation -->
    <nav class="top-nav">
      <div class="nav-logo">
        <img src="./assets/logo.svg" alt="VDFusion" class="logo-img" />
      </div>
      <div class="nav-pills">
        <button class="nav-pill" :class="{ active: activeTab === 'scanner' }"
          @click="activeTab = 'scanner'">Scanner</button>
        <button class="nav-pill" :class="{ active: activeTab === 'results' }"
          @click="activeTab = 'results'">Results</button>
        <button class="nav-pill" :class="{ active: activeTab === 'blacklist' }"
          @click="activeTab = 'blacklist'">Blacklist</button>
        <button class="nav-pill" :class="{ active: activeTab === 'log' }" @click="activeTab = 'log'">Log</button>
        <button class="nav-pill" :class="{ active: activeTab === 'settings' }"
          @click="activeTab = 'settings'">Settings</button>
      </div>
      <div style="width: 42px"></div>
    </nav>

    <!-- Main Content -->
    <div class="main-content">
      <!-- Scanner Tab -->
      <div v-show="activeTab === 'scanner'">
        <div class="scanner-layout">
          <div class="scanner-main">
            <div class="dashboard-header">
              <h1>Scanner Dashboard</h1>
              <p>Manage your scan tasks and monitor progress.</p>
            </div>

            <div class="action-row">
              <button class="action-btn primary" @click="scanning ? stopScan() : startScan()">
                {{ scanning ? 'Stop Scan' : 'Start New Scan' }}
              </button>
            </div>

            <ProgressBar :scanning="scanning" :initial-state="scanState" @stop="stopScan" />

            <!-- Statistics Card -->
            <div v-if="!scanning" class="card stats-card">
              <div class="card-body">
                <div class="summary-header">
                  <div class="summary-icon"></div>
                  <div class="summary-title">Library Statistics</div>
                  <button class="action-btn icon-only" title="Refresh stats" @click="fetchStats">Refresh</button>
                </div>
                <div class="summary-stats">
                  <div class="stat-item">
                    <div class="stat-value">{{ stats.total_files.toLocaleString() }}</div>
                    <div class="stat-label">Total Files</div>
                  </div>
                  <div class="stat-divider"></div>
                  <div class="stat-item">
                    <div class="stat-value">{{ formatSize(stats.total_size) }}</div>
                    <div class="stat-label">Total Size</div>
                  </div>
                  <div class="stat-divider"></div>
                  <div class="stat-item">
                    <div class="stat-value">{{ formatDuration(stats.total_duration) }}</div>
                    <div class="stat-label">Total Playtime</div>
                  </div>
                </div>
              </div>
            </div>

            <!-- Scan Summary Card -->
            <div v-if="hasScannedOnce && !scanning" class="card summary-card" @click="activeTab = 'results'">
              <div class="card-body">
                <div class="summary-header">
                  <div class="summary-icon">📊</div>
                  <div class="summary-title">Latest Scan Summary</div>
                  <div class="summary-time">{{ formatDuration(lastScanStats.duration) }}</div>
                </div>
                <div class="summary-stats">
                  <div class="stat-item">
                    <div class="stat-value">{{ lastScanStats.groups }}</div>
                    <div class="stat-label">Duplicate Groups</div>
                  </div>
                  <div class="stat-divider"></div>
                  <div class="stat-item">
                    <div class="stat-value">{{ lastScanStats.files }}</div>
                    <div class="stat-label">Total Duplicates</div>
                  </div>
                </div>
                <div class="summary-footer">
                  Click to view detailed results →
                </div>
              </div>
            </div>

            <!-- Suspicious Files Card -->
            <div v-if="suspiciousFiles.length > 0 && !scanning" class="card suspicious-card">
              <div class="card-body">
                <div class="summary-header">
                  <div class="summary-icon">⚠️</div>
                  <div class="summary-title">Suspicious Files</div>
                  <div class="suspicious-count">{{ suspiciousFiles.length }} file{{ suspiciousFiles.length === 1 ? '' :
                    's' }}</div>
                </div>
                <p class="suspicious-desc">These files have FFmpeg warnings that may indicate quality issues or encoding
                  problems.</p>
                <div class="suspicious-list" v-if="showSuspiciousExpanded">
                  <div v-for="f in suspiciousFiles" :key="f.path" class="suspicious-item">
                    <div class="suspicious-path">{{ f.path }}</div>
                    <div v-for="w in f.warnings" :key="w.message" class="warning-entry">
                      <span class="warning-tag">{{ w.message }}</span>
                      <div v-if="w.fix" class="fix-cmd" @click="copyToClipboard(w.fix)" title="Click to copy">
                        <code>{{ w.fix }}</code>
                      </div>
                    </div>
                  </div>
                </div>
                <div class="suspicious-actions">
                  <button class="action-btn small" @click="showSuspiciousExpanded = !showSuspiciousExpanded">
                    {{ showSuspiciousExpanded ? 'Collapse' : 'Expand List' }}
                  </button>
                  <button class="action-btn small primary" @click="saveSuspiciousList">
                    💾 Save List
                  </button>
                </div>
              </div>
            </div>

            <!-- Empty state if no scan yet -->
            <div v-if="!hasScannedOnce && !scanning && stats.total_files === 0" class="empty-dashboard">
              <div class="empty-icon">📂</div>
              <h3>Ready to Scan</h3>
              <p>Add some folders in settings and click "Start New Scan" to begin.</p>
            </div>

            <!-- Missing Dependencies Alert -->
            <div v-if="dependencies.missing && !downloading" class="card warning-card">
              <div class="card-body">
                <div class="summary-header">
                  <div class="summary-icon">🛠️</div>
                  <div class="summary-title">Missing Dependencies</div>
                </div>
                <p>VDFusion requires <strong>FFmpeg</strong> binaries for video processing, streaming, and external player support.</p>
                <div class="tools-status">
                  <span :class="{ 'text-success': dependencies.ffmpeg, 'text-danger': !dependencies.ffmpeg }">
                    • FFmpeg: {{ dependencies.ffmpeg ? 'Ready' : 'Missing' }}
                  </span>
                  <span :class="{ 'text-success': dependencies.ffprobe, 'text-danger': !dependencies.ffprobe }">
                    • FFprobe: {{ dependencies.ffprobe ? 'Ready' : 'Missing' }}
                  </span>
                  <span :class="{ 'text-success': dependencies.ffplay, 'text-danger': !dependencies.ffplay }">
                    • FFplay: {{ dependencies.ffplay ? 'Ready' : 'Missing' }}
                  </span>
                </div>
                <div class="suspicious-actions" style="margin-top: 15px">
                  <button class="action-btn primary" @click="startDownload">
                    Install Automatically
                  </button>
                </div>
              </div>
            </div>

            <!-- Download Progress Overlay -->
            <div v-if="downloading" class="card info-card">
              <div class="card-body">
                <div class="summary-header">
                  <div class="summary-icon">📥</div>
                  <div class="summary-title">{{ downloadProgress.message || 'Starting Download...' }}</div>
                </div>
                <div class="download-progress-container">
                  <div class="download-progress-bar" :style="{ width: (downloadProgress.progress * 100) + '%' }"></div>
                </div>
                <p class="text-muted" style="font-size: 12px; margin-top: 10px; text-align: center;">
                  {{ Math.round(downloadProgress.progress * 100) }}%
                </p>
              </div>
            </div>
          </div>

          <div class="sidebar-settings">
            <ScanSettings ref="sidebarSettingsRef" :compact="true" @openSettings="activeTab = 'settings'" />
          </div>
        </div>
      </div>

      <!-- Results Tab -->
      <div v-show="activeTab === 'results'">
        <div>
          <div class="dashboard-header">
            <h1>All Results</h1>
            <p>Browse and manage all duplicate groups found.</p>
          </div>
          <ResultsGrid ref="allResultsRef" :preview="false" @open-preview="openVideoPreview"
            @selection-change="onSelectionChange" @results-changed="refreshScanSummary" />
        </div>
      </div>

      <!-- Blacklist Tab -->
      <div v-show="activeTab === 'blacklist'">
        <BlacklistTab ref="blacklistTabRef" />
      </div>

      <!-- Log Tab -->
      <div v-show="activeTab === 'log'">
        <LogTab />
      </div>

      <!-- Settings Tab -->
      <div v-show="activeTab === 'settings'">
        <ScanSettings ref="mainSettingsRef" :compact="false" />
      </div>
    </div>

    <!-- Status Line -->
    <StatusLine :stats="stats" :scanning="scanning" :scan-state="scanState" :results-summary="lastScanStats"
      :selection-count="totalSelectionCount" />

    <!-- Video Preview Modal -->
    <VideoPreview :path="previewPath" @close="previewPath = null" />

    <!-- Global Modal -->
    <Modal v-model:show="modalState.show" :title="modalState.title" :message="modalState.message"
      :type="modalState.type" :confirmLabel="modalState.confirmLabel" :cancelLabel="modalState.cancelLabel"
      :defaultValue="modalState.defaultValue" :placeholder="modalState.placeholder" :isDanger="modalState.isDanger"
      @confirm="onModalConfirm" @cancel="onModalCancel" />
  </div>
</template>

<script setup>
import { ref, onMounted, watch } from 'vue'
import { StartScan, StopScan, EventsOn, GetScanStatus, GetSuspiciousFiles, GetStats, CheckDependencies, DownloadDependencies } from './api'

import ScanSettings from './components/ScanSettings.vue'
import ProgressBar from './components/ProgressBar.vue'
import ResultsGrid from './components/ResultsGrid.vue'
import BlacklistTab from './components/BlacklistTab.vue'
import LogTab from './components/LogTab.vue'
import VideoPreview from './components/VideoPreview.vue'
import StatusLine from './components/StatusLine.vue'
import Modal from './components/Modal.vue'
import { provide, computed } from 'vue'

const activeTab = ref(localStorage.getItem('vdf_active_tab') || 'scanner')
const scanning = ref(false)
const scanState = ref({ current: 0, total: 0, phase: '', last_file: '' })
const hasScannedOnce = ref(false)
const resultsGridRef = ref(null)
const allResultsRef = ref(null)
const blacklistTabRef = ref(null)
const sidebarSettingsRef = ref(null)
const mainSettingsRef = ref(null)
const previewPath = ref(null)
const suspiciousFiles = ref([])
const showSuspiciousExpanded = ref(false)
const totalSelectionCount = ref(0)
const dependencies = ref({ ffmpeg: true, ffprobe: true, ffplay: true, missing: false })
const downloading = ref(false)
const downloadProgress = ref({ message: '', progress: 0 })
const isWails = !!window.go

const stats = ref({
  total_files: 0,
  total_size: 0,
  total_duration: 0,
  suspicious_count: 0
})

const lastScanStats = ref({
  duration: 0,
  groups: 0,
  files: 0
})

const modalState = ref({
  show: false,
  title: '',
  message: '',
  type: 'confirm',
  confirmLabel: 'Confirm',
  cancelLabel: 'Cancel',
  defaultValue: '',
  placeholder: '',
  isDanger: false,
  resolve: null
})

const showModal = (options) => {
  return new Promise((resolve) => {
    modalState.value = {
      title: 'Confirm',
      message: '',
      type: 'confirm',
      confirmLabel: 'Confirm',
      cancelLabel: 'Cancel',
      defaultValue: '',
      placeholder: '',
      isDanger: false,
      ...options,
      show: true,
      resolve
    }
  })
}

const onModalConfirm = (value) => {
  if (modalState.value.resolve) modalState.value.resolve(value !== undefined ? value : true)
  modalState.value.show = false
}

const onModalCancel = () => {
  if (modalState.value.resolve) modalState.value.resolve(false)
  modalState.value.show = false
}

provide('showModal', showModal)

const openVideoPreview = (path) => {
  previewPath.value = path
}

watch(activeTab, (val) => {
  localStorage.setItem('vdf_active_tab', val)
  if (val === 'blacklist' && blacklistTabRef.value) {
    blacklistTabRef.value.refresh()
  }
  if (val === 'scanner' && sidebarSettingsRef.value) {
    sidebarSettingsRef.value.refresh()
  }
  if (val === 'settings' && mainSettingsRef.value) {
    mainSettingsRef.value.refresh()
  }
})

const formatDuration = (totalSeconds) => {
  if (!totalSeconds || totalSeconds <= 0) return '0s'

  const secondsInMinute = 60
  const secondsInHour = 60 * secondsInMinute
  const secondsInDay = 24 * secondsInHour
  const secondsInWeek = 7 * secondsInDay
  const secondsInMonth = 30.44 * secondsInDay // Average month length
  const secondsInYear = 365.25 * secondsInDay // Average year length

  if (totalSeconds < secondsInMinute) {
    return `${totalSeconds.toFixed(0)}s`
  } else if (totalSeconds < secondsInHour) {
    const m = Math.floor(totalSeconds / secondsInMinute)
    const s = Math.floor(totalSeconds % secondsInMinute)
    return `${m}m ${s}s`
  } else if (totalSeconds < secondsInDay) {
    const h = Math.floor(totalSeconds / secondsInHour)
    const m = Math.floor((totalSeconds % secondsInHour) / secondsInMinute)
    return `${h}h ${m}m`
  } else if (totalSeconds < secondsInWeek) {
    const d = Math.floor(totalSeconds / secondsInDay)
    const h = Math.floor((totalSeconds % secondsInDay) / secondsInHour)
    return `${d}d ${h}h`
  } else if (totalSeconds < secondsInMonth) {
    const w = Math.floor(totalSeconds / secondsInWeek)
    const d = Math.floor((totalSeconds % secondsInWeek) / secondsInDay)
    return `${w}w ${d}d`
  } else if (totalSeconds < secondsInYear) {
    const mo = Math.floor(totalSeconds / secondsInMonth)
    const d = Math.floor((totalSeconds % secondsInMonth) / secondsInDay)
    return `${mo}mo ${d}d`
  } else {
    const y = Math.floor(totalSeconds / secondsInYear)
    const mo = Math.floor((totalSeconds % secondsInYear) / secondsInMonth)
    return `${y}y ${mo}mo`
  }
}

const refreshResults = async (duration = 0) => {
  if (allResultsRef.value) {
    await allResultsRef.value.loadResults()
    const summary = allResultsRef.value.getSummary()
    lastScanStats.value = {
      duration: duration,
      groups: summary.groups,
      files: summary.files
    }
  }
  // Fetch suspicious files after scan
  try {
    const files = await GetSuspiciousFiles()
    suspiciousFiles.value = files || []
  } catch (e) {
    console.error('Failed to fetch suspicious files', e)
  }
}

const refreshScanSummary = () => {
  if (allResultsRef.value) {
    const summary = allResultsRef.value.getSummary()
    lastScanStats.value = {
      ...lastScanStats.value,
      groups: summary.groups,
      files: summary.files
    }
  }
}

const onSelectionChange = (count) => {
  totalSelectionCount.value = count
}

const fetchStats = async () => {
  try {
    const s = await GetStats()
    stats.value = s
  } catch (e) {
    console.error('Failed to fetch stats', e)
  }
}

onMounted(async () => {
  fetchStats()
  // Check if a scan is already running on the server
  try {
    const status = await GetScanStatus()
    if (status && status.running) {
      scanning.value = true
      scanState.value = {
        current: status.current,
        total: status.total,
        phase: status.phase,
        last_file: status.last_file || ''
      }
      activeTab.value = 'scanner'
    } else if (status && status.phase === 'completed') {
      hasScannedOnce.value = true
      refreshResults(status.duration_seconds || 0)
    }
  } catch (e) {
    console.error('Failed to get scan status', e)
  }

  EventsOn('scan_progress', (data) => {
    scanState.value = {
      current: data.current,
      total: data.total,
      phase: data.phase,
      last_file: data.last_file || ''
    }

    if (data.phase === 'completed' || data.phase.startsWith('error') || data.phase === 'stopped') {
      scanning.value = false
      if (data.phase === 'completed') {
        hasScannedOnce.value = true
        refreshResults(data.duration_seconds || 0)
      }
      fetchStats()
    } else {
      scanning.value = true
    }
  })

  EventsOn('download_progress', (data) => {
    downloadProgress.value = data
  })

  if (isWails) {
    checkDeps()
  }
})

const checkDeps = async () => {
  try {
    const status = await CheckDependencies()
    dependencies.value = status
  } catch (e) {
    console.error('Failed to check dependencies', e)
  }
}

const startDownload = async () => {
  downloading.value = true
  try {
    await DownloadDependencies()
    await checkDeps()
  } catch (e) {
    console.error('Download failed', e)
    showModal({
      title: 'Download Failed',
      message: 'FFmpeg download failed. Please check your internet connection or install FFmpeg manually.',
      type: 'alert'
    })
  } finally {
    downloading.value = false
  }
}

const formatSize = (bytes) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const startScan = async () => {
  if (allResultsRef.value) {
    allResultsRef.value.cancelThumbnails()
  }
  scanning.value = true
  try {
    await StartScan([])
  } catch (e) {
    console.error(e)
    scanning.value = false
  }
}

const stopScan = async () => {
  await StopScan()
}

const rescan = async () => {
  startScan()
}

const saveSuspiciousList = () => {
  const lines = suspiciousFiles.value.map(f => {
    const warns = f.warnings.map(w => {
      let line = `  Warning: ${w.message}`
      if (w.fix) line += `\n  Fix: ${w.fix}`
      return line
    })
    return `${f.path}\n${warns.join('\n')}`
  })
  const text = `Suspicious Files Report\n${'='.repeat(40)}\n\n${lines.join('\n\n')}\n`
  const blob = new Blob([text], { type: 'text/plain' })
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = 'suspicious_files.txt'
  a.click()
  URL.revokeObjectURL(url)
}

const copyToClipboard = (text) => {
  navigator.clipboard.writeText(text)
}
</script>

<style scoped>
/* Layout moved to style.css */

/* Summary Card Styles */
.summary-card {
  margin-top: 24px;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
  border: 1px solid var(--border);
  background: var(--surface);
}

.summary-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 16px rgba(0, 0, 0, 0.1);
  border-color: var(--accent);
}

.summary-header {
  display: flex;
  align-items: center;
  margin-bottom: 20px;
}

.summary-title {
  flex-grow: 1;
  font-weight: 600;
  font-size: 16px;
}

.summary-time {
  font-size: 14px;
  color: var(--text-muted);
  background: var(--surface-alt);
  padding: 4px 10px;
  border-radius: 20px;
}

.summary-stats {
  display: flex;
  justify-content: space-around;
  align-items: center;
  padding: 20px 0;
  background: var(--surface-alt);
  border-radius: var(--radius-sm);
  margin-bottom: 16px;
}

.stat-item {
  text-align: center;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  color: var(--accent);
  line-height: 1;
  margin-bottom: 4px;
}

.stat-label {
  font-size: 12px;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.05em;
}

.stat-divider {
  width: 1px;
  height: 40px;
  background: var(--border);
}

.summary-footer {
  text-align: right;
  font-size: 13px;
  color: var(--accent);
  font-weight: 500;
}

.empty-dashboard {
  text-align: center;
  padding: 60px 20px;
  color: var(--text-muted);
}

.empty-dashboard .empty-icon {
  font-size: 48px;
  margin-bottom: 16px;
  opacity: 0.5;
}

/* Suspicious Files Card */
.suspicious-card {
  margin-top: 16px;
  border: 1px solid var(--warning, #e8a83e);
  background: var(--surface);
  min-width: 0;
  overflow: hidden;
}

.suspicious-count {
  font-size: 13px;
  color: var(--warning, #e8a83e);
  background: rgba(232, 168, 62, 0.1);
  padding: 4px 10px;
  border-radius: 20px;
  font-weight: 600;
}

.suspicious-desc {
  font-size: 13px;
  color: var(--text-muted);
  margin-bottom: 12px;
}

.suspicious-list {
  max-height: 300px;
  overflow-y: auto;
  margin-bottom: 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 8px;
  background: var(--surface-alt);
}

.suspicious-item {
  padding: 8px;
  border-bottom: 1px solid var(--border);
}

.suspicious-item:last-child {
  border-bottom: none;
}

.suspicious-path {
  font-size: 12px;
  font-family: monospace;
  word-break: break-all;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--text);
  margin-bottom: 4px;
}

.suspicious-warnings {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}

.warning-tag {
  font-size: 11px;
  padding: 3px 8px;
  border-radius: 4px;
  background: #fff3cd;
  color: #664d03;
  border: 1px solid #ffda6a;
}

.warning-entry {
  margin-bottom: 6px;
}

.fix-cmd {
  margin-top: 4px;
  padding: 4px 8px;
  background: var(--surface-alt);
  border: 1px solid var(--border);
  border-radius: 4px;
  cursor: pointer;
  transition: background 0.15s;
}

.fix-cmd:hover {
  background: var(--border);
}

.fix-cmd code {
  font-size: 11px;
  word-break: break-all;
  color: var(--accent);
}

.suspicious-actions {
  display: flex;
  gap: 8px;
  justify-content: flex-end;
}

.action-btn.small {
  font-size: 12px;
  padding: 6px 12px;
}

/* Warning Card for Dependencies */
.warning-card {
  border: 1px solid var(--warning, #e8a83e);
  background: rgba(232, 168, 62, 0.05);
  margin-top: 16px;
}

.tools-status {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin-top: 12px;
  font-size: 14px;
}

.text-success {
  color: #28a745;
}

.text-danger {
  color: #dc3545;
}

/* Info Card for Downloads */
.info-card {
  border: 1px solid var(--accent);
  background: rgba(var(--accent-rgb, 52, 152, 219), 0.05);
  margin-top: 16px;
}

.download-progress-container {
  width: 100%;
  height: 8px;
  background: var(--surface-alt);
  border-radius: 4px;
  overflow: hidden;
  margin-top: 10px;
}

.download-progress-bar {
  height: 100%;
  background: linear-gradient(90deg, var(--accent), #5dade2);
  transition: width 0.3s ease;
}

@media (max-width: 768px) {
  .top-nav {
    padding: 0 12px;
  }
}
</style>
