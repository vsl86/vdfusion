<template>
  <!-- Compact sidebar mode (Scanner tab) -->
  <div v-if="compact" class="card">
    <div class="card-body" style="padding: 16px">
      <h3>Quick Settings</h3>

      <!-- SIMILARITY -->
      <div class="section-label">Comparison</div>
      <div style="margin-bottom: 24px;">
        <div class="filter-row" style="margin-bottom: 8px;">
          <span>Similarity Threshold</span>
        </div>
        <SliderFine v-model="settings.percent" :min="50" :max="100" :step="0.1" suffix="%" />
      </div>

      <div style="margin-bottom: 16px;">
        <div class="filter-row" style="margin-bottom: 8px;">
          <span>Duration Difference</span>
        </div>
        <SliderFine v-model="settings.percent_duration_difference" :min="0" :max="100" :step="0.1" suffix="%" />
      </div>

      <button class="manage-bl-btn" :class="{ success: saveSuccess }"
        style="margin-top:14px; background: var(--accent); border-color: var(--accent); color: white; border-radius: var(--radius-xs); width: 100%; transition: all 0.3s ease;"
        :disabled="!isDirty || saveSuccess" @click="save">
        {{ saveSuccess ? '✓ Saved' : 'Save' }}
      </button>

      <button class="add-dir-btn" style="margin-top:10px" @click="$emit('openSettings')">
        All Settings →
      </button>
    </div>
  </div>

  <!-- Full settings page mode -->
  <div v-else class="settings-page">
    <div class="dashboard-header"
      style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 24px;">
      <div>
        <h1>Settings</h1>
        <p>Configure scan behaviour, thresholds, and filters.</p>
      </div>
      <div style="display: flex; flex-direction: column; align-items: flex-end; gap: 8px;">
        <button class="action-btn primary" :class="{ success: saveSuccess }"
          style="min-width: 120px; transition: all 0.3s ease;" :disabled="!isDirty || saveSuccess" @click="save">
          {{ saveSuccess ? '✓ Saved' : 'Save' }}
        </button>
        <div v-if="versionInfo" class="version-display" style="font-size: 11px; color: var(--text-muted);">
          Version: {{ versionInfo.current }}
          <button class="check-update-btn" @click="checkUpdates" :disabled="updateLoading"
            style="margin-left: 8px; background: none; border: 1px solid var(--border); border-radius: 4px; padding: 2px 6px; cursor: pointer; color: var(--text-muted);">
            {{ updateLoading ? '...' : 'Check Updates' }}
          </button>
          <div v-if="updateAvailable" style="margin-top: 6px;">
            <a :href="updateAvailable.url" target="_blank"
              style="color: var(--accent); text-decoration: none; font-weight: 600;">
              🚀 Update Available: {{ updateAvailable.latest }}
            </a>
          </div>
        </div>
      </div>
    </div>

    <div class="card mb-3">
      <div class="card-header">Scan Configuration</div>
      <div class="card-body">
        <div class="setting-group">
          <h4>Thresholds & Filters</h4>
          <div class="setting-row">
            <div class="filter-row" style="margin-top: 0; margin-bottom: 12px;">
              <span style="font-weight: 500;">Similarity Threshold</span>
            </div>
            <SliderFine v-model="settings.percent" :min="50" :max="100" :step="0.1" suffix="%" />
          </div>
          <div class="setting-row" style="margin-top: 24px;">
            <div class="filter-row" style="margin-top: 0; margin-bottom: 12px;">
              <span style="font-weight: 500;">Duration Difference</span>
            </div>
            <SliderFine v-model="settings.percent_duration_difference" :min="0" :max="100" :step="0.1" suffix="%" />
          </div>
          <div class="setting-row">
            <label class="filter-check">
              <input type="checkbox" v-model="settings.filter_by_duration" />
              Limit time difference
            </label>
            <div v-if="settings.filter_by_duration" class="impact-preview" style="margin-top: 8px;">
              <span class="impact-icon">⏱️</span>
              <div class="impact-content">
                <strong>Duration Filtering Active</strong>
                <p>Videos will only be matched if their lengths are within this absolute range.</p>
              </div>
            </div>

            <div v-if="settings.filter_by_duration" style="margin-top: 8px; padding-left: 24px">
              <div class="setting-row">
                <label>Duration Diff Min (Sec)</label>
                <NumberInput v-model="settings.duration_difference_min_seconds" :min="0" />
              </div>
              <div class="setting-row">
                <label>Duration Diff Max (Sec)</label>
                <NumberInput v-model="settings.duration_difference_max_seconds" :min="0" />
              </div>
            </div>
          </div>

          <div class="setting-row" style="margin-top: 10px">
            <label class="filter-check">
              <input type="checkbox" v-model="settings.filter_by_file_size" />
              Filter by file size
            </label>
            <div v-if="settings.filter_by_file_size" class="impact-preview" style="margin-top: 8px;">
              <span class="impact-icon">ℹ️</span>
              <div class="impact-content">
                <strong>File Size Filtering Active</strong>
                <p>Videos outside these size limits will be completely ignored during the scan.</p>
              </div>
            </div>

            <div v-if="settings.filter_by_file_size" style="margin-top: 8px; padding-left: 24px">
              <div class="setting-row">
                <label>Min File Size (MB)</label>
                <NumberInput v-model="minSizeMB" :min="0" :step="1" />
              </div>
              <div class="setting-row">
                <label>Max File Size (MB)</label>
                <NumberInput v-model="maxSizeMB" :min="0" :step="1" />
              </div>
            </div>

            <label class="filter-check" style="margin-top: 12px">
              <input type="checkbox" v-model="settings.recheck_suspicious" />
              Recheck Suspicious Files
            </label>
            <div v-if="settings.recheck_suspicious" class="impact-preview warning" style="margin-top: 8px;">
              <span class="impact-icon">⚠️</span>
              <div class="impact-content">
                <strong>Recheck Enabled</strong>
                <p>This will clear the ignored status on previously marked false positives.</p>
              </div>
            </div>

            <label class="filter-check" style="margin-top: 12px">
              <input type="checkbox" v-model="settings.cleanup_orphans" />
              Cleanup missing files from library
            </label>
            <div v-if="settings.cleanup_orphans" class="impact-preview danger" style="margin-top: 8px;">
              <span class="impact-icon">🗑️</span>
              <div class="impact-content">
                <strong>Destructive Action</strong>
                <p>Removes records for files not found during scan. It might affect files on currently disconnected
                  network mounts.</p>
              </div>
            </div>
          </div>
        </div>

        <div class="setting-group">
          <div class="setting-row">
            <label>Count of Thumbnails (per file)</label>
            <NumberInput v-model="settings.thumbnails" :min="1" :max="32" />
            <div v-if="scanImpact" class="impact-preview" :class="scanImpact.level">
              <span class="impact-icon">{{ scanImpact.icon }}</span>
              <div class="impact-content">
                <strong>{{ scanImpact.title }}</strong>
                <p>{{ scanImpact.message }}</p>
              </div>
            </div>
            <small v-else>Number of frames extracted for the comparison engine. Note: The results UI only displays a maximum of 7 thumbnails.</small>
          </div>

          <div class="setting-row">
            <label>Scan Concurrency (Worker Threads)</label>
            <NumberInput v-model="settings.concurrency" :min="1" :max="16" />
            <small>Lower this if you experience "Resource temporarily unavailable" errors</small>
          </div>
          <div class="setting-row">
            <label class="filter-check">
              <input type="checkbox" v-model="settings.auto_fetch_thumbnails" />
              Auto-fetch thumbnails in results
            </label>
            <small style="display:block; margin-top:4px">If disabled, you'll need to click a button to load thumbnails
              for each group.</small>
          </div>
          <div class="setting-row" style="margin-top: 12px; padding-top: 12px; border-top: 1px dashed var(--border);">
            <label class="filter-check" style="color: var(--danger);">
              <input type="checkbox" v-model="settings.debug_logging" />
              Enable Backend Debug Logging
            </label>
            <div v-if="settings.debug_logging" class="impact-preview danger" style="margin-top: 8px;">
              <span class="impact-icon">🐞</span>
              <div class="impact-content">
                <strong>Verbose Output</strong>
                <p>Only check this if diagnosing issues. It prints heavy verbose output to the terminal.</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Include List -->
    <div class="card mb-3">
      <div class="card-header">Search Directories</div>
      <div class="card-body" style="display:block; padding: 16px">
        <div v-for="(path, i) in settings.include_list" :key="'inc-' + i" class="path-row">
          <PathInput v-model="settings.include_list[i]" placeholder="/path/to/videos" />
          <button class="icon-btn" @click="openPicker('include', i)" title="Browse">📂</button>
          <button class="icon-btn danger" @click="removePath('include', i)">×</button>
        </div>
        <button class="btn secondary btn-small" @click="addPath('include')">+ Add Directory</button>
      </div>
    </div>

    <!-- Blacklist List -->
    <div class="card mb-3">
      <div class="card-header">Blacklisted Directories</div>
      <div class="card-body" style="display:block; padding: 16px">
        <div v-for="(p, i) in settings.black_list" :key="'bl-' + i" class="path-row">
          <PathInput v-model="settings.black_list[i]" placeholder="/path/to/exclude" />
          <button class="icon-btn" @click="openPicker('exclude', i)" title="Browse">📂</button>
          <button class="icon-btn danger" @click="removePath('exclude', i)">×</button>
        </div>
        <button class="btn secondary btn-small" @click="addPath('exclude')">+ Add Directory</button>
      </div>
    </div>

    <!-- Global DirPicker -->
    <DirPicker v-if="pickerVisible" :initial-path="pickerTargetValue" @close="pickerVisible = false"
      @select="onDirSelected" />

    <!-- DB Management -->
    <div class="card mb-3">
      <div class="card-header">Database Management</div>
      <div class="card-body db-mgmt-body" style="padding: 16px">
        <div class="db-mgmt-row">
          <div class="db-info">
            <strong>Cleanup Database</strong>
            <p>Remove records for files that no longer exist on disk.</p>
          </div>
          <button class="bl-del-btn" @click="cleanupDB" :disabled="dbActionLoading">Cleanup</button>
        </div>
        <div class="db-mgmt-row">
          <div class="db-info">
            <strong>Export Database</strong>
            <p>Save a copy of your library connection and metadata.</p>
          </div>
          <button class="bl-del-btn" @click="exportDB" :disabled="dbActionLoading">Export</button>
        </div>
        <div class="db-mgmt-row">
          <div class="db-info">
            <strong>Import Database</strong>
            <p>Overwrite the current database with a backup file.</p>
          </div>
          <button class="bl-del-btn danger" @click="importDB" :disabled="dbActionLoading">Import</button>
        </div>
        <div class="db-mgmt-row">
          <div class="db-info">
            <strong>Reset Database</strong>
            <p>Clear all indexed files and metadata. ⚠️ Irreversible.</p>
          </div>
          <button class="bl-del-btn danger" @click="resetDB" :disabled="dbActionLoading">Reset</button>
        </div>
      </div>
    </div>

    <button class="bl-del-btn danger"
      style="width:100%; margin-top: 10px; margin-bottom: 24px; padding: 12px; font-weight: 600;"
      @click="resetToDefaults">
      Reset Settings to Defaults
    </button>

    <!-- Backend Connection: ONLY SHOW IN DESKTOP (Wails) MODE -->
    <div v-if="isWails" class="card mb-3" style="margin-top: 24px; border-top: 3px solid var(--accent);">
      <div class="card-header">Backend Connection</div>
      <div class="card-body" ref="connBlock">
        <div class="setting-group">
          <div class="connection-toggle-container">
            <button class="conn-toggle" :class="{ active: connectionConfig.mode === 'local' }"
              @click="connectionConfig.mode = 'local'">
              Local Desktop
            </button>
            <button class="conn-toggle" :class="{ active: connectionConfig.mode === 'remote' }"
              @click="connectionConfig.mode = 'remote'">
              Remote Server
            </button>
          </div>

          <div v-if="connectionConfig.mode === 'remote'" class="remote-config-area">
            <label style="display: block; font-size: 13px; margin-bottom: 8px;">Server Address (URL)</label>
            <div style="display: flex; gap: 8px;">
              <input v-model="connectionConfig.url" class="remote-url-input" placeholder="http://192.168.1.10:8080"
                @keyup.enter="saveConnection" />
              <button class="action-btn secondary" style="padding: 0 16px; height: 36px;" @click="testConnection"
                :disabled="testLoading">
                {{ testLoading ? '...' : 'Test' }}
              </button>
            </div>
            <small style="display: block; margin-top: 8px; color: var(--text-secondary);">
              Connecting to a remote host will reload the app. Local filesystem pickers will be restricted to the
              server's path environment.
            </small>
            <div v-if="testResult" class="test-badge" :class="testResult.success ? 'success' : 'error'">
              {{ testResult.message }}
            </div>
          </div>

          <button v-if="connDirty" class="action-btn primary" style="width: 100%; margin-top: 16px;"
            @click="saveConnection">
            Apply & Restart Connection
          </button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, computed, inject, watch, nextTick } from 'vue'
