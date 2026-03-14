<template>
  <div class="path-input-wrapper">
    <input type="text" :value="modelValue" @input="onInput" @keydown="onKeyDown" @keyup.enter="onEnter"
      :placeholder="placeholder" ref="inputRef" autocomplete="off" spellcheck="false" />

    <!-- Suggestions Dropdown -->
    <div v-if="showSuggestions && suggestions.length > 0" class="suggestions-dropdown" ref="suggestionsRef">
      <div v-for="(s, idx) in suggestions" :key="s" class="suggestion-item" :class="{ selected: idx === selectedIndex }"
        @mousedown.prevent="selectSuggestion(s)" @mouseover="selectedIndex = idx">
        <span class="icon">📁</span> {{ s }}
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch, nextTick, onMounted } from 'vue'
import { ListDirs } from '../api'

const props = defineProps({
  modelValue: String,
  placeholder: { type: String, default: 'Type a path...' }
})

const emit = defineEmits(['update:modelValue', 'submit'])

const suggestions = ref([])
const showSuggestions = ref(false)
const selectedIndex = ref(0)
const inputRef = ref(null)
const suggestionsRef = ref(null)

let lastFetchedParent = null
let allParentDirs = []

const onInput = (e) => {
  const value = e.target.value
  emit('update:modelValue', value)
  handleAutocompletion(value)
}

const handleAutocompletion = async (path) => {
  const lastSlash = path.lastIndexOf('/')

  if (lastSlash === -1) {
    showSuggestions.value = false
    return
  }

  const parent = path.substring(0, lastSlash) || '/'
  const prefix = path.substring(lastSlash + 1).toLowerCase()

  if (parent !== lastFetchedParent) {
    try {
      const res = await ListDirs(parent)
      allParentDirs = res.dirs || []
      lastFetchedParent = parent
    } catch (e) {
      allParentDirs = []
      lastFetchedParent = null
    }
  }

  suggestions.value = allParentDirs.filter(d => d.toLowerCase().startsWith(prefix))
  showSuggestions.value = suggestions.value.length > 0
  selectedIndex.value = 0
}

const selectSuggestion = (name) => {
  const lastSlash = props.modelValue.lastIndexOf('/')
  const parent = props.modelValue.substring(0, lastSlash)
  const newPath = (parent.endsWith('/') ? parent : parent + '/') + name
  emit('update:modelValue', newPath)
  showSuggestions.value = false
  emit('submit', newPath)
}

const onKeyDown = (e) => {
  if (!showSuggestions.value) return

  if (e.key === 'ArrowDown') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value + 1) % suggestions.value.length
    scrollIntoView()
  } else if (e.key === 'ArrowUp') {
    e.preventDefault()
    selectedIndex.value = (selectedIndex.value - 1 + suggestions.value.length) % suggestions.value.length
    scrollIntoView()
  } else if (e.key === 'Escape') {
    showSuggestions.value = false
  } else if (e.key === 'Tab' && suggestions.value.length > 0) {
    e.preventDefault()
    selectSuggestion(suggestions.value[selectedIndex.value])
  } else if (e.key === 'Enter') {
    if (showSuggestions.value && suggestions.value.length > 0) {
      e.preventDefault()
      e.stopPropagation() // Prevent triggering the submit from enter key up
    }
  }
}

const onEnter = (e) => {
  if (showSuggestions.value && suggestions.value.length > 0) {
    selectSuggestion(suggestions.value[selectedIndex.value])
  } else {
    emit('submit', props.modelValue)
  }
}

const scrollIntoView = () => {
  nextTick(() => {
    const el = suggestionsRef.value?.children[selectedIndex.value]
    if (el) el.scrollIntoView({ block: 'nearest' })
  })
}

// Close suggestions on outside click
onMounted(() => {
  document.addEventListener('click', (e) => {
    if (inputRef.value && !inputRef.value.contains(e.target)) {
      showSuggestions.value = false
    }
  })
})
</script>

<style scoped>
.path-input-wrapper {
  flex: 1;
  position: relative;
}

input {
  width: 100%;
  background: var(--background);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: 8px 12px;
  color: var(--text);
  font-family: inherit;
  font-size: 14px;
  outline: none;
}

input:focus {
  border-color: var(--accent);
}

.suggestions-dropdown {
  position: absolute;
  top: 100%;
  left: 0;
  right: 0;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 0 0 8px 8px;
  box-shadow: 0 10px 25px rgba(0, 0, 0, 0.2);
  z-index: 1001;
  max-height: 200px;
  overflow-y: auto;
  margin-top: 1px;
}

.suggestion-item {
  padding: 8px 12px;
  cursor: pointer;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
  transition: background 0.1s;
  color: var(--text);
}

.suggestion-item:hover,
.suggestion-item.selected {
  background: var(--surface-alt);
  color: var(--accent);
}

.icon {
  font-size: 16px;
}
</style>
