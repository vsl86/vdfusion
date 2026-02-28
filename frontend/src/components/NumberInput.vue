<template>
  <div class="number-input-group">
    <button class="num-btn" @click="decrement" :disabled="modelValue <= min">-</button>
    <input 
      type="number" 
      :value="modelValue" 
      @input="onInput"
      class="num-input"
      :min="min"
      :max="max"
    />
    <button class="num-btn" @click="increment" :disabled="modelValue >= max">+</button>
  </div>
</template>

<script setup>
const props = defineProps({
  modelValue: { type: Number, required: true },
  min: { type: Number, default: 0 },
  max: { type: Number, default: Infinity },
  step: { type: Number, default: 1 }
})

const emit = defineEmits(['update:modelValue'])

const onInput = (e) => {
  let val = parseInt(e.target.value)
  if (isNaN(val)) val = props.min
  if (val < props.min) val = props.min
  if (val > props.max) val = props.max
  emit('update:modelValue', val)
}

const increment = () => {
  if (props.modelValue < props.max) {
    emit('update:modelValue', props.modelValue + props.step)
  }
}

const decrement = () => {
  if (props.modelValue > props.min) {
    emit('update:modelValue', props.modelValue - props.step)
  }
}
</script>

<style scoped>
.number-input-group {
  display: flex;
  align-items: center;
  border: 1px solid var(--border);
  border-radius: var(--radius-xs);
  overflow: hidden;
  background: var(--surface);
  width: fit-content;
}

.num-btn {
  background: var(--surface-alt);
  border: none;
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  font-size: 16px;
  font-weight: 600;
  color: var(--text-secondary);
  transition: all 0.2s;
}

.num-btn:hover:not(:disabled) {
  background: var(--border);
  color: var(--text);
}

.num-btn:disabled {
  opacity: 0.3;
  cursor: not-allowed;
}

.num-input {
  width: 60px;
  height: 32px;
  border: none;
  border-left: 1px solid var(--border);
  border-right: 1px solid var(--border);
  text-align: center;
  font-size: 13px;
  font-weight: 500;
  outline: none;
  background: transparent;
  padding: 0;
  appearance: textfield;
  -moz-appearance: textfield;
}

.num-input::-webkit-outer-spin-button,
.num-input::-webkit-inner-spin-button {
  -webkit-appearance: none;
  margin: 0;
}
</style>
