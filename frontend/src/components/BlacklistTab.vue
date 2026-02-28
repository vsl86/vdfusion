<template>
  <div style="max-width: 900px; margin: 0 auto">
    <div class="dashboard-header">
      <h1>Manual Exclusions</h1>
      <p>Manage file groups you've explicitly marked as "Not a match".</p>
    </div>

    <div class="card">
      <div class="card-header">
        <div style="display: flex; align-items: center; gap: 12px">
          <span>Ignored/Excluded Groups</span>
          <span v-if="loading" class="loading-spinner">Refreshing...</span>
        </div>
        <div style="display: flex; align-items: center; gap: 12px">
          <button class="mini-btn danger-text" v-if="ignoredGroups.length > 0" @click="purgeAll" :disabled="loading">🗑
            Purge All</button>
          <button class="mini-btn" @click="refresh" :disabled="loading">⟳ Refresh</button>
          <span style="font-size: 12px; color: var(--text-muted)">{{ ignoredGroups.length }} groups</span>
        </div>
      </div>
      <div class="card-body" style="padding: 0">
        <table class="bl-table">
          <thead>
            <tr>
              <th style="width: 50px">#</th>
              <th>Label / Files in Group</th>
              <th style="width: 100px; text-align: center">Actions</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="(group, i) in ignoredGroups" :key="group.id">
              <td class="bl-num">{{ i + 1 }}</td>
              <td class="bl-path">
                <div style="font-weight: 600; margin-bottom: 4px">{{ group.label }}</div>
                <div v-for="(h, idx) in group.identifier_hashes" :key="h" class="bl-sub-item">
                  <span class="bl-path-text">{{ (group.resolved_paths && group.resolved_paths[idx]) ?
                    group.resolved_paths[idx] : '(File not found)' }}</span>
                </div>
              </td>
              <td class="bl-actions">
                <button class="bl-del-btn" @click="deleteIgnored(group.id)" title="Remove Exclusion">✕</button>
              </td>
            </tr>
            <tr v-if="ignoredGroups.length === 0">
              <td colspan="3" class="bl-empty">No manual exclusions yet. Mark groups as "Not a match" in results.</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

  </div>
</template>

<script setup>
import { ref, onMounted, inject } from 'vue'
import { GetIgnoredGroups, DeleteIgnoredGroup, PurgeBlacklist } from '../api'

const showModal = inject('showModal')

const ignoredGroups = ref([])
const loading = ref(false)

onMounted(async () => {
  refresh()
})

defineEmits(['openSettings'])

const refresh = async () => {
  loading.value = true
  try {
    const ig = await GetIgnoredGroups()
    ignoredGroups.value = ig || []
  } catch (e) {
    console.error(e)
  } finally {
    loading.value = false
  }
}

const deleteIgnored = async (id) => {
  const confirmed = await showModal({
    title: 'Remove Exclusion',
    message: 'Remove this exclusion? These files may appear as duplicates again in future scans.',
    confirmLabel: 'Remove',
    isDanger: true
  })

  if (confirmed) {
    try {
      loading.value = true
      await DeleteIgnoredGroup(id)
      await refresh()
    } catch (e) {
      console.error(e)
      loading.value = false
    }
  }
}

const purgeAll = async () => {
  const confirmed = await showModal({
    title: 'Purge All Exclusions',
    message: 'Remove ALL manual exclusions? This will clear everything from this list. Continue?',
    confirmLabel: 'Purge All',
    isDanger: true
  })

  if (confirmed) {
    try {
      loading.value = true
      await PurgeBlacklist()
      await refresh()
    } catch (e) {
      console.error(e)
      loading.value = false
    }
  }
}

defineExpose({ refresh })
</script>

<style scoped>
.danger-text {
  color: var(--danger) !important;
}

.bl-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.bl-table th {
  text-align: left;
  padding: 10px 16px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.05em;
  color: var(--text-muted);
  border-bottom: 1px solid var(--border);
  background: var(--surface-alt);
}

.bl-table td {
  padding: 10px 16px;
  border-bottom: 1px solid var(--border);
}

.bl-num {
  color: var(--text-muted);
  font-size: 12px;
}

.bl-path {
  word-break: break-all;
}

.bl-actions {
  text-align: center;
}

.bl-del-btn {
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  cursor: pointer;
  padding: 4px 10px;
  font-size: 12px;
  color: var(--text-muted);
  transition: all 0.15s ease;
}

.bl-del-btn:hover {
  color: var(--danger);
  border-color: var(--danger);
}

.bl-empty {
  text-align: center;
  color: var(--text-muted);
  padding: 32px 16px !important;
}

.bl-sub-item {
  font-size: 12px;
  color: var(--text-secondary);
  padding-left: 12px;
  border-left: 2px solid var(--border);
  margin-bottom: 4px;
  display: flex;
  flex-direction: column;
}

.bl-path-text {
  font-family: 'Inter', sans-serif;
}

.bl-hash-text {
  font-family: 'JetBrains Mono', monospace;
  font-size: 10px;
  color: var(--text-muted);
  opacity: 0.7;
}

.loading-spinner {
  font-size: 11px;
  color: var(--accent);
  font-weight: 500;
  animation: pulse 1.5s infinite;
}

@keyframes pulse {

  0%,
  100% {
    opacity: 1;
  }

  50% {
    opacity: 0.5;
  }
}

.mini-btn {
  background: var(--surface-alt);
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  padding: 4px 10px;
  font-size: 11px;
  font-weight: 600;
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.15s ease;
}

.mini-btn:hover:not(:disabled) {
  background: var(--border);
  color: var(--text);
  border-color: var(--text-muted);
}

.mini-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
