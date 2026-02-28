<template>
  <!-- Compact sidebar mode (Scanner tab) -->
  <div v-if="compact" class="card">
    <div class="card-body" style="padding: 16px">
      <h3>Quick Settings</h3>

      <!-- SIMILARITY -->
      <div class="section-label">Comparison</div>
      <div class="filter-row">
        <span>Similarity</span>
        <span>{{ settings.percent }}%</span>
      </div>
      <input type="range" min="50" max="100" step="0.5" v-model.number="settings.percent" class="filter-slider" />

      <div class="filter-row" style="margin-top: 8px">
        <span>Duration Diff</span>
        <span>{{ settings.percent_duration_difference }}%</span>
      </div>
      <input type="range" min="0" max="100" step="1" v-model.number="settings.percent_duration_difference"
        class="filter-slider" />

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
    <div class="dashboard-header" style="display: flex; justify-content: space-between; align-items: flex-start;">
      <div>
        <h1>Settings</h1>
        <p>Configure scan behaviour, thresholds, and filters.</p>
      </div>
      <button class="action-btn primary" :class="{ success: saveSuccess }"
        style="min-width: 120px; transition: all 0.3s ease;" :disabled="!isDirty || saveSuccess" @click="save">
        {{ saveSuccess ? '✓ Saved' : 'Save' }}
      </button>
    </div>

    <div class="card mb-3">
      <div class="card-header">Scan Configuration</div>
      <div class="card-body">
        <div class="setting-group">
          <h4>Thresholds & Filters</h4>
          <div class="setting-row">
            <label>Similarity Threshold: {{ settings.percent }}%</label>
            <input type="range" min="50" max="100" step="0.1" v-model.number="settings.percent" />
          </div>
          <div class="setting-row">
            <label>Duration Difference: {{ settings.percent_duration_difference }}%</label>
            <input type="range" min="0" max="100" step="0.1" v-model.number="settings.percent_duration_difference" />
          </div>
          <div class="setting-row">
            <label>Duration Diff Min (Sec)</label>
            <NumberInput v-model="settings.duration_difference_min_seconds" :min="0" />
          </div>
          <div class="setting-row">
            <label>Duration Diff Max (Sec)</label>
            <NumberInput v-model="settings.duration_difference_max_seconds" :min="0" />
          </div>

          <div class="setting-row" style="margin-top: 10px">
            <label class="filter-check">
              <input type="checkbox" v-model="settings.filter_by_file_size" />
              Filter by file size
            </label>
            <label class="filter-check" style="margin-top: 8px">
              <input type="checkbox" v-model="settings.recheck_suspicious" />
              Recheck Suspicious Files
            </label>
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
            <small v-else>Stable timestamps allow adding more hashes without full re-indexing.</small>
          </div>

          <div class="setting-row" style="margin-top: 15px">
            <label style="margin-bottom: 8px; display: block; font-weight: 600;">Display Options</label>
            <div class="display-toggles">
              <label class="filter-check">
                <input type="checkbox" v-model="settings.show_thumbnails" />
                Show Thumbnails
              </label>
              <label class="filter-check">
                <input type="checkbox" v-model="settings.show_similarity" />
                Show Similarity Score
              </label>
              <label class="filter-check">
                <input type="checkbox" v-model="settings.show_media_info" />
                Show Media Info
              </label>
            </div>
          </div>

          <h4 style="margin-top: 20px">Performance & UI</h4>
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
            <small style="display:block; margin-top:4px">Only check this if diagnosing issues. It prints heavy verbose
              output to the terminal.</small>
          </div>
        </div>
      </div>
    </div>

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

    <!-- Include List -->
    <div class="card mb-3">
      <div class="card-header">Search Directories</div>
      <div class="card-body" style="display:block; padding: 16px">
        <div v-for="(path, i) in settings.include_list" :key="i" class="path-row">
          <input type="text" v-model="settings.include_list[i]" placeholder="/path/to/videos" />
          <button class="icon-btn" @click="openPicker(i)" title="Browse">📂</button>
          <button class="icon-btn danger" @click="removePath(i)">×</button>
        </div>
        <button class="btn secondary btn-small" @click="addPath">+ Add Directory</button>
      </div>

      <DirPicker v-if="pickerVisible" :initial-path="pickerTargetValue" @close="pickerVisible = false"
        @select="onDirSelected" />
    </div>

    <!-- Blacklist List -->
    <div class="card mb-3">
      <div class="card-header">Blacklisted Paths (Substrings)</div>
      <div class="card-body" style="display:block; padding: 16px">
        <div v-for="(p, i) in settings.black_list" :key="'bl-f-' + i" class="path-chip bl">
          <span class="chip-icon">🚫</span>
          <span class="chip-text">{{ p }}</span>
          <button class="chip-remove" @click="removeBL(i)">✕</button>
        </div>
        <div class="add-path-inline" style="margin-top:8px">
          <input v-model="newBL" placeholder="Add blacklist pattern..." @keyup.enter="addBL" />
          <button @click="addBL">+ Add</button>
        </div>
      </div>
    </div>

    <button class="bl-del-btn danger" style="width:100%; margin-top: 10px; padding: 12px; font-weight: 600;"
      @click="resetToDefaults">
      Reset Settings to Defaults
    </button>
  </div>