import { GetSettings, SaveSettings, ResetDB, CleanupDB, ExportDB, ImportDB, ResetSettings, getConnectionConfig, setConnectionConfig, GetDebugInfo, CheckForUpdates } from '../api'
import NumberInput from './NumberInput.vue'
import SliderFine from './SliderFine.vue'
import DirPicker from './DirPicker.vue'
import PathInput from './PathInput.vue'

const showModal = inject('showModal')

// Determine if we are running in Wails (desktop context)
const isWails = !!window.go

const props = defineProps({ compact: { type: Boolean, default: false } })

const emit = defineEmits(['openSettings', 'results-changed'])
const settings = ref({
  include_list: [],
  black_list: [],
  percent: 96,
  percent_duration_difference: 20,
  filter_by_duration: false, // New virtual switch or added to backend config later
  duration_difference_min_seconds: 0,
  duration_difference_max_seconds: 3600,
  thumbnails: 4,
  concurrency: 4,
  auto_fetch_thumbnails: true,
  filter_by_file_size: false,
  minimum_file_size: 0,
  maximum_file_size: 0,
  cleanup_orphans: false
})

const baseSettings = ref(null)
const saveSuccess = ref(false)
const dbActionLoading = ref(false)

const pickerVisible = ref(false)
const pickerType = ref('include') // 'include' or 'exclude'
const pickerTargetIndex = ref(-1)
const pickerTargetValue = ref('/')

