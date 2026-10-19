<script setup lang="ts">
// Masked input for secrets that aren't account passwords (archive passwords).
// Password managers treat every type="password" field as a login form — they
// fill in the saved account password and offer to save what's typed — and
// ignore autocomplete="off" there. So this is a text field masked with CSS,
// which they leave alone. Browsers that can't mask text fields get a
// type="password" field marked as a new password (no autofill).
// Other attributes (class, placeholder, disabled, listeners) fall through.
const model = defineModel<string>({ required: true })

const canMaskText = typeof CSS !== 'undefined' && CSS.supports('-webkit-text-security', 'disc')
</script>

<template>
  <input
    v-model="model"
    :type="canMaskText ? 'text' : 'password'"
    :autocomplete="canMaskText ? 'off' : 'new-password'"
    :class="{ '[-webkit-text-security:disc]': canMaskText }"
    autocapitalize="off"
    spellcheck="false"
    data-1p-ignore
    data-lpignore="true"
    data-bwignore
  />
</template>
