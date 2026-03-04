<script setup lang="ts">
const props = defineProps<{
  modelValue: string | number
  options: { label: string; value: string | number }[]
  disabled?: boolean
}>()

const emit = defineEmits<{
  'update:modelValue': [value: string | number]
}>()

function handleChange(e: Event) {
  const raw = (e.target as HTMLSelectElement).value
  // Return the original typed value from the options array so numbers stay numbers
  const match = props.options.find(o => String(o.value) === raw)
  emit('update:modelValue', match ? match.value : raw)
}
</script>

<template>
  <select
    :value="modelValue"
    :disabled="disabled"
    class="w-full bg-zinc-800 border border-zinc-700 rounded-lg px-3 py-1.5 text-sm text-white focus:outline-none focus:border-blue-500 disabled:opacity-50 disabled:cursor-not-allowed"
    @change="handleChange"
  >
    <option v-for="o in options" :key="String(o.value)" :value="o.value">{{ o.label }}</option>
  </select>
</template>
