<template>
  <div class="results-section">
    <div v-if="preview" class="results-section-header">
      <h2>Latest Duplicate Groups</h2>
      <span v-if="results.length > 0" class="view-all" @click="$emit('viewAll')">
        View All ›
      </span>
    </div>

    <div v-if="!preview" class="results-toolbar">
      <div class="toolbar-left">
        <div class="dropdown">
          <button class="tool-btn">Selection ▾</button>
          <div class="dropdown-content">
            <button @click="selectAll">All</button>
            <button @click="selectNone">None</button>
            <button @click="selectInvert">Invert</button>
            <hr />
            <button @click="selectBest">Select Best (Keep largest)</button>
            <button @click="selectLowest">Select Lowest Quality</button>
          </div>
        </div>
        <div class="dropdown">
          <button class="tool-btn" :disabled="!hasSelection">Actions ▾</button>
          <div class="dropdown-content">
            <button @click="deleteSelected" class="danger">Delete Selected</button>
            <button @click="removeFromList">Remove from List</button>
            <button @click="markNotMatch" :disabled="!canExclude">Mark as "Not a Match"</button>
          </div>
        </div>
        <div class="dropdown">
          <button class="tool-btn">Display ▾</button>
          <div class="dropdown-content">
            <label class="dropdown-check">
              <input type="checkbox" v-model="settings.show_thumbnails" @change="saveDisplaySettings" />
              Show Thumbnails
            </label>
            <label class="dropdown-check">
              <input type="checkbox" v-model="settings.show_similarity" @change="saveDisplaySettings" />
              Show Similarity Score
            </label>
            <label class="dropdown-check">
              <input type="checkbox" v-model="settings.show_media_info" @change="saveDisplaySettings" />
              Show Media Info
            </label>
          </div>
        </div>
        <button v-if="!preview" class="tool-btn" @click="fetchAllThumbnails">Fetch All Thumbnails</button>
      </div>
      <div class="toolbar-right">
        <span v-if="statusMsg" class="status-msg">{{ statusMsg }}</span>
        <span v-if="hasSelection" class="selection-count">{{ Object.keys(selectedItems).length }} selected · {{
          formatSize(selectionTotalSize) }}</span>
      </div>
    </div>

    <div v-if="results.length > 0">
      <div v-for="(group, gi) in displayGroups" :key="group.id" class="group-card"
        :class="{ 'group-identical': isIdenticalGroup(group) }">
        <div class="group-header" :class="{ 'focused': isFocused(gi, -1) }" :id="`group-${gi}`"
          @click="toggleGroup(gi)" @mousedown="focusedItem = { groupIndex: gi, fileIndex: -1 }"
          @contextmenu.prevent="openContextMenu($event, 'group', { gi, group })">
          <div class="group-check" @click.stop>
            <input type="checkbox" :checked="isGroupSelected(group)" @change="toggleGroupSelection(group)" />
          </div>
          <div class="group-icon" :class="iconColor(gi)">
            {{ expanded[gi] ? '▾' : '▸' }}
          </div>
          <div class="group-meta">
            <div class="group-title">Group #{{ gi + 1 }}: "{{ basename(group.files[0]?.path) }}"</div>
            <div class="group-sub">{{ group.files.length }} copies · {{ formatTotalSize(group) }}</div>
          </div>
          <span class="group-badge green" v-if="isIdenticalGroup(group)">Identical Match</span>
          <span class="group-badge red" v-else-if="group.files.length > 3">High</span>
          <span class="group-badge orange" v-else-if="group.files.length > 2">Med</span>
        </div>

        <div v-if="expanded[gi]" class="group-body-table">
          <table class="results-table">
            <thead>
              <tr>
                <th class="col-check" v-if="!preview"></th>
                <th v-if="settings.show_thumbnails" class="col-thumb"
                  :style="{ minWidth: (Math.min(settings.thumbnails, MAX_DISPLAY_THUMBS) * 84) + 'px' }">Thumbnails</th>
                <th class="col-path sortable" @click="toggleSort('path')">
                  Path
                  <span v-if="sortKey === 'path'" class="sort-icon">{{ sortOrder === 'asc' ? '▴' : '▾' }}</span>
                </th>
                <th v-if="settings.show_similarity" class="col-sim sortable" @click="toggleSort('similarity')">
                  Sim.
                  <span v-if="sortKey === 'similarity'" class="sort-icon">{{ sortOrder === 'asc' ? '▴' : '▾' }}</span>
                </th>
                <th v-if="settings.show_media_info" class="col-meta">Media Info</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="(file, fi) in group.files" :key="fi"
                :class="{ 'row-best': fi === bestIndex(group), 'row-selected': isSelected(file.path), 'focused': isFocused(gi, fi) }"
                :id="`file-${gi}-${fi}`" @mousedown="focusedItem = { groupIndex: gi, fileIndex: fi }"
                @contextmenu.prevent="openContextMenu($event, 'file', { gi, fi, file, group })">
                <td v-if="!preview" class="cell-check">
                  <input type="checkbox" :checked="isSelected(file.path)" @change="toggleItem(file.path)" />
                </td>
                <td v-if="settings.show_thumbnails" class="cell-thumb">
                  <div class="thumb-strip">
                    <img v-for="(t, ti) in getFileThumbs(file.path).slice(0, MAX_DISPLAY_THUMBS)" :key="ti" :src="t"
                      class="thumb-img" />
                    <div v-if="!getFileThumbs(file.path).length" class="thumb-placeholder">
                      <button v-if="!settings.auto_fetch_thumbnails" class="fetch-btn"
                        @click.stop="fetchFileThumbnails(file)">
                        Load
                      </button>
                      <span v-else>...</span>
                    </div>
                  </div>
                </td>
                <td class="cell-path" :title="file.path">
                  <div class="path-text">{{ file.path }}</div>
                  <div class="path-actions">
                    <button class="mini-btn" @click="openFile(file.path, $event)">Open</button>
                    <button class="mini-btn" @click="renameFile(file)">Rename</button>
                  </div>
                </td>
                <td v-if="settings.show_similarity" class="cell-sim">
                  <div class="sim-badge" :class="simClass(file.similarity)">
                    {{ file.similarity === 100 ? 'MATCH' : (file.similarity.toFixed(1) + '%') }}
                  </div>
                </td>
                <td v-if="settings.show_media_info" class="cell-meta">
                  <div class="meta-row">
                    <span :class="getClass(file, 'duration', group)">{{ formatDuration(file.duration) }}</span>
                    <span class="meta-sep">|</span>
                    <span :class="getClass(file, 'codec', group)">{{ file.codec || '??' }}</span>
                  </div>
                  <div class="meta-row">
                    <span :class="getClass(file, 'resolution', group)">{{ formatResolution(file) }}</span>
                    <span class="meta-sep">|</span>
                    <span :class="getClass(file, 'fps', group)">{{ formatFPS(file.fps) }}</span>
                  </div>
                  <div class="meta-row">
                    <span :class="getClass(file, 'size', group)">{{ formatSize(file.size) }}</span>
                    <span class="meta-sep">|</span>
                    <span :class="getClass(file, 'bitrate', group)">{{ formatBitrate(file.bitrate) }}</span>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div v-if="!expanded[gi] && group.files.length > 1" class="group-expand" @click="toggleGroup(gi)">
          Click to expand · {{ group.files.length }} files
        </div>
      </div>

      <!-- Pagination/Infinite Scroll Observer -->
      <div v-if="!preview && results.length < totalResults" ref="scrollObserver" class="scroll-observer">
        <div class="loading-spinner"></div>
        <span>Loading more duplicates ({{ results.length }} / {{ totalResults }})...</span>
      </div>
      <div v-else-if="!preview && results.length > 0" class="scroll-end">
        Showing all {{ totalResults }} duplicate groups.
      </div>
    </div>

    <div v-else class="empty-state">
      <div class="empty-icon">🔍</div>
      <h3>No duplicates found yet</h3>
      <p>Run a scan to find duplicate videos.</p>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onUnmounted, inject, watch } from 'vue'
