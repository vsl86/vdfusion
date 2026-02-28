<template>
  <Teleport to="body">
    <Transition name="modal">
      <div v-if="show" class="modal-overlay" @click.self="handleCancel">
        <div class="modal-container" :class="{ 'modal-prompt': type === 'prompt' }">
          <div class="modal-header">
            <h3>{{ title }}</h3>
            <button class="close-btn" @click="handleCancel">✕</button>
          </div>
          <div class="modal-body">
            <p v-if="message">{{ message }}</p>
            <div v-if="type === 'prompt'" class="prompt-input-container">
              <input 
                ref="inputRef"
                v-model="inputValue" 
                type="text" 
                class="prompt-input" 
                @keyup.enter="handleConfirm"
                :placeholder="placeholder"
              />
            </div>
          </div>
          <div class="modal-footer">
            <button v-if="type !== 'alert'" class="modal-btn secondary" @click="handleCancel">
              {{ cancelLabel }}
            </button>
            <button class="modal-btn primary" :class="{ danger: isDanger }" @click="handleConfirm">
              {{ confirmLabel }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<script setup>
import { ref, watch, nextTick } from 'vue'

const props = defineProps({
  show: Boolean,
  title: { type: String, default: 'Confirm' },
  message: String,
  type: { type: String, default: 'confirm' }, // alert, confirm, prompt
  confirmLabel: { type: String, default: 'Confirm' },
  cancelLabel: { type: String, default: 'Cancel' },
  defaultValue: { type: String, default: '' },
  placeholder: { type: String, default: '' },
  isDanger: { type: Boolean, default: false }
})

const emit = defineEmits(['confirm', 'cancel', 'update:show'])

const inputValue = ref('')
const inputRef = ref(null)

watch(() => props.show, (newVal) => {
  if (newVal) {
    inputValue.value = props.defaultValue
    if (props.type === 'prompt') {
      nextTick(() => {
        inputRef.value?.focus()
        inputRef.value?.select()
      })
    }
  }
})

const handleConfirm = () => {
  if (props.type === 'prompt') {
    emit('confirm', inputValue.value)
  } else {
    emit('confirm')
  }
  emit('update:show', false)
}

const handleCancel = () => {
  emit('cancel')
  emit('update:show', false)
}
</script>

<style scoped>
.modal-overlay {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.4);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 9999;
}

.modal-container {
  background: var(--surface);
  border-radius: var(--radius);
  box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.1), 0 10px 10px -5px rgba(0, 0, 0, 0.04);
  width: 90%;
  max-width: 450px;
  overflow: hidden;
  border: 1px solid var(--border);
}

.modal-prompt {
  max-width: 500px;
}

.modal-header {
  padding: 16px 20px;
  border-bottom: 1px solid var(--border);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.modal-header h3 {
  margin: 0;
  font-size: 16px;
  font-weight: 600;
}

.close-btn {
  background: none;
  border: none;
  font-size: 18px;
  color: var(--text-muted);
  cursor: pointer;
}

.modal-body {
  padding: 20px;
}

.modal-body p {
  margin: 0;
  font-size: 14px;
  line-height: 1.5;
  color: var(--text-secondary);
}

.prompt-input-container {
  margin-top: 16px;
}

.prompt-input {
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--border);
  border-radius: var(--radius-sm);
  font-size: 14px;
  font-family: inherit;
  outline: none;
}

.prompt-input:focus {
  border-color: var(--accent);
  box-shadow: 0 0 0 2px rgba(59, 130, 246, 0.1);
}

.modal-footer {
  padding: 16px 20px;
  background: var(--surface-alt);
  border-top: 1px solid var(--border);
  display: flex;
  justify-content: flex-end;
  gap: 12px;
}

.modal-btn {
  padding: 8px 16px;
  border-radius: var(--radius-sm);
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid transparent;
}

.modal-btn.primary {
  background: var(--primary);
  color: white;
}

.modal-btn.primary:hover {
  background: var(--primary-hover);
}

.modal-btn.primary.danger {
  background: var(--danger);
}

.modal-btn.primary.danger:hover {
  filter: brightness(0.9);
}

.modal-btn.secondary {
  background: white;
  border-color: var(--border);
  color: var(--text);
}

.modal-btn.secondary:hover {
  background: var(--surface-alt);
  border-color: var(--text-muted);
}

/* Transitions */
.modal-enter-active, .modal-leave-active {
  transition: opacity 0.3s ease;
}

.modal-enter-from, .modal-leave-to {
  opacity: 0;
}

.modal-enter-active .modal-container, .modal-leave-active .modal-container {
  transition: transform 0.3s cubic-bezier(0.34, 1.56, 0.64, 1);
}

.modal-enter-from .modal-container {
  transform: scale(0.9) translateY(20px);
}

.modal-leave-to .modal-container {
  transform: scale(0.95);
}
</style>