const connectionConfig = ref(getConnectionConfig())
const baseConnConfigStr = ref(JSON.stringify(connectionConfig.value))
const connBlock = ref(null)

watch(() => connectionConfig.value.mode, async (newMode) => {
  if (newMode === 'remote') {
    await nextTick()
    if (connBlock.value) {
      connBlock.value.scrollIntoView({ behavior: 'smooth', block: 'center' })
    }
  }
})

const testLoading = ref(false)
const testResult = ref(null)
const versionInfo = ref(null)
const updateLoading = ref(false)
const updateAvailable = ref(null)

const checkUpdates = async () => {
  updateLoading.value = true
  try {
    const res = await CheckForUpdates()
    versionInfo.value = res
    if (res.update_available) {
      updateAvailable.value = res
    } else {
      updateAvailable.value = null
      showModal({ title: 'Up to Date', message: 'You are running the latest version.', type: 'alert' })
    }
  } catch (e) {
    console.error('Failed to check updates', e)
  } finally {
    updateLoading.value = false
  }
}

const connDirty = computed(() => {
  return JSON.stringify(connectionConfig.value) !== baseConnConfigStr.value
})

const testConnection = async () => {
  if (!connectionConfig.value.url) {
    testResult.value = { success: false, message: 'Please enter a server URL first' }
    return
  }
  testLoading.value = true
  testResult.value = null
  try {
    // Override fetch to test against prospective URL
    const originalBase = localStorage.getItem('vdf_connection_config');
    localStorage.setItem('vdf_connection_config', JSON.stringify(connectionConfig.value));

    // We can't easily re-init the whole API module without a reload, 
    // but GetDebugInfo already respects the dynamic base
    const info = await GetDebugInfo(connectionConfig.value.url)
    testResult.value = { success: true, message: `Success! Connected to version ${info.version || 'unknown'}` }

    // Restore
    if (originalBase) localStorage.setItem('vdf_connection_config', originalBase);
    else localStorage.removeItem('vdf_connection_config');
  } catch (e) {
    testResult.value = { success: false, message: `Failed: ${e.message}` }
  } finally {
    testLoading.value = false
  }
}