import { GetResults, GetThumbnails, GetSettings, ExcludeGroup, DeleteFiles, OpenFile, RenameFile, GetStreamUrl } from '../api'

const showModal = inject('showModal')

const props = defineProps({ preview: { type: Boolean, default: false } })
const emit = defineEmits(['viewAll', 'open-preview'])

const results = ref([])
const totalResults = ref(0)
const totalFiles = ref(0)
const expanded = ref({})
const thumbnails = ref({})
const settings = ref({
  auto_fetch_thumbnails: true,
  thumbnails: 4,
  show_thumbnails: true,
  show_similarity: true,
  show_media_info: true
})
const MAX_DISPLAY_THUMBS = 7
const selectedItems = ref({}) // { path: true }
const statusMsg = ref('')
const abortController = ref(null)
const scrollObserver = ref(null)
const loading = ref(false)
const sortKey = ref('similarity') // 'similarity', 'size', 'duration', 'path'
const sortOrder = ref('desc') // 'asc', 'desc'

// Keyboard navigation focus state
// fileIndex === -1 means the group header is focused
const focusedItem = ref({ groupIndex: 0, fileIndex: -1 })

const contextMenu = ref({
  show: false,
  x: 0,
  y: 0,
  type: '', // 'file' or 'group'
  data: null
})

const openContextMenu = (e, type, data) => {
  contextMenu.value = {
    show: true,
    x: e.clientX,
    y: e.clientY,
    type,
    data
  }
}

const closeContextMenu = () => {
  contextMenu.value.show = false
}

const isFocused = (gIndex, fIndex) => {
  return focusedItem.value.groupIndex === gIndex && focusedItem.value.fileIndex === fIndex
}

