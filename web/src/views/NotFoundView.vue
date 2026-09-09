<script setup lang="ts">
import { chunkLoadRecovery } from '../utils/chunkLoadRecovery';

function returnToDashboard() {
  return chunkLoadRecovery.push({ name: 'dashboard' });
}
</script>

<template>
  <section
    class="grid min-h-64 place-items-center rounded-xl border border-border bg-surface p-8 text-center"
  >
    <div>
      <h2 class="font-display text-2xl font-bold text-text-strong">页面不存在</h2>
      <p class="mt-2 text-sm text-muted">当前地址没有对应的管理台页面。</p>
      <button
        type="button"
        class="mt-5 inline-flex h-9 items-center rounded-md bg-brand-600 px-4 text-sm font-medium text-white hover:bg-brand-700"
        @click="returnToDashboard"
      >
        返回总览
      </button>
    </div>
  </section>
</template>
