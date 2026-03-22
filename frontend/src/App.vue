<template>
  <div class="app-shell">
    <!-- Top Navigation -->
    <nav class="top-nav">
      <div class="nav-left">
        <button class="menu-toggle" @click="menuOpen = true" aria-label="Open Menu">
          <span class="hamburger"></span>
        </button>
        <div class="nav-logo">
          <Logo :color="brandingColor" class="logo-img" />
          <span class="logo-text desktop-only">VDFusion</span>
          <span v-if="connectionMode === 'remote'" class="remote-badge">Remote</span>
        </div>
      </div>
      <div class="nav-pills desktop-only">
        <button class="nav-pill" :class="{ active: activeTab === 'scanner' }"
          @click="selectTab('scanner')">Scanner</button>
        <button class="nav-pill" :class="{ active: activeTab === 'results' }"
          @click="selectTab('results')">Results</button>
        <button class="nav-pill" :class="{ active: activeTab === 'blacklist' }"
          @click="selectTab('blacklist')">Blacklist</button>
        <button class="nav-pill" :class="{ active: activeTab === 'log' }" @click="selectTab('log')">Log</button>
        <button class="nav-pill" :class="{ active: activeTab === 'settings' }"
          @click="selectTab('settings')">Settings</button>
      </div>
      <div class="theme-toggle">
        <button class="theme-btn toggle-single" @click="toggleTheme" 
                :title="theme === 'light' ? 'Switch to Dark' : theme === 'dark' ? 'Switch to System' : 'Switch to Light'">
          <span v-if="theme === 'light'">☼</span>
          <span v-else-if="theme === 'dark'">🌑</span>
          <span v-else>A</span>
        </button>
      </div>
    </nav>

    <!-- Side Drawer (Mobile) -->
    <div class="nav-drawer-overlay" v-if="menuOpen" @click="menuOpen = false"></div>
    <div class="nav-drawer" :class="{ open: menuOpen }">
      <div class="drawer-header">
        <div class="drawer-logo">
          <Logo :color="brandingColor" class="logo-img" />
          <span>VDFusion</span>
        </div>
        <button class="close-btn" @click="menuOpen = false">&times;</button>
      </div>
      <div class="drawer-links">
        <button class="drawer-link" :class="{ active: activeTab === 'scanner' }"
          @click="selectTab('scanner')">Scanner</button>
        <button class="drawer-link" :class="{ active: activeTab === 'results' }"
          @click="selectTab('results')">Results</button>
        <button class="drawer-link" :class="{ active: activeTab === 'blacklist' }"
          @click="selectTab('blacklist')">Blacklist</button>
        <button class="drawer-link" :class="{ active: activeTab === 'log' }"
          @click="selectTab('log')">Log</button>
        <button class="drawer-link" :class="{ active: activeTab === 'settings' }"
          @click="selectTab('settings')">Settings</button>
      </div>
    </div>

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
                <p>VDFusion requires <strong>FFmpeg</strong> binaries for video processing, streaming, and external
                  player support.</p>
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
          <ResultsGrid ref="allResultsRef" :preview="false" :scanning="scanning" @open-preview="openVideoPreview"
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
      :selection-count="totalSelectionCount" :selection-size="totalSelectionSize" />

    <!-- Video Preview Modal -->
    <VideoPreview :path="previewPath" @close="previewPath = null" />

    <!-- Global Modal -->
    <Modal v-model:show="modalState.show" :title="modalState.title" :message="modalState.message"
      :type="modalState.type" :confirmLabel="modalState.confirmLabel" :cancelLabel="modalState.cancelLabel"
      :defaultValue="modalState.defaultValue" :placeholder="modalState.placeholder" :isDanger="modalState.isDanger"
      @confirm="onModalConfirm" @cancel="onModalCancel" />

    <transition name="slide-in">
      <div v-if="updateAvailable" class="update-bar" @click="openUpdateLink">
        <span class="update-icon">🚀</span>
        <span class="update-text">New version available: <strong>{{ updateVersion }}</strong></span>
        <span class="update-link">Review changes →</span>
        <button class="close-update" @click.stop="updateAvailable = false">×</button>
      </div>
    </transition>
  </div>
</template>