const scrollToFocused = () => {
  setTimeout(() => {
    let id = ''
    if (focusedItem.value.fileIndex === -1) {
      id = `group-${focusedItem.value.groupIndex}`
    } else {
      id = `file-${focusedItem.value.groupIndex}-${focusedItem.value.fileIndex}`
    }
    const el = document.getElementById(id)
    if (el) {
      el.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  }, 10)
}

const handleKeydown = (e) => {
  // Don't intercept if user is typing in an input
  if (['INPUT', 'TEXTAREA'].includes(document.activeElement.tagName)) return

  // Don't intercept if results isn't active/visible
  if (!results.value || results.value.length === 0) return

  const maxGroupIndex = displayGroups.value.length - 1
  let { groupIndex, fileIndex } = focusedItem.value

  const currentGroup = displayGroups.value[groupIndex]
  if (!currentGroup) return

  switch (e.key) {
    case 'ArrowDown':
      e.preventDefault()
      if (fileIndex === -1 && expanded.value[groupIndex] && currentGroup.files.length > 0) {
        // From header -> into first file
        fileIndex = 0
      } else if (fileIndex >= 0 && fileIndex < currentGroup.files.length - 1) {
        // Move to next file in group
        fileIndex++
      } else if (groupIndex < maxGroupIndex) {
        // Move to next group header
        groupIndex++
        fileIndex = -1
      }
      break

    case 'ArrowUp':
      e.preventDefault()
      if (fileIndex > 0) {
        // Move to previous file in group
        fileIndex--
      } else if (fileIndex === 0) {
        // Move from first file -> to its group header
        fileIndex = -1
      } else if (fileIndex === -1 && groupIndex > 0) {
        // Move from header -> to previous group
        groupIndex--
        if (expanded.value[groupIndex] && displayGroups.value[groupIndex].files.length > 0) {
          fileIndex = displayGroups.value[groupIndex].files.length - 1
        } else {
          fileIndex = -1
        }
      }
      break

    case 'ArrowLeft':
      e.preventDefault()
      if (fileIndex !== -1) {
        // Focus parent header
        fileIndex = -1
      } else if (fileIndex === -1 && expanded.value[groupIndex]) {
        // Collapse expanded group
        toggleGroup(groupIndex)
      }
      break

    case 'ArrowRight':
      e.preventDefault()
      if (fileIndex === -1 && !expanded.value[groupIndex]) {
        // Expand collapsed group
        toggleGroup(groupIndex)
      }
      break

    case ' ': // Space
      e.preventDefault()
      if (fileIndex === -1) {
        // Toggle selection for entire group
        toggleGroupSelection(currentGroup)
      } else {
        // Toggle selection for specific file
        toggleItem(currentGroup.files[fileIndex].path)
      }
      break

    default:
      return
  }

  focusedItem.value = { groupIndex, fileIndex }
  scrollToFocused()
}

watch(selectedItems, (newVal) => {
  const selectionSize = results.value.flatMap(g => g.files)
    .filter(f => newVal[f.path])
    .reduce((sum, f) => sum + (f.size || 0), 0)
  emit('selection-change', Object.keys(newVal).length, selectionSize)
}, { deep: true })

const setStatus = (msg, duration = 3000) => {
  statusMsg.value = msg
  if (duration > 0) setTimeout(() => { if (statusMsg.value === msg) statusMsg.value = '' }, duration)
}

const displayGroups = computed(() => {
  let groups = results.value

  if (sortKey.value) {
    groups = [...groups].sort((a, b) => {
      function getVal(g) {
        const best = g.files[bestIndex(g)]
        switch (sortKey.value) {
          case 'similarity':
            const others = g.files.filter(f => f.similarity < 100)
            if (others.length === 0) return 100
            return others.reduce((s, f) => s + f.similarity, 0) / others.length
          case 'size': return best.size
          case 'duration': return best.duration
          case 'path': return best.path
          default: return 0
        }
      }
      const valA = getVal(a)
      const valB = getVal(b)
      const modifier = sortOrder.value === 'asc' ? 1 : -1
      if (valA < valB) return -1 * modifier
      if (valA > valB) return 1 * modifier
      return 0
    })
  }

  if (props.preview) return groups.slice(0, 5)
  return groups
})

const canExclude = computed(() => {
  return Object.keys(selectedItems.value).length >= 2
})

const loadResults = async (append = false) => {
  if (loading.value) return
  loading.value = true
  try {
    const limit = props.preview ? 5 : 50
    const offset = append ? results.value.length : 0

    const [res, s] = await Promise.all([
      GetResults(offset, limit),
      GetSettings()
    ])

    if (res && res.items) {
      if (append) {
        results.value = [...results.value, ...res.items]
      } else {
        results.value = res.items
      }
      totalResults.value = res.total || 0
      totalFiles.value = res.total_files || 0

      if (!props.preview) {
        const newExpanded = { ...expanded.value }
        results.value.forEach((_, i) => {
          if (newExpanded[i] === undefined) newExpanded[i] = true
        })
        expanded.value = newExpanded
      }
    } else {
      results.value = []
      totalResults.value = 0
      totalFiles.value = 0
    }

    if (s) {
      if (s.show_thumbnails === undefined) s.show_thumbnails = true
      if (s.show_similarity === undefined) s.show_similarity = true
      if (s.show_media_info === undefined) s.show_media_info = true
      settings.value = s
    }

  } catch (e) {
    console.error('Error fetching results', e)
    setStatus('Failed to load results')
  } finally {
    loading.value = false
  }
}

const saveDisplaySettings = async () => {
  try {
    await SaveSettings(settings.value)
  } catch (e) {
    console.error("Failed to save display settings", e)
  }
}

onMounted(() => {
  loadResults()

  if (!props.preview) {
    const observer = new IntersectionObserver((entries) => {
      if (entries[0].isIntersecting && results.value.length < totalResults.value && !loading.value) {
        loadResults(true)
      }
    }, { threshold: 0.1 })

    watch(scrollObserver, (el) => {
      if (el) observer.observe(el)
      else observer.disconnect()
    })
  }

  window.addEventListener('keydown', handleKeydown)
  window.addEventListener('mousedown', closeContextMenu)
})

onUnmounted(() => {
  window.removeEventListener('keydown', handleKeydown)
  window.removeEventListener('mousedown', closeContextMenu)
})

const toggleGroup = (index) => {
  expanded.value = { ...expanded.value, [index]: !expanded.value[index] }
  if (expanded.value[index] && settings.value.auto_fetch_thumbnails) {
    fetchGroupThumbnails(results.value[index])
  }
}

const cancelThumbnails = () => {
  if (abortController.value) {
    abortController.value.abort()
    abortController.value = null
  }
}

const fetchGroupThumbnails = async (group, signal) => {
  for (const file of group.files) {
    if (signal?.aborted) return
    await fetchFileThumbnails(file, signal)
  }
}

const fetchFileThumbnails = async (file, signal) => {
  if (!thumbnails.value[file.path]) {
    try {
      const count = Math.min(settings.value.thumbnails || 4, MAX_DISPLAY_THUMBS)
      const thumbs = await GetThumbnails(file.path, file.duration, count, signal)
      thumbnails.value[file.path] = thumbs
    } catch (e) {
      if (e.name !== 'AbortError') {
        console.error('Error fetching thumbnails for', file.path, e)
        thumbnails.value[file.path] = []
      }
    }
  }
}

const fetchAllThumbnails = async () => {
  cancelThumbnails()
  abortController.value = new AbortController()
  const signal = abortController.value.signal

  const allFiles = results.value.flatMap(group => group.files)

  const concurrency = settings.value.concurrency || 4

  // Simple worker pool
  const workers = []

  for (let i = 0; i < concurrency; i++) {
    workers.push((async () => {
      while (allFiles.length > 0 && !signal.aborted) {
        const file = allFiles.shift()
        if (file) {
          await fetchFileThumbnails(file, signal)
        }
      }
    })())
  }

  await Promise.all(workers)
}

defineExpose({ loadResults, getSummary: () => ({ groups: totalResults.value, files: totalFiles.value }), cancelThumbnails })

const getFileThumbs = (path) => thumbnails.value[path] || []

const copyToClipboard = (text) => {
  navigator.clipboard.writeText(text)
  setStatus('Copied to clipboard')
}

const toggleSort = (key) => {
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortKey.value = key
    sortOrder.value = key === 'similarity' ? 'desc' : 'asc'
  }
}