</template>

<script setup>
import { ref, onMounted, computed, inject } from 'vue'
import { GetSettings, SaveSettings, ResetDB, CleanupDB, ExportDB, ImportDB, ResetSettings } from '../api'
import NumberInput from './NumberInput.vue'
import DirPicker from './DirPicker.vue'

const showModal = inject('showModal')

const props = defineProps({ compact: { type: Boolean, default: false } })

const emit = defineEmits(['openSettings', 'results-changed'])
const settings = ref({
  include_list: [],
  black_list: [],
  percent: 96,
  percent_duration_difference: 20,
  duration_difference_min_seconds: 0,
  duration_difference_max_seconds: 3600,
  thumbnails: 4,
  concurrency: 4,
  auto_fetch_thumbnails: true,
  filter_by_file_size: false,
  minimum_file_size: 0,
  maximum_file_size: 0,
  show_media_info: true,
  show_similarity: true,
  show_thumbnails: true
})

const baseSettings = ref(null)
const saveSuccess = ref(false)
const dbActionLoading = ref(false)

const pickerVisible = ref(false)
const pickerTargetIndex = ref(-1)
const pickerTargetValue = ref('/')

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
      message: `Only ${newVal - oldVal} new frame(s) will be extracted per file. Existing ${oldVal} hashes will be reused.`
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

const addPath = () => {
  if (!settings.value.include_list) settings.value.include_list = []
  settings.value.include_list.push('')
}
const removePath = (i) => settings.value.include_list.splice(i, 1)

const openPicker = (index) => {
  pickerTargetIndex.value = index
  pickerTargetValue.value = settings.value.include_list[index] || '/'
  pickerVisible.value = true
}

const onDirSelected = (path) => {
  if (pickerTargetIndex.value !== -1) {
    settings.value.include_list[pickerTargetIndex.value] = path
  }
  pickerVisible.value = false
}

const addBL = () => {
  if (newBL.value) {
    if (!settings.value.black_list) settings.value.black_list = []
    settings.value.black_list.push(newBL.value.trim())
    newBL.value = ''
  }
}
const removeBL = (i) => settings.value.black_list.splice(i, 1)

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
  background: #10b981 !important;
  border-color: #10b981 !important;
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
  color: var(--text-muted);
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
  background: #fee2e2;
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
  color: var(--text-muted);
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
  background: #fee2e2;
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
  background: rgba(16, 185, 129, 0.08);
  border-color: rgba(16, 185, 129, 0.2);
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
  color: #10b981;
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
</style>