<script setup>
import { ref, onMounted, watch, watchEffect } from 'vue'
import { StartScan, StopScan, EventsOn, GetScanStatus, GetSuspiciousFiles, GetStats, CheckDependencies, DownloadDependencies, getConnectionConfig, GetDebugInfo, GetResults } from './api'

import ScanSettings from './components/ScanSettings.vue'
import Logo from './components/Logo.vue'
import ProgressBar from './components/ProgressBar.vue'
import ResultsGrid from './components/ResultsGrid.vue'
import BlacklistTab from './components/BlacklistTab.vue'
import LogTab from './components/LogTab.vue'
import VideoPreview from './components/VideoPreview.vue'
import StatusLine from './components/StatusLine.vue'
import Modal from './components/Modal.vue'
import { provide, computed } from 'vue'

const activeTab = ref(localStorage.getItem('vdf_active_tab') || 'scanner')
const connectionMode = ref(getConnectionConfig().mode)
const menuOpen = ref(false)

const selectTab = (tab) => {
  activeTab.value = tab
  menuOpen.value = false
}

// Theme management
const theme = ref(localStorage.getItem('vdf_theme') || 'system')
const setTheme = (t) => {
  theme.value = t
  localStorage.setItem('vdf_theme', t)
}
const toggleTheme = () => {
  if (theme.value === 'light') setTheme('dark')
  else if (theme.value === 'dark') setTheme('system')
  else setTheme('light')
}
const applyTheme = () => {
  let resolved = theme.value
  if (resolved === 'system') {
    resolved = window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light'
  }
  document.documentElement.setAttribute('data-theme', resolved)
}
watchEffect(applyTheme)
onMounted(() => {
  window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', applyTheme)
})

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
const totalSelectionSize = ref(0)
const dependencies = ref({ ffmpeg: true, ffprobe: true, ffplay: true, missing: false })
const downloading = ref(false)
const brandingColor = computed(() => {
  if (!scanning.value) return '#3b82f6'
  const phase = scanState.value.phase?.toLowerCase() || ''
  if (phase === 'discovery' || phase === 'walking') return '#64748b'
  if (phase === 'comparing') return '#28a745'
  return '#3b82f6'
})


const updateAvailable = ref(false)
const updateVersion = ref('')
const updateLink = ref('https://github.com/vsl86/vdfusion/releases')

const updateFavicon = (color) => {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 2000 2000"><circle cx="1000" cy="1000" r="900" fill="#203a54" /><circle cx="1000" cy="1000" r="700" fill="white" /><path d="M756,557.439C711.401,564.39 671.292,596.204 659.759,641C655.832,656.254 656,672.374 656,688C656,697.608 655.433,707.414 656.089,717C657.591,738.919 657,761 657,783L657,884C657,980.663 657.465,1077.34 656.999,1174C656.781,1219.18 653.924,1264.84 657.089,1310C658.205,1325.91 660.602,1343.6 667.981,1358C687.336,1395.76 732.868,1424.45 776,1419.83C803.656,1416.86 826.672,1401.43 850,1387.6C880.998,1369.22 912.096,1350.98 943,1332.45C1058.14,1263.4 1174.88,1197.08 1290,1128C1305.29,1118.83 1320.52,1109.53 1336,1100.7C1372.66,1079.78 1415.22,1059.92 1426.33,1015C1427.25,1011.31 1428.63,1007.83 1428.91,1004C1430.43,983.432 1428.09,960.361 1418.1,942C1403.79,915.696 1376.35,900.618 1351,886.694C1337.12,879.071 1323.58,870.833 1310,862.692C1181.73,785.77 1053.27,709.161 925,632.2C901.016,617.81 876.842,603.694 853,589.089C823.97,571.305 791.666,551.88 756,557.439Z" fill="${color}" /></svg>`;
  const blob = new Blob([svg], { type: 'image/svg+xml' });
  const url = URL.createObjectURL(blob);
  const link = document.querySelector("link[rel*='icon']") || document.createElement('link');
  link.type = 'image/svg+xml';
  link.rel = 'icon';
  link.href = url;
  if (!link.parentNode) document.getElementsByTagName('head')[0].appendChild(link);
}

watch(brandingColor, (color) => {
  updateFavicon(color)
}, { immediate: true })

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