const simClass = (s) => {
  if (s >= 99.5) return 'sim-ref'
  if (s >= 98) return 'sim-high'
  if (s >= 95) return 'sim-med'
  return 'sim-low'
}

const basename = (path) => {
  if (!path) return 'Unknown'
  return path.split('/').pop().split('\\').pop()
}

const iconColor = (i) => {
  const colors = ['blue', 'pink', 'green']
  return colors[i % colors.length]
}

const formatDuration = (secs) => {
  if (!secs || secs <= 0) return '-'
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60)
  const s = Math.floor(secs % 60)
  if (h > 0) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  return `${m}:${String(s).padStart(2, '0')}`
}

const formatResolution = (file) => {
  const w = file.width || file.Width || 0
  const h = file.height || file.Height || 0
  if (!w || !h) return 'N/A'
  const label = (() => {
    const known = [2160, 1440, 1080, 720, 480, 360, 240]
    for (const p of known) {
      if (Math.abs(h - p) <= 8) return `${p}p`
    }
    return `${h}p`
  })()
  return `${w}×${h} (${label})`
}

const formatFPS = (fps) => {
  if (!fps) return '-'
  return fps.toFixed(2) + ' fps'
}

const formatSize = (bytes) => {
  if (!bytes) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const formatBitrate = (br) => {
  if (!br) return '-'
  return Math.round(br / 1000) + ' kbps'
}

const formatTotalSize = (group) => {
  const total = group.files.reduce((sum, f) => sum + (f.size || 0), 0)
  return formatSize(total)
}

const bestIndex = (group) => {
  let best = 0
  for (let i = 1; i < group.files.length; i++) {
    if ((group.files[i].size || 0) > (group.files[best].size || 0)) best = i
  }
  return best
}

const isIdenticalGroup = (group) => {
  if (!group || !group.files || group.files.length < 2) return false
  return group.files.every(f => f.similarity === 100)
}

const getClass = (file, prop, group) => {
  if (group.files.length < 2) return ''

  let val = 0
  let isBest = false
  let isWorst = false

  const getVal = (f) => {
    switch (prop) {
      case 'size': return f.size || 0
      case 'bitrate': return f.bitrate || 0
      case 'duration': return f.duration || 0
      case 'resolution': return (f.width || 0) * (f.height || 0)
      case 'fps': return f.fps || 0
      default: return 0
    }
  }

  const currentVal = getVal(file)
  const vals = group.files.map(getVal)
  const max = Math.max(...vals)
  const min = Math.min(...vals)

  if (max === min) return ''

  if (currentVal === max) isBest = true
  if (currentVal === min) isWorst = true

  if (isBest) return 'text-better'
  if (isWorst) return 'text-worse'
  return ''
}

// Selection Logic
const isSelected = (path) => !!selectedItems.value[path]
const hasSelection = computed(() => Object.keys(selectedItems.value).length > 0)
const selectionTotalSize = computed(() => {
  return results.value.flatMap(g => g.files)
    .filter(f => selectedItems.value[f.path])
    .reduce((sum, f) => sum + (f.size || 0), 0)
})

const toggleItem = (path) => {
  if (selectedItems.value[path]) {
    const newItems = { ...selectedItems.value }
    delete newItems[path]
    selectedItems.value = newItems
  } else {
    selectedItems.value = { ...selectedItems.value, [path]: true }
  }
}

const isGroupSelected = (group) => group.files.every(f => isSelected(f.path))
const deleteItem = async (path) => {
  const confirmed = await showModal({
    title: 'Delete File',
    message: 'Delete this file permanently from disk? This action cannot be undone.',
    confirmLabel: 'Delete',
    isDanger: true
  })

  if (confirmed) {
    try {
      setStatus('Deleting...')
      await DeleteFiles([path])
      const groupIndex = results.value.findIndex(g => g.files.some(f => f.path === path))
      if (groupIndex !== -1) {
        const group = results.value[groupIndex]
        group.files = group.files.filter(f => f.path !== path)
        totalFiles.value--

        if (group.files.length < 2) {
          totalResults.value--
          totalFiles.value -= group.files.length
          results.value.splice(groupIndex, 1)
        }
      }
      const newThumbs = { ...thumbnails.value }
      delete newThumbs[path]
      thumbnails.value = newThumbs
      const newSelected = { ...selectedItems.value }
      delete newSelected[path]
      selectedItems.value = newSelected
      emit('results-changed')
      setStatus('Deleted successfully', 4000)
    } catch (e) {
      console.error(e)
      showModal({ title: 'Delete Failed', message: String(e), type: 'alert' })
    }
  }
}

const openFile = async (path, event) => {
  if (event && event.shiftKey) {
    try {
      await OpenFile(path)
    } catch (e) {
      console.error('Failed to open file externally', e)
    }
    return
  }

  emit('open-preview', path)
}

const renameFile = async (file) => {
  const oldBase = basename(file.path)
  const newName = await showModal({
    title: 'Rename File',
    message: 'Enter a new filename for this file:',
    type: 'prompt',
    defaultValue: oldBase,
    confirmLabel: 'Rename'
  })

  if (newName && newName !== oldBase) {
    const parent = file.path.substring(0, file.path.lastIndexOf(oldBase))
    const newPath = parent + newName
    try {
      setStatus('Renaming...')
      await RenameFile(file.path, newPath)

      results.value = results.value.map(group => ({
        ...group,
        files: group.files.map(f => f.path === file.path ? { ...f, path: newPath } : f)
      }))
      if (thumbnails.value[file.path]) {
        thumbnails.value[newPath] = thumbnails.value[file.path]
        delete thumbnails.value[file.path]
      }

      if (selectedItems.value[file.path]) {
        const newSelected = { ...selectedItems.value }
        delete newSelected[file.path]
        newSelected[newPath] = true
        selectedItems.value = newSelected
      }

      setStatus('Renamed')
    } catch (e) {
      console.error('Rename failed', e)
      showModal({ title: 'Rename Failed', message: String(e), type: 'alert' })
    }
  }
}

const toggleGroupSelection = (group) => {
  if (isGroupSelected(group)) {
    const newItems = { ...selectedItems.value }
    group.files.forEach(f => delete newItems[f.path])
    selectedItems.value = newItems
  } else {
    const newItems = { ...selectedItems.value }
    group.files.forEach(f => newItems[f.path] = true)
    selectedItems.value = newItems
  }
}

const selectAll = () => {
  const newItems = {}
  results.value.forEach(g => {
    g.files.forEach(f => {
      newItems[f.path] = true
    })
  })
  selectedItems.value = newItems
}

const selectNone = () => {
  selectedItems.value = {}
}

const selectInvert = () => {
  const newItems = {}
  results.value.forEach(g => {
    g.files.forEach(f => {
      if (!selectedItems.value[f.path]) {
        newItems[f.path] = true
      }
    })
  })
  selectedItems.value = newItems
}
const selectBest = () => {
  selectNone()
  results.value.forEach(g => {
    const bi = bestIndex(g)
    selectedItems.value[g.files[bi].path] = true
  })
}
const selectLowest = () => {
  selectNone()
  results.value.forEach(g => {
    const bi = bestIndex(g)
    g.files.forEach((f, i) => {
      if (i !== bi) selectedItems.value[f.path] = true
    })
  })
}

const deleteSelected = async () => {
  const paths = Object.keys(selectedItems.value)
  if (!paths.length) return

  const confirmed = await showModal({
    title: 'Delete Selected Files',
    message: `Delete ${paths.length} files (${formatSize(selectionTotalSize.value)}) permanently from disk?`,
    confirmLabel: 'Delete All',
    isDanger: true
  })

  if (confirmed) {
    try {
      setStatus('Deleting...')
      const deletedSize = selectionTotalSize.value
      await DeleteFiles(paths)

      const groupsToRemoveIndices = []
      results.value.forEach((group, idx) => {
        const originalGroupLength = group.files.length
        const remainingFiles = group.files.filter(f => !paths.includes(f.path))
        const removedCount = originalGroupLength - remainingFiles.length

        if (removedCount > 0) {
          totalFiles.value -= removedCount
          group.files = remainingFiles

          if (group.files.length < 2) {
            groupsToRemoveIndices.push(idx)
            totalResults.value--
            totalFiles.value -= group.files.length
          }
        }
      })
      groupsToRemoveIndices.sort((a, b) => b - a).forEach(idx => {
        results.value.splice(idx, 1)
      })

      const newThumbs = { ...thumbnails.value }
      paths.forEach(p => { delete newThumbs[p] })
      thumbnails.value = newThumbs
      selectedItems.value = {}
      emit('results-changed')
      setStatus(`Deleted ${paths.length} files (${formatSize(deletedSize)})`, 5000)
    } catch (e) {
      console.error('Delete failed', e)
      setStatus(`Failed to delete: ${e}`, 5000)
      showModal({ title: 'Delete Failed', message: String(e), type: 'alert' })
    }
  }
}

const markNotMatch = async () => {
  const selectedPaths = Object.keys(selectedItems.value)
  if (selectedPaths.length < 2) {
    showModal({ title: 'Selection Required', message: 'Select at least 2 files.', type: 'alert' })
    return
  }

  try {
    const groupsToExclude = []

    for (const group of results.value) {
      const selectedInGroup = group.files.filter(f => selectedItems.value[f.path])
      if (selectedInGroup.length >= 2) {
        const hashes = selectedInGroup.map(f => f.identifier_hash)
        const label = `Ignored: ${basename(group.files[0].path)}`
        groupsToExclude.push({ label, hashes, paths: selectedInGroup.map(f => f.path) })
      }
    }

    if (groupsToExclude.length === 0) {
      showModal({ title: 'Invalid Selection', message: 'Select at least 2 files within the same group to mark as not a match.', type: 'alert' })
      return
    }

    const confirmed = await showModal({
      title: 'Mark as Not a Match',
      message: `Mark ${groupsToExclude.length} group(s) as false positives? They will be ignored in future scans.`,
      confirmLabel: 'Mark Not Match'
    })

    if (!confirmed) return

    setStatus('Processing exclusions...')
    let done = 0
    const total = groupsToExclude.length
    for (const g of groupsToExclude) {
      done++
      setStatus(`Excluding ${done}/${total}: ${g.label}...`, 0)
      await ExcludeGroup(g.label, g.paths)
    }

    const affectedGroupIndices = []
    results.value.forEach((group, idx) => {
      const selectedInGroup = group.files.filter(f => selectedItems.value[f.path])
      if (selectedInGroup.length >= 2) {
        affectedGroupIndices.push(idx)
      }
    })

    affectedGroupIndices.sort((a, b) => b - a).forEach(idx => {
      const g = results.value[idx]
      totalResults.value--
      totalFiles.value -= g.files.length
      results.value.splice(idx, 1)
    })

    emit('results-changed')
    setStatus(`Marked ${groupsToExclude.length} group(s) as not a match.`, 5000)
    selectedItems.value = {}

  } catch (e) {
    console.error('Mark Not Match failed', e)
    setStatus(`Failed: ${e}`, 5000)
    showModal({ title: 'Error', message: String(e), type: 'alert' })
  }
}

const removeFromList = () => {
  // Track removed items to adjust counts
  const pathsToRemove = Object.keys(selectedItems.value)

  const groupsToRemoveIndices = []
  results.value.forEach((group, idx) => {
    const originalLen = group.files.length
    const remaining = group.files.filter(f => !selectedItems.value[f.path])
    const removedCount = originalLen - remaining.length

    if (removedCount > 0) {
      totalFiles.value -= removedCount
      group.files = remaining

      if (group.files.length < 2) {
        groupsToRemoveIndices.push(idx)
        totalResults.value--
        totalFiles.value -= group.files.length
      }
    }
  })

  groupsToRemoveIndices.sort((a, b) => b - a).forEach(idx => {
    results.value.splice(idx, 1)
  })

  selectedItems.value = {}
  emit('results-changed')
}

</script>

<style scoped>
.group-body-table {
  overflow-x: auto;
}

.results-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}

