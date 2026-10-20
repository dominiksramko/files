<script setup lang="ts">
import AppToast from '@/components/ui/AppToast.vue'
</script>

<template>
  <RouterView />
  <!-- Here rather than in AppLayout, so messages show on the login page too. -->
  <AppToast />
</template>