const onSelectionChange = (count, size = 0) => {
  totalSelectionCount.value = count
  totalSelectionSize.value = size
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

  // If not scanning, check if we have results to show the summary
  if (!scanning.value) {
    try {
      const res = await GetResults(0, 5)
      if (res && res.total > 0) {
        hasScannedOnce.value = true
        refreshResults(0) // Duration unknown on refresh, but we want the counts
      }
    } catch (e) {
      console.error('Failed to pre-fetch results', e)
    }
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

  // Check for updates after 3 seconds to not block startup
  setTimeout(() => checkIfUpdateAvailable(true), 3000)
})

const parseVersion = (v) => {
  if (!v) return [0, 0, 0]
  const match = v.match(/(\d+)\.(\d+)\.(\d+)/)
  if (!match) return [0, 0, 0]
  return [parseInt(match[1]), parseInt(match[2]), parseInt(match[3])]
}

const checkIfUpdateAvailable = async (silent = false) => {
  try {
    const debug = await GetDebugInfo()
    const current = debug.version
    if (!current || current.includes('dev')) {
      if (!silent) showModal({ title: 'Updates', message: 'Running a development build — update checks are disabled.', type: 'alert' })
      return
    }

    const resp = await fetch('https://api.github.com/repos/vsl86/vdfusion/releases/latest')
    if (!resp.ok) {
      if (!silent) showModal({ title: 'Update Check Failed', message: 'Could not reach GitHub. Please check your internet connection.', type: 'alert' })
      return
    }
    const latest = await resp.json()
    const latestTag = latest.tag_name

    const cv = parseVersion(current)
    const lv = parseVersion(latestTag)

    let newer = false
    if (lv[0] > cv[0]) newer = true
    else if (lv[0] === cv[0] && lv[1] > cv[1]) newer = true
    else if (lv[0] === cv[0] && lv[1] === cv[1] && lv[2] > cv[2]) newer = true

    if (newer) {
      if (silent) {
        // Startup: show non-intrusive banner
        updateAvailable.value = true
        updateVersion.value = latestTag
        updateLink.value = latest.html_url
      } else {
        // Manual: show modal with action
        const confirmed = await showModal({
          title: '🚀 Update Available',
          message: `A new version ${latestTag} is available (you have ${current}).\n\nWould you like to open the release page?`,
          confirmLabel: 'Open Release Page',
          cancelLabel: 'Later'
        })
        if (confirmed) {
          window.open(latest.html_url, '_blank')
        }
      }
    } else if (!silent) {
      showModal({ title: 'No Updates', message: `You are running the latest version (${current}).`, type: 'alert' })
    }
  } catch (e) {
    console.warn('Failed to check for updates', e)
    if (!silent) showModal({ title: 'Update Check Failed', message: 'Something went wrong while checking for updates.', type: 'alert' })
  }
}

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