.results-table th {
  text-align: left;
  padding: 8px 12px;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  color: var(--text-muted);
  border-bottom: 1px solid var(--border);
  background: var(--surface-alt);
}

.results-table td {
  padding: 8px 12px;
  border-bottom: 1px solid var(--border);
  vertical-align: middle;
}

.col-check {
  width: 40px;
  text-align: center;
}

.col-path {
  min-width: 200px;
}

.col-sim {
  width: 80px;
}

.col-meta {
  width: 220px;
}

.sortable {
  cursor: pointer;
  user-select: none;
}

.sortable:hover {
  color: var(--text);
  background: var(--surface-alt) !important;
}

.sort-icon {
  margin-left: 4px;
  font-size: 10px;
  color: var(--accent);
}

.cell-meta {
  font-family: 'JetBrains Mono', 'Consolas', monospace;
  line-height: 1.4;
  white-space: nowrap;
}

.meta-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.meta-sep {
  color: var(--border);
  font-size: 10px;
}

.cell-sim {
  text-align: center;
}

.sim-badge {
  display: inline-block;
  padding: 4px 8px;
  border-radius: 99px;
  font-size: 11px;
  font-weight: 700;
  font-family: 'JetBrains Mono', monospace;
  min-width: 45px;
}

.sim-ref {
  background: rgba(16, 185, 129, 0.1) !important;
  color: #10b981 !important;
  border-color: rgba(16, 185, 129, 0.2) !important;
  font-weight: 700;
}