const saveConnection = () => {
  setConnectionConfig(connectionConfig.value)
}

const isDirty = computed(() => {
  if (!baseSettings.value) return false
  return JSON.stringify(settings.value) !== JSON.stringify(baseSettings.value)
})

const scanImpact = computed(() => {
  if (!baseSettings.value || settings.value.thumbnails === baseSettings.value.thumbnails) return null

  const oldVal = baseSettings.value.thumbnails
  const newVal = settings.value.thumbnails

  if (newVal > oldVal) {
    return {
      level: 'upgrade',
      icon: '⚡',
      title: 'Partial Rescan (Incremental)',
      message: `Only ${newVal - oldVal} new frame(s) will be extracted per file. Existing ${oldVal} hashes will be reused. UI will be capped at 7 frames.`
    }
  } else {
    return {
      level: 'recalc',
      icon: '✨',
      title: 'Instant Recalculation',
      message: `No new extraction needed. Will instantly reuse the first ${newVal} existing hashes.`
    }
  }
})

const minSizeMB = computed({
  get: () => Math.round(settings.value.minimum_file_size / (1024 * 1024)),
  set: (val) => { settings.value.minimum_file_size = val * 1024 * 1024 }
})

const maxSizeMB = computed({
  get: () => Math.round(settings.value.maximum_file_size / (1024 * 1024)),
  set: (val) => { settings.value.maximum_file_size = val * 1024 * 1024 }
})