const openUpdateLink = () => {
  window.open(updateLink.value, '_blank')
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
  background: rgba(245, 158, 11, 0.15);
  color: var(--warning);
  border: 1px solid rgba(245, 158, 11, 0.3);
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

.nav-drawer, .nav-drawer-overlay {
  display: none;
}

.menu-toggle {
  display: none;
}

.logo-img {
  width: 32px;
  height: 32px;
  flex-shrink: 0;
}

.nav-left {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-text {
  font-weight: 700;
  font-size: 18px;
  color: var(--primary);
  letter-spacing: -0.02em;
}

.nav-logo {
  display: flex;
  align-items: center;
  gap: 8px;
}

.remote-badge {
  background: var(--accent);
  color: var(--on-primary);
  font-size: 10px;
  font-weight: 800;
  text-transform: uppercase;
  padding: 2px 6px;
  border-radius: 4px;
  height: 16px;
  display: flex;
  align-items: center;
  letter-spacing: 0.05em;
  margin-left: 4px;
  box-shadow: 0 0 10px rgba(var(--accent-rgb), 0.3);
  animation: pulse-remote 2s infinite;
  white-space: nowrap;
}

@keyframes pulse-remote {
  0% { opacity: 0.8; }
  50% { opacity: 1; transform: scale(1.05); }
  100% { opacity: 0.8; }
}

@media (max-width: 1024px) {
  .scanner-layout {
    grid-template-columns: 1fr;
  }
  
  .sidebar-settings {
    margin-top: 24px;
  }
}

@media (max-width: 768px) {
  .desktop-only {
    display: none !important;
  }

  .main-content {
    padding: 16px;
  }

  .top-nav {
    padding: 12px 16px;
    height: 60px;
    grid-template-columns: 1fr auto;
  }

  .menu-toggle {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 40px;
    background: var(--surface-alt);
    border: 1px solid var(--border);
    border-radius: 8px;
    cursor: pointer;
    z-index: 100;
  }

  .hamburger {
    position: relative;
    width: 20px;
    height: 2px;
    background: var(--text);
    border-radius: 2px;
  }

  .hamburger::before,
  .hamburger::after {
    content: '';
    position: absolute;
    width: 20px;
    height: 2px;
    background: var(--text);
    left: 0;
    border-radius: 2px;
  }

  .hamburger::before { top: -6px; }
  .hamburger::after { bottom: -6px; }

  /* Drawer Overlay */
  .nav-drawer-overlay {
    display: block;
    position: fixed;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: rgba(0, 0, 0, 0.4);
    z-index: 3000;
    backdrop-filter: blur(4px);
  }

  /* Drawer */
  .nav-drawer {
    display: flex;
    position: fixed;
    top: 0;
    left: -280px;
    width: 280px;
    height: 100%;
    background: var(--surface);
    z-index: 3001;
    transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    box-shadow: 10px 0 30px rgba(0, 0, 0, 0.1);
    flex-direction: column;
  }

  .nav-drawer.open {
    transform: translateX(280px);
  }

  .drawer-header {
    padding: 20px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    border-bottom: 1px solid var(--border);
    margin-bottom: 8px;
  }

  .drawer-logo {
    display: flex;
    align-items: center;
    gap: 10px;
    font-weight: 700;
    font-size: 18px;
  }

  .drawer-logo .logo-img {
    width: 32px;
    height: 32px;
  }

  .close-btn {
    background: var(--surface-alt);
    border: 1px solid var(--border);
    width: 32px;
    height: 32px;
    border-radius: 6px;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 20px;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .drawer-links {
    padding: 12px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .drawer-link {
    width: 100%;
    display: flex;
    align-items: center;
    padding: 14px 16px;
    border-radius: 10px;
    background: transparent;
    border: none;
    color: var(--text-secondary);
    font-size: 15px;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
  }

  .drawer-link:hover {
    background: var(--surface-alt);
    color: var(--primary);
  }

  .drawer-link.active {
    background: var(--primary);
    color: var(--on-primary);
  }
}


@keyframes pulse-remote {
  0% { opacity: 0.8; }
  50% { opacity: 1; transform: scale(1.05); }
  100% { opacity: 0.8; }
}

/* Update Notification Bar */
.update-bar {
  position: fixed;
  top: 60px; /* Below navigation */
  left: 50%;
  transform: translateX(-50%);
  background: var(--accent);
  color: #fff;
  padding: 8px 20px;
  border-radius: 30px;
  box-shadow: 0 4px 15px rgba(0, 0, 0, 0.3);
  display: flex;
  align-items: center;
  gap: 12px;
  z-index: 2000;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid rgba(255, 255, 255, 0.2);
}

.update-bar:hover {
  transform: translateX(-50%) translateY(-2px);
  background: #4f46e5;
}

.update-icon {
  font-size: 16px;
}

.update-text {
  font-size: 13px;
}

.update-link {
  font-size: 11px;
  opacity: 0.8;
  font-weight: 600;
  text-transform: uppercase;
}

.close-update {
  background: rgba(0, 0, 0, 0.2);
  border: none;
  color: #fff;
  width: 20px;
  height: 20px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-size: 14px;
}

.close-update:hover {
  background: rgba(0, 0, 0, 0.4);
}

.slide-in-enter-active, .slide-in-leave-active {
  transition: all 0.4s cubic-bezier(0.175, 0.885, 0.32, 1.1);
}

.slide-in-enter-from, .slide-in-leave-to {
  transform: translateX(-50%) translateY(-100px);
  opacity: 0;
}
</style>
