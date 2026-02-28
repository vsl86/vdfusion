<template>
  <div v-if="path" class="video-preview-overlay" @click.self="close">
    <div class="video-container">
      <div class="video-header">
        <div class="file-name">{{ fileName }}</div>
        <div class="header-actions">
          <button class="action-btn" title="Open in default player (ffplay/VLC)" @click="openExternal">
            📺 Open External
          </button>
          <button class="close-btn" @click="close">×</button>
        </div>
      </div>
      <div v-if="error" class="video-error">
        <div class="error-msg">
          <span class="error-icon">⚠️</span>
          <p>This video format ({{ fileName.split('.').pop().toUpperCase() }}) might not be supported natively by your browser.</p>
          <button class="retry-btn" @click="openExternal">Open in External Player</button>
        </div>
      </div>
      <video 
        v-else
        ref="videoPlayer" 
        :src="streamUrl" 
        controls 
        autoplay 
        class="video-element"
        @error="handleVideoError"
      ></video>
      <div class="video-footer">
        <div class="controls-hint">
          Space: Play/Pause | ← →: Seek 15s | ↑ ↓: Volume | Esc: Close
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { GetStreamUrl } from '../api'

const props = defineProps({
  path: String
})

const emit = defineEmits(['close'])

const videoPlayer = ref(null)
const error = ref(false)

const fileName = computed(() => {
  if (!props.path) return ''
  return props.path.split(/[\\/]/).pop()
})

const streamUrl = computed(() => {
  if (!props.path) return ''
  return GetStreamUrl(props.path)
})

const handleVideoError = (e) => {
  console.error("Video playback error:", e)
  error.value = true
}

const openExternal = async () => {
  try {
    // Assuming backend has an OpenFile method exposed
    if (window.go?.main?.App?.OpenFile) {
        await window.go.main.App.OpenFile(props.path)
    } else {
        // Fallback for Web UI
        await fetch('/api/files/open', {
            method: 'POST',
            body: JSON.stringify({ path: props.path }),
            headers: { 'Content-Type': 'application/json' }
        })
    }
    // Optionally close the preview if it was opened externally
    // close()
  } catch (err) {
    console.error("Failed to open external player:", err)
  }
}

const close = () => {
  error.value = false
  emit('close')
}

const handleGlobalKeydown = (e) => {
  if (!props.path) return

  // Close on Escape even if videoPlayer isn't ready
  if (e.code === 'Escape') {
    e.preventDefault()
    close()
    return
  }

  if (!videoPlayer.value) return

  switch (e.code) {
    case 'Space':
      e.preventDefault()
      if (videoPlayer.value.paused) videoPlayer.value.play()
      else videoPlayer.value.pause()
      break
    case 'ArrowLeft':
      e.preventDefault()
      videoPlayer.value.currentTime = Math.max(0, videoPlayer.value.currentTime - 15)
      break
    case 'ArrowRight':
      e.preventDefault()
      videoPlayer.value.currentTime = Math.min(videoPlayer.value.duration, videoPlayer.value.currentTime + 15)
      break
    case 'ArrowUp':
      e.preventDefault()
      videoPlayer.value.volume = Math.min(1, videoPlayer.value.volume + 0.1)
      break
    case 'ArrowDown':
      e.preventDefault()
      videoPlayer.value.volume = Math.max(0, videoPlayer.value.volume - 0.1)
      break
  }
}

onMounted(() => {
    window.addEventListener('keydown', handleGlobalKeydown)
})

onUnmounted(() => {
    window.removeEventListener('keydown', handleGlobalKeydown)
})

watch(() => props.path, (newPath) => {
    if (newPath) {
        nextTick(() => {
            videoPlayer.value?.focus()
        })
    }
})
</script>

<style scoped>
.video-preview-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100vw;
  height: 100vh;
  background: rgba(0, 0, 0, 0.85);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
  backdrop-filter: blur(4px);
}

.video-container {
  width: 90%;
  max-width: 1200px;
  background: #1a1b1e;
  border-radius: 12px;
  overflow: hidden;
  box-shadow: 0 20px 50px rgba(0, 0, 0, 0.5);
  border: 1px solid #333;
}

.video-header {
  padding: 12px 20px;
  background: #25262b;
  display: flex;
  justify-content: space-between;
  align-items: center;
  border-bottom: 1px solid #333;
}

.header-actions {
  display: flex;
  gap: 12px;
  align-items: center;
}

.action-btn {
  background: #3b82f6;
  color: white;
  border: none;
  padding: 4px 10px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.2s;
}

.action-btn:hover {
  background: #2563eb;
}

.file-name {
  color: #eee;
  font-size: 14px;
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  margin-right: 20px;
}

.close-btn {
  background: none;
  border: none;
  color: #999;
  font-size: 28px;
  cursor: pointer;
  line-height: 1;
  transition: color 0.2s;
}

.close-btn:hover {
  color: #fff;
}

.video-error {
  padding: 40px;
  text-align: center;
  background: #000;
  min-height: 200px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.error-msg {
  max-width: 400px;
}

.error-icon {
  font-size: 48px;
  display: block;
  margin-bottom: 16px;
}

.error-msg p {
  color: #ccc;
  margin-bottom: 20px;
  line-height: 1.5;
}

.retry-btn {
  background: #6366f1;
  color: white;
  border: none;
  padding: 8px 16px;
  border-radius: 6px;
  font-size: 14px;
  font-weight: 600;
  cursor: pointer;
}

.retry-btn:hover {
  background: #4f46e5;
}

.video-element {
  width: 100%;
  max-height: 70vh;
  display: block;
  outline: none;
}

.video-footer {
  padding: 10px 20px;
  background: #25262b;
  border-top: 1px solid #333;
}

.controls-hint {
  font-size: 12px;
  color: #666;
  text-align: center;
}
</style>