const newInc = ref('')
const newBL = ref('')

onMounted(async () => {
  refresh()
  try {
    const info = await GetDebugInfo()
    versionInfo.value = { current: info.version }
  } catch (e) { }
})

const refresh = async () => {
  try {
    const s = await GetSettings()
    if (s) {
      settings.value = s
      baseSettings.value = JSON.parse(JSON.stringify(s))
    }
  } catch (e) { console.error('Error loading settings', e) }
}

const addPath = (type) => {
  const list = type === 'include' ? 'include_list' : 'black_list'
  if (!settings.value[list]) settings.value[list] = []
  settings.value[list].push('')
}

const removePath = (type, i) => {
  const list = type === 'include' ? 'include_list' : 'black_list'
  settings.value[list].splice(i, 1)
}

const openPicker = (type, index) => {
  const list = type === 'include' ? 'include_list' : 'black_list'
  pickerType.value = type
  pickerTargetIndex.value = index
  pickerTargetValue.value = settings.value[list][index] || '/'
  pickerVisible.value = true
}

const onDirSelected = (path) => {
  if (pickerTargetIndex.value !== -1) {
    const list = pickerType.value === 'include' ? 'include_list' : 'black_list'
    settings.value[list][pickerTargetIndex.value] = path
  }
  pickerVisible.value = false
}