.group-identical {
  border-color: rgba(16, 185, 129, 0.3) !important;
  box-shadow: 0 4px 12px rgba(16, 185, 129, 0.05);
}

.group-identical .group-header {
  background: rgba(16, 185, 129, 0.03);
}

.group-badge.green {
  background: #10b981;
  color: white;
}

.sim-high {
  background: rgba(74, 222, 128, 0.1);
  color: #4ade80;
  border: 1px solid rgba(74, 222, 128, 0.2);
}

.sim-med {
  background: rgba(251, 191, 36, 0.1);
  color: #fbbf24;
  border: 1px solid rgba(251, 191, 36, 0.2);
}

.sim-low {
  background: rgba(248, 113, 113, 0.1);
  color: #f87171;
  border: 1px solid rgba(248, 113, 113, 0.2);
}

.text-better {
  color: #4ade80;
  /* emerald-400 */
  font-weight: 600;
}

.text-worse {
  color: #f87171;
  /* red-400 */
}

.cell-thumb {
  padding: 4px 8px !important;
}

.thumb-strip {
  display: flex;
  gap: 4px;
  flex-wrap: nowrap;
  padding: 2px 0;
}

.thumb-img {
  width: 80px;
  height: 45px;
  object-fit: contain;
  border-radius: 4px;
  background: #000;
}

