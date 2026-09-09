<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, useId, watch, type Component } from 'vue';
import { Ellipsis } from '@lucide/vue';
import CSpin from './CSpin.vue';
import CTooltip from './CTooltip.vue';

interface ActionMenuItem {
  key: string;
  label: string;
  icon?: Component;
  disabled?: boolean;
  title?: string;
  danger?: boolean;
  separatorBefore?: boolean;
  loading?: boolean;
}

const props = withDefaults(
  defineProps<{
    items: ActionMenuItem[];
    disabled?: boolean;
    loading?: boolean;
    ariaLabel?: string;
  }>(),
  {
    disabled: false,
    loading: false,
    ariaLabel: '更多操作',
  },
);

const emit = defineEmits<{ select: [key: string] }>();
const visible = ref(false);
const positioned = ref(false);
const triggerRef = ref<HTMLButtonElement | null>(null);
const panelRef = ref<HTMLElement | null>(null);
const menuId = `c-action-menu-${useId().replace(/[^A-Za-z0-9_-]/g, '')}`;
const positionStyle = ref<Record<string, string>>({ left: '0px', top: '0px' });
const VIEWPORT_PADDING = 8;
const MENU_GAP = 8;

function enabledItems(): HTMLButtonElement[] {
  return Array.from(
    panelRef.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not([disabled])') ?? [],
  );
}

function focusInitial(preferLast: boolean): void {
  const items = enabledItems();
  const target = preferLast ? items.at(-1) : items[0];
  (target ?? panelRef.value)?.focus();
}

function addListeners(): void {
  document.addEventListener('click', onDocumentClick, true);
  document.addEventListener('keydown', onDocumentKeydown);
  window.addEventListener('scroll', updatePosition, true);
  window.addEventListener('resize', updatePosition);
}

function removeListeners(): void {
  document.removeEventListener('click', onDocumentClick, true);
  document.removeEventListener('keydown', onDocumentKeydown);
  window.removeEventListener('scroll', updatePosition, true);
  window.removeEventListener('resize', updatePosition);
}

async function open(preferLast = false, focusMenu = true): Promise<void> {
  if (props.disabled || props.loading) return;
  positioned.value = false;
  visible.value = true;
  await nextTick();
  if (!visible.value) return;
  updatePosition();
  addListeners();
  if (focusMenu) focusInitial(preferLast);
}

function close(restoreFocus = true): void {
  if (!visible.value) return;
  visible.value = false;
  positioned.value = false;
  removeListeners();
  if (restoreFocus) nextTick(() => triggerRef.value?.focus());
}

function toggle(): void {
  if (visible.value) close();
  else void open(false, false);
}

function onTriggerKeydown(event: KeyboardEvent): void {
  if (!['ArrowDown', 'ArrowUp', 'Enter', ' '].includes(event.key)) return;
  event.preventDefault();
  void open(event.key === 'ArrowUp');
}

function onPanelKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.preventDefault();
    close();
    return;
  }
  if (event.key === 'Tab') {
    close(false);
    return;
  }
  const items = enabledItems();
  if (!['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key) || items.length === 0) return;
  event.preventDefault();
  const currentIndex = items.indexOf(document.activeElement as HTMLButtonElement);
  let targetIndex = 0;
  if (event.key === 'End') targetIndex = items.length - 1;
  else if (event.key === 'ArrowDown') targetIndex = (currentIndex + 1) % items.length;
  else if (event.key === 'ArrowUp') {
    targetIndex = (currentIndex - 1 + items.length) % items.length;
  }
  items[targetIndex].focus();
}

function select(item: ActionMenuItem): void {
  if (item.disabled || item.loading) return;
  emit('select', item.key);
  close();
}

function onDocumentClick(event: MouseEvent): void {
  const target = event.target;
  if (!(target instanceof Node)) return;
  if (triggerRef.value?.contains(target) || panelRef.value?.contains(target)) return;
  close(false);
}

function onDocumentKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return;
  event.preventDefault();
  close();
}