const save = async () => {
  try {
    await SaveSettings(settings.value)
    baseSettings.value = JSON.parse(JSON.stringify(settings.value))
    saveSuccess.value = true
    setTimeout(() => {
      saveSuccess.value = false
    }, 2000)
  } catch (e) {
    showModal({ title: 'Save Failed', message: 'Error saving settings: ' + e.message, type: 'alert' })
  }
}

const resetToDefaults = async () => {
  const confirmed = await showModal({
    title: 'Reset Settings',
    message: 'Reset ALL settings to their default values? Your search directories and blacklist will be preserved, but thresholds and other options will be reset.',
    confirmLabel: 'Reset to Defaults',
    isDanger: true
  })

  if (confirmed) {
    try {
      await ResetSettings()
      await refresh()
      showModal({ title: 'Settings Reset', message: 'Settings have been restored to defaults.', type: 'alert' })
    } catch (e) {
      showModal({ title: 'Reset Failed', message: String(e), type: 'alert' })
    }
  }
}

const resetDB = async () => {
  const confirmed = await showModal({
    title: 'Reset Database',
    message: 'Are you sure you want to RESET the database? This will clear all indexed data and metadata. This action is irreversible!',
    confirmLabel: 'Reset Everything',
    isDanger: true
  })

  if (confirmed) {
    dbActionLoading.value = true
    try {
      await ResetDB()
      showModal({ title: 'Reset Successful', message: 'Database has been cleared.', type: 'alert' })
    } catch (e) {
      showModal({ title: 'Reset Failed', message: String(e), type: 'alert' })
    }
    dbActionLoading.value = false
  }
}

const cleanupDB = async () => {
  dbActionLoading.value = true
  try {
    const res = await CleanupDB()
    showModal({
      title: 'Cleanup Complete',
      message: `${res.removed_count} orphaned entries removed from the database.`,
      type: 'alert'
    })
  } catch (e) {
    showModal({ title: 'Cleanup Failed', message: String(e), type: 'alert' })
  }
  dbActionLoading.value = false
}

const exportDB = async () => {
  dbActionLoading.value = true
  try {
    await ExportDB()
  } catch (e) {
    showModal({ title: 'Export Failed', message: String(e), type: 'alert' })
  }
  dbActionLoading.value = false
}

const importDB = async () => {
  const confirmed = await showModal({
    title: 'Import Database',
    message: 'WARNING: Importing a database will replace your current library completely! Any unsaved configurations or currently scanning tasks will be interrupted. Continue?',
    confirmLabel: 'Overwrite Database',
    isDanger: true
  })

  if (confirmed) {
    dbActionLoading.value = true
    try {
      await ImportDB()
      showModal({ title: 'Import Successful', message: 'The database has been updated.', type: 'alert' })
      emit('results-changed') // Trigger a reload at the app level
      setTimeout(() => window.location.reload(), 1500) // Easiest way to fully reinstate the state
    } catch (e) {
      showModal({ title: 'Import Failed', message: String(e), type: 'alert' })
    }
    dbActionLoading.value = false
  }
}

defineExpose({ refresh })
</script>

<style scoped>
.path-chip.bl {
  border-color: #fee2e2;
  background: #fef2f2;
}

.save-btn.success,
.manage-bl-btn.success {
  background: var(--success) !important;
  border-color: var(--success) !important;
  color: white !important;
}

.db-mgmt-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.db-mgmt-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding-bottom: 12px;
  border-bottom: 1px solid var(--border);
}

.db-mgmt-row:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.db-info strong {
  display: block;
  font-size: 14px;
}

.db-info p {
  margin: 0;
  font-size: 12px;
  color: var(--text-muted);
}

.bl-del-btn {
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  cursor: pointer;
  padding: 6px 14px;
  font-size: 12px;
  color: var(--text);
  transition: all 0.15s ease;
}

.bl-del-btn:hover {
  background: var(--surface-alt);
}

.bl-del-btn.danger {
  color: var(--danger);
  border-color: var(--danger);
}

.bl-del-btn.danger:hover {
  background: rgba(239, 68, 68, 0.1);
}

.path-row {
  display: flex;
  gap: 8px;
  margin-bottom: 8px;
  align-items: center;
}