.thumb-placeholder {
  width: 80px;
  height: 45px;
  background: var(--surface-alt);
  border: 1px dashed var(--border);
  border-radius: 4px;
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-muted);
}

.fetch-btn {
  background: var(--accent);
  border: 1px solid var(--accent);
  border-radius: 4px;
  padding: 4px 12px;
  font-size: 11px;
  font-weight: 600;
  cursor: pointer;
  color: white;
  box-shadow: 0 2px 4px rgba(var(--accent-rgb), 0.3);
}

.fetch-btn:hover {
  filter: brightness(1.1);
}

.path-text {
  word-break: break-all;
  margin-bottom: 4px;
}

.path-actions {
  display: flex;
  gap: 8px;
  opacity: 0;
  transition: opacity 0.2s;
}

tr:hover .path-actions {
  opacity: 1;
}

.mini-btn {
  background: none;
  border: none;
  color: var(--accent);
  font-size: 11px;
  cursor: pointer;
  padding: 0;
  text-decoration: underline;
}

.tbl-btn {
  background: none;
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  cursor: pointer;
  padding: 3px 8px;
  font-size: 13px;
  transition: all 0.15s ease;
}

.tbl-btn:hover {
  border-color: var(--text-muted);
}

.tbl-btn.danger:hover {
  border-color: var(--danger);
  color: var(--danger);
  background: rgba(var(--danger-rgb), 0.05);
}

