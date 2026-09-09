<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue';
import { chunkLoadRecovery } from '../utils/chunkLoadRecovery';
import CButton from './ui/CButton.vue';
import { registerOverlay } from './ui/overlayStack';

const failure = computed(() => chunkLoadRecovery.failure.value);
const screen = ref<HTMLElement | null>(null);
const panel = ref<HTMLElement | null>(null);
let unregisterOverlay: (() => void) | null = null;

function deactivateOverlay(): void {
  unregisterOverlay?.();
  unregisterOverlay = null;
}

function activateOverlay(): void {
  unregisterOverlay = registerOverlay({
    elements: [screen.value!],
    focusRoot: panel.value!,
    modal: true,
  });
}

watch(
  () => failure.value !== null,
  (visible) => {
    if (visible) activateOverlay();
    else deactivateOverlay();
  },
  { flush: 'post' },
);

onMounted(() => {
  if (failure.value !== null) activateOverlay();
});

onUnmounted(deactivateOverlay);
</script>

<template>
  <Teleport to="body">
    <div
      v-if="failure"
      ref="screen"
      class="chunk-load-error-screen fixed inset-0 z-[60] grid min-h-screen place-items-center bg-bg px-5"
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="chunk-load-error-title"
      aria-describedby="chunk-load-error-description"
    >
      <div
        ref="panel"
        class="w-full max-w-md rounded-xl border border-border bg-surface p-6 text-center shadow-(--shadow-card)"
      >
        <h1 id="chunk-load-error-title" class="font-display text-lg font-semibold text-text-strong">
          页面资源加载失败
        </h1>
        <p id="chunk-load-error-description" class="mt-2 text-sm text-muted">
          页面可能已经更新，或网络暂时不可用。请检查网络后重新加载。
        </p>
        <div class="mt-5 flex justify-center gap-3">
          <CButton
            v-if="failure.canStay"
            variant="secondary"
            @click="chunkLoadRecovery.stayOnCurrentPage"
          >
            留在当前页
          </CButton>
          <CButton variant="primary" @click="chunkLoadRecovery.retryReload">重新加载</CButton>
        </div>
      </div>
    </div>
  </Teleport>
</template>