function updatePosition(): void {
  if (!visible.value || !triggerRef.value || !panelRef.value) return;
  const triggerRect = triggerRef.value.getBoundingClientRect();
  const panelRect = panelRef.value.getBoundingClientRect();
  const viewportWidth = document.documentElement.clientWidth || window.innerWidth;
  const viewportHeight = document.documentElement.clientHeight || window.innerHeight;
  const spaceBelow = viewportHeight - triggerRect.bottom - MENU_GAP;
  const spaceAbove = triggerRect.top - MENU_GAP;
  const placeAbove = spaceBelow < panelRect.height && spaceAbove > spaceBelow;
  const preferredTop = placeAbove
    ? triggerRect.top - MENU_GAP - panelRect.height
    : triggerRect.bottom + MENU_GAP;
  const preferredLeft = triggerRect.right - panelRect.width;
  const maxLeft = Math.max(VIEWPORT_PADDING, viewportWidth - panelRect.width - VIEWPORT_PADDING);
  const maxTop = Math.max(VIEWPORT_PADDING, viewportHeight - panelRect.height - VIEWPORT_PADDING);
  positionStyle.value = {
    left: `${Math.min(Math.max(preferredLeft, VIEWPORT_PADDING), maxLeft)}px`,
    top: `${Math.min(Math.max(preferredTop, VIEWPORT_PADDING), maxTop)}px`,
  };
  positioned.value = true;
}

watch(
  () => props.disabled || props.loading,
  (blocked) => {
    if (blocked) close(false);
  },
);

onBeforeUnmount(removeListeners);
</script>

<template>
  <span class="relative inline-flex">
    <button
      ref="triggerRef"
      type="button"
      :disabled="disabled || loading"
      class="c-action-menu-trigger table-action-button inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-md border border-border bg-surface text-text shadow-[var(--shadow-xs)] transition-[background-color,box-shadow,transform] duration-(--duration-fast) hover:border-border-strong hover:bg-surface-2 active:scale-[0.98] disabled:opacity-50"
      :aria-label="ariaLabel"
      aria-haspopup="menu"
      :aria-expanded="visible"
      :aria-controls="visible ? menuId : undefined"
      @click="toggle"
      @keydown="onTriggerKeydown"
    >
      <CSpin v-if="loading" size="sm" />
      <Ellipsis v-else :size="16" />
    </button>

    <Teleport to="body">
      <Transition
        enter-active-class="transition-[opacity,translate] duration-[var(--duration-fast)] ease-[var(--ease-out-quad)]"
        leave-active-class="transition-[opacity,translate] duration-[var(--duration-fast)] ease-[var(--ease-in-quad)]"
        enter-from-class="opacity-0 -translate-y-1"
        leave-to-class="opacity-0 -translate-y-1"
      >
        <div
          v-if="visible"
          :id="menuId"
          ref="panelRef"
          :style="positionStyle"
          :class="[
            'c-action-menu-panel fixed z-50 w-52 rounded-lg border border-border bg-surface p-1 shadow-[var(--shadow-popover)]',
            positioned ? '' : 'pointer-events-none opacity-0',
          ]"
          role="menu"
          :aria-label="ariaLabel"
          tabindex="-1"
          @keydown="onPanelKeydown"
        >
          <component
            v-for="item in items"
            :key="item.key"
            :is="item.title ? CTooltip : 'span'"
            v-bind="item.title ? { content: item.title, placement: 'top' } : {}"
            :class="item.title ? 'w-full' : 'contents'"
          >
            <button
              type="button"
              role="menuitem"
              :disabled="item.disabled || item.loading"
              :class="[
                'flex min-h-9 w-full items-center gap-2 rounded-md px-3 py-2 text-left text-sm transition-[background-color] hover:bg-surface-2 disabled:cursor-not-allowed disabled:opacity-50',
                item.danger ? 'text-error-600' : 'text-text',
                item.separatorBefore ? 'mt-1 border-t border-border pt-2' : '',
              ]"
              @click="select(item)"
            >
              <CSpin v-if="item.loading" class="shrink-0" size="sm" />
              <component :is="item.icon" v-else-if="item.icon" :size="15" class="shrink-0" />
              <span>{{ item.label }}</span>
            </button>
            <template v-if="item.title" #content>
              <span class="flex flex-col gap-1">
                <span v-for="(line, index) in item.title.split('\n')" :key="index">{{ line }}</span>
              </span>
            </template>
          </component>
        </div>
      </Transition>
    </Teleport>
  </span>
</template>