.path-row input {
  flex: 1;
  background: var(--background);
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  padding: 6px 12px;
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
}

.icon-btn {
  background: var(--surface-alt);
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  cursor: pointer;
  padding: 4px 10px;
  font-size: 14px;
  color: var(--text);
  transition: all 0.15s ease;
  display: flex;
  align-items: center;
  justify-content: center;
  height: 32px;
  width: 32px;
}

.icon-btn:hover {
  border-color: var(--text-muted);
  background: var(--surface);
}

.icon-btn.danger:hover {
  border-color: var(--danger);
  color: var(--danger);
  background: rgba(239, 68, 68, 0.1);
}

.btn-small {
  padding: 4px 12px;
  font-size: 12px;
}

.impact-preview {
  margin-top: 10px;
  padding: 10px 14px;
  border-radius: var(--radius-sm);
  display: flex;
  gap: 12px;
  align-items: flex-start;
  border: 1px solid transparent;
  animation: slideDown 0.2s ease-out;
}

.impact-preview.upgrade {
  background: rgba(var(--accent-rgb, 99, 102, 241), 0.08);
  border-color: rgba(var(--accent-rgb, 99, 102, 241), 0.2);
}

.impact-preview.recalc {
  background: rgba(34, 197, 94, 0.08);
  border-color: rgba(34, 197, 94, 0.2);
}

.impact-preview.warning {
  background: rgba(245, 158, 11, 0.08);
  border-color: rgba(245, 158, 11, 0.2);
}

.impact-preview.danger {
  background: rgba(239, 68, 68, 0.08);
  border-color: rgba(239, 68, 68, 0.2);
}

.impact-icon {
  font-size: 18px;
}

.impact-content strong {
  display: block;
  font-size: 13px;
  margin-bottom: 2px;
}

.impact-preview.upgrade strong {
  color: var(--accent);
}

.impact-preview.recalc strong {
  color: var(--success);
}

.impact-preview.warning strong {
  color: var(--warning);
}

.impact-preview.danger strong {
  color: var(--danger);
}

.impact-content p {
  margin: 0;
  font-size: 11px;
  color: var(--text-secondary);
  line-height: 1.4;
}

@keyframes slideDown {
  from {
    opacity: 0;
    transform: translateY(-5px);
  }

  to {
    opacity: 1;
    transform: translateY(0);
  }
}

.display-toggles {
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding: 8px;
  background: var(--surface-alt);
  border-radius: var(--radius-xs);
  border: 1px solid var(--border);
}

.filter-check {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  cursor: pointer;
}

.filter-check input {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

/* Connection Settings Styles */
.connection-toggle-container {
  display: flex;
  gap: 4px;
  background: var(--bg);
  padding: 4px;
  border-radius: 10px;
  border: 1px solid var(--border);
  margin-bottom: 20px;
}

.conn-toggle {
  flex: 1;
  padding: 10px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
  border-radius: 8px;
  transition: all 0.2s;
}

.conn-toggle:hover:not(.active) {
  background: var(--surface-alt);
  color: var(--text);
}

.conn-toggle.active {
  background: var(--surface);
  color: var(--accent);
  border-color: var(--border);
  box-shadow: var(--shadow);
}

.remote-config-area {
  padding: 16px;
  background: var(--surface-alt);
  border-radius: 12px;
  border: 1px dashed var(--border);
}

.remote-url-input {
  flex: 1;
  height: 36px;
  background: var(--background);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0 12px;
  color: var(--text);
  font-family: inherit;
  font-size: 13px;
}

.test-badge {
  margin-top: 12px;
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
}

.test-badge.success {
  background: rgba(16, 185, 129, 0.1);
  color: #10b981;
  border: 1px solid rgba(16, 185, 129, 0.2);
}

.test-badge.error {
  background: rgba(239, 68, 68, 0.1);
  color: var(--danger);
  border: 1px solid rgba(239, 68, 68, 0.2);
}
</style>
