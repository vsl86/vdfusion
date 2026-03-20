<template>
  <div class="slider-fine">
    <div class="value-display">
      <span class="value-text">{{ formattedValue }}<span class="suffix">{{ suffix }}</span></span>
    </div>

    <div class="control-row">
      <button class="round-btn" @click="decrement" :disabled="modelValue <= min" aria-label="Decrease">
        <svg viewBox="0 0 20 20" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5"
          stroke-linecap="round">
          <line x1="5" y1="10" x2="15" y2="10" />
        </svg>
      </button>

      <div class="slider-container">
        <input type="range" :min="min" :max="max" :step="step" :value="modelValue" @input="onInput" class="fine-range"
          :style="sliderStyle" />
      </div>

      <button class="round-btn" @click="increment" :disabled="modelValue >= max" aria-label="Increase">
        <svg viewBox="0 0 20 20" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2.5"
          stroke-linecap="round">
          <line x1="10" y1="5" x2="10" y2="15" />
          <line x1="5" y1="10" x2="15" y2="10" />
        </svg>
      </button>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  modelValue: { type: Number, required: true },
  min: { type: Number, default: 0 },
  max: { type: Number, default: 100 },
  step: { type: Number, default: 1 },
  suffix: { type: String, default: '' },
  decimals: { type: Number, default: 1 },
  showRaw: { type: Boolean, default: true }
})

const emit = defineEmits(['update:modelValue'])

const formattedValue = computed(() => {
  return props.modelValue.toFixed(props.decimals)
})

const sliderStyle = computed(() => {
  const percentage = ((props.modelValue - props.min) / (props.max - props.min)) * 100
  return {
    '--progress': `${percentage}%`
  }
})

const onInput = (e) => {
  emit('update:modelValue', parseFloat(e.target.value))
}

const increment = () => {
  if (props.modelValue < props.max) {
    const val = Math.min(props.max, props.modelValue + props.step)
    emit('update:modelValue', Number(val.toFixed(props.decimals)))
  }
}

const decrement = () => {
  if (props.modelValue > props.min) {
    const val = Math.max(props.min, props.modelValue - props.step)
    emit('update:modelValue', Number(val.toFixed(props.decimals)))
  }
}
</script>

<style scoped>
.slider-fine {
  width: 100%;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
  padding: 8px 0;
  user-select: none;
}

.value-display {
  display: flex;
  flex-direction: column;
  align-items: center;
}

.value-text {
  font-size: 20px;
  font-weight: 800;
  color: var(--accent);
}

.suffix {
  font-size: 0.8em;
  font-weight: 700;
  margin-left: 1px;
}

.control-row {
  display: flex;
  align-items: center;
  gap: 16px;
  width: 100%;
}

.round-btn {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text-secondary);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  transition: all 0.2s cubic-bezier(0.4, 0, 0.2, 1);
  box-shadow: var(--shadow);
  flex-shrink: 0;
}

.round-btn:hover:not(:disabled) {
  border-color: var(--accent);
  color: var(--accent);
  transform: scale(1.05);
  box-shadow: var(--shadow-md);
}

.round-btn:active:not(:disabled) {
  transform: scale(0.95);
  background: var(--surface-alt);
}

.round-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.slider-container {
  flex: 1;
  display: flex;
  align-items: center;
  height: 44px;
}

.fine-range {
  -webkit-appearance: none;
  appearance: none;
  width: 100%;
  height: 6px;
  border-radius: 3px;
  background: linear-gradient(to right, var(--accent) 0%, var(--accent) var(--progress), var(--border) var(--progress), var(--border) 100%);
  outline: none;
  cursor: pointer;
}

.fine-range::-webkit-slider-thumb {
  -webkit-appearance: none;
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: white;
  border: 1px solid var(--border);
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.15);
  cursor: grab;
  transition: transform 0.1s;
}

.fine-range::-webkit-slider-thumb:active {
  cursor: grabbing;
  transform: scale(1.1);
}

.fine-range::-moz-range-thumb {
  width: 24px;
  height: 24px;
  border-radius: 50%;
  background: white;
  border: 1px solid var(--border);
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.15);
  cursor: grab;
}
</style>
