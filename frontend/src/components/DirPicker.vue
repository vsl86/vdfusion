<template>
  <div class="dir-picker-overlay" @click.self="$emit('close')">
    <div class="dir-picker-modal">
      <div class="picker-header">
        <h3>Select Directory</h3>
        <button class="close-btn" @click="$emit('close')">×</button>
      </div>
      
      <div class="path-bar">
        <input type="text" v-model="currentPath" @keyup.enter="loadDir(currentPath)" />
        <button @click="loadDir(currentPath)">Go</button>
      </div>

      <div class="dir-list" v-if="!loading">
        <div class="dir-item parent" @click="goUp" v-if="currentPath !== '/' && currentPath !== ''">
          <span class="icon">📁</span> .. [Parent Directory]
        </div>
        <div v-for="dir in dirs" :key="dir" class="dir-item" @click="loadDir(joinPath(currentPath, dir))">
          <span class="icon">📁</span> {{ dir }}
        </div>
        <div v-if="dirs.length === 0 && (currentPath === '/' || currentPath === '')" class="empty-msg">
          No subdirectories found.
        </div>
      </div>
      <div v-else class="loading-state">
        Scanning directories...
      </div>

      <div class="picker-footer">
        <button class="btn secondary" @click="$emit('close')">Cancel</button>
        <button class="btn primary" @click="selectCurrent">Select This Folder</button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ListDirs } from '../api'

const props = defineProps({
  initialPath: { type: String, default: '/' }
})

const emit = defineEmits(['close', 'select'])

const currentPath = ref(props.initialPath || '/')
const dirs = ref([])
const loading = ref(false)

const loadDir = async (path) => {
  loading.value = true
  try {
    const res = await ListDirs(path)
    if (res && res.path !== undefined) {
      currentPath.value = res.path
      dirs.value = res.dirs || []
    }
  } catch (e) {
    console.error('Failed to list dirs', e)
    alert('Error accessing directory: ' + e.message)
  } finally {
    loading.value = false
  }
}

const joinPath = (base, sub) => {
  if (base.endsWith('/')) return base + sub
  return base + '/' + sub
}

const goUp = () => {
  const parts = currentPath.value.split('/').filter(Boolean)
  parts.pop()
  loadDir('/' + parts.join('/'))
}

const selectCurrent = () => {
  emit('select', currentPath.value)
}

onMounted(() => {
  loadDir(currentPath.value)
})
</script>

<style scoped>
.dir-picker-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(0, 0, 0, 0.7);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 10000;
  backdrop-filter: blur(4px);
}

.dir-picker-modal {
  width: 500px;
  max-width: 90vw;
  background: var(--surface);
  border-radius: 12px;
  border: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  max-height: 80vh;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
}

.picker-header {
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.picker-header h3 { margin: 0; font-size: 1.1rem; }

.close-btn {
  background: none;
  border: none;
  color: var(--text-muted);
  font-size: 24px;
  cursor: pointer;
}

.path-bar {
  padding: 12px 20px;
  display: flex;
  gap: 8px;
  background: var(--surface-alt);
}

.path-bar input {
  flex: 1;
  background: var(--background);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 6px 12px;
  color: var(--text);
  font-family: inherit;
}

.path-bar button {
  padding: 6px 16px;
  background: var(--accent);
  color: white;
  border: none;
  border-radius: 4px;
  cursor: pointer;
}

.dir-list {
  flex: 1;
  overflow-y: auto;
  padding: 8px 0;
  min-height: 200px;
}

.dir-item {
  padding: 10px 20px;
  cursor: pointer;
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 14px;
  transition: background 0.15s;
}

.dir-item:hover {
  background: var(--surface-alt);
}

.dir-item.parent {
  color: var(--accent);
  font-weight: 500;
  border-bottom: 1px solid var(--border);
  margin-bottom: 4px;
}

.icon { font-size: 16px; }

.loading-state, .empty-msg {
  padding: 40px;
  text-align: center;
  color: var(--text-muted);
}

.picker-footer {
  padding: 16px 20px;
  border-top: 1px solid var(--border);
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.btn {
  padding: 8px 18px;
  border-radius: 6px;
  font-weight: 600;
  cursor: pointer;
  border: 1px solid transparent;
}

.btn.primary { background: var(--accent); color: white; }
.btn.secondary { background: var(--surface-alt); border-color: var(--border); color: var(--text); }
</style>