.row-best td {
  background: var(--surface-alt);
  border-left: 3px dotted var(--accent);
}

.row-selected td {
  background-color: rgba(var(--accent-rgb), 0.08) !important;
}

/* Toolbar */
.results-toolbar {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 12px 24px;
  margin: -24px -24px 24px -24px;
  border-bottom: 2px solid var(--border);
  position: sticky;
  top: -24px;
  background: rgba(255, 255, 255, 0);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(8px);
  z-index: 10;
}

.toolbar-left {
  display: flex;
  gap: 8px;
}

.tool-btn {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  padding: 6px 14px;
  font-size: 13px;
  cursor: pointer;
  color: var(--text);
}

.tool-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.selection-count {
  font-size: 12px;
  color: var(--text-muted);
}

/* Dropdown */
.dropdown {
  position: relative;
  display: inline-block;
}

.dropdown-content {
  display: none;
  position: absolute;
  background-color: var(--surface);
  min-width: 200px;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  z-index: 40;
  margin-top: 2px;
}

.dropdown-content::before {
  content: "";
  position: absolute;
  top: -10px;
  left: 0;
  right: 0;
  height: 10px;
}

.dropdown:hover .dropdown-content {
  display: block;
}

.dropdown-content button {
  color: var(--text);
  padding: 10px 14px;
  text-decoration: none;
  display: block;
  font-size: 13px;
  cursor: pointer;
  width: 100%;
  text-align: left;
  border: none;
  background: none;
  font-family: inherit;
}

.dropdown-content button:hover {
  background-color: var(--surface-alt);
}

.dropdown-content hr {
  margin: 4px 0;
  border: none;
  border-top: 1px solid var(--border);
}

.dropdown-content button.danger {
  color: var(--danger);
}

.status-msg {
  font-size: 13px;
  color: var(--primary);
  margin-right: 15px;
  font-weight: 600;
  padding: 4px 10px;
  background: var(--surface-alt);
  border-radius: var(--radius-sm);
  border: 1px solid var(--border-alt);
}

.group-check {
  margin-right: 12px;
  display: flex;
  align-items: center;
}

.group-check input,
.cell-check input {
  width: 16px;
  height: 16px;
  cursor: pointer;
}

.scroll-observer {
  padding: 40px;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  color: var(--text-secondary);
  font-size: 14px;
}

.scroll-end {
  padding: 40px;
  text-align: center;
  color: var(--text-muted);
  font-size: 14px;
  border-top: 1px dashed var(--border);
  margin-top: 20px;
}

.loading-spinner {
  width: 30px;
  height: 30px;
  border: 3px solid var(--border);
  border-top-color: var(--accent);
  border-radius: 50%;
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to {
    transform: rotate(360deg);
  }
}

.dropdown-check {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 16px;
  color: var(--text);
  cursor: pointer;
  font-size: 13px;
  transition: background 0.15s ease;
}

.dropdown-check:hover {
  background: var(--surface-alt);
}

.dropdown-check input[type="checkbox"] {
  margin: 0;
  width: 14px;
  height: 14px;
  accent-color: var(--accent);
  cursor: pointer;
}

/* Keyboard Navigation focus */
.group-header.focused,
tr.focused {
  scroll-margin-top: 80px;
  scroll-margin-bottom: 80px;
}

.group-header.focused,
tr.focused td {
  background-color: rgba(var(--accent-rgb), 0.15) !important;
}

tr.focused.row-best td {
  background-color: rgba(var(--accent-rgb), 0.15) !important;
}

.group-identical .group-header.focused {
  background-color: rgba(16, 185, 129, 0.12) !important;
}

/* Context Menu */
.context-menu {
  position: fixed;
  z-index: 3000;
  background: rgba(26, 26, 36, 0.85);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border: 1px solid rgba(255, 255, 255, 0.1);
  border-radius: 12px;
  box-shadow: 0 10px 40px rgba(0, 0, 0, 0.5);
  padding: 6px;
  min-width: 180px;
  animation: context-fade 0.15s ease-out;
}

@keyframes context-fade {
  from {
    opacity: 0;
    transform: translateY(-5px) scale(0.98);
  }

  to {
    opacity: 1;
    transform: translateY(0) scale(1);
  }
}

.menu-item {
  width: 100%;
  padding: 8px 12px;
  background: transparent;
  border: none;
  border-radius: 8px;
  color: #e2e8f0;
  font-size: 13px;
  text-align: left;
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  transition: all 0.2s;
  font-family: inherit;
}

.menu-item:hover {
  background: rgba(var(--accent-rgb), 0.2);
  color: #fff;
}

.menu-icon {
  font-size: 14px;
  width: 20px;
  text-align: center;
  filter: grayscale(0.5);
}

.menu-sep {
  margin: 6px;
  border: none;
  border-top: 1px solid rgba(255, 255, 255, 0.08);
}
</style>
