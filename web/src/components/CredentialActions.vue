<script setup lang="ts">
import { computed, ref, type Component } from 'vue';
import {
  Building2,
  CalendarCheck,
  CircleCheckBig,
  MousePointerClick,
  Pencil,
  RefreshCw,
  RotateCcw,
  Trash2,
} from '@lucide/vue';
import type { CredentialRecord } from '../types';
import CActionMenu from './ui/CActionMenu.vue';
import CButton from './ui/CButton.vue';
import CModal from './ui/CModal.vue';
import CTooltip from './ui/CTooltip.vue';

interface Props {
  credential: CredentialRecord;
  isCurrent: boolean;
  autoRotationEnabled?: boolean;
  isTesting: boolean;
  isSelecting?: boolean;
  isDeleting?: boolean;
  writeInProgress?: boolean;
  hasActiveTests?: boolean;
  canSwitchAccount?: boolean;
  canCheckIn?: boolean;
  isCheckingIn?: boolean;
  checkinDisabledReason?: string;
  isRefreshingQuota?: boolean;
  canEditQuotaProbeMode?: boolean;
  quotaProbeModeDisabledReason?: string;
}

interface MenuItem {
  key: string;
  label: string;
  icon: Component;
  disabled?: boolean;
  title?: string;
  danger?: boolean;
  separatorBefore?: boolean;
}

const props = withDefaults(defineProps<Props>(), {
  autoRotationEnabled: false,
  isSelecting: false,
  isDeleting: false,
  writeInProgress: false,
  hasActiveTests: false,
  canSwitchAccount: false,
  canCheckIn: false,
  isCheckingIn: false,
  checkinDisabledReason: '',
  isRefreshingQuota: false,
  canEditQuotaProbeMode: false,
  quotaProbeModeDisabledReason: '',
});

const emit = defineEmits<{
  select: [credentialId: string];
  test: [credentialId: string];
  delete: [credentialId: string];
  switchAccount: [credentialId: string];
  checkin: [credentialId: string];
  refreshQuota: [credentialId: string];
  editQuotaProbeMode: [credentialId: string];
}>();

const deleteModalOpen = ref(false);
const deleteTitle = computed(
  () =>
    `确定删除凭证 ${props.credential.email || props.credential.user_id || props.credential.credential_id}？该操作不可恢复`,
);
const isFixedCurrent = computed(() => props.isCurrent && !props.autoRotationEnabled);
const rowBusy = computed(() => props.isCheckingIn || props.isRefreshingQuota || props.isDeleting);
const actionsBlocked = computed(
  () => props.writeInProgress || props.hasActiveTests || rowBusy.value,
);
const selectDisabled = computed(() => isFixedCurrent.value || actionsBlocked.value);
const testDisabled = computed(() => props.isTesting || props.writeInProgress || rowBusy.value);
const checkinDisabled = computed(
  () =>
    actionsBlocked.value ||
    Boolean(props.checkinDisabledReason) ||
    props.credential.daily_checkin?.success === true,
);
const checkinTitle = computed(() => {
  if (props.checkinDisabledReason) return props.checkinDisabledReason;
  const detail = props.credential.daily_checkin;
  if (!detail) return '签到';
  const lines: string[] = [];
  if (detail.code === 0) {
    const checkedInAt =
      typeof detail.checked_in_at === 'number'
        ? new Date(detail.checked_in_at * 1000).toLocaleString()
        : '-';
    lines.push(`签到时间：${checkedInAt}`);
    lines.push(`获得积分：${detail.credit ?? '-'}`);
  } else {
    lines.push(`Code：${detail.code ?? '未知'}`);
    lines.push(`消息：${detail.message}`);
  }
  if (detail.success && typeof detail.next_checkin_at === 'number') {
    lines.push(`下次可签到：${new Date(detail.next_checkin_at * 1000).toLocaleString()}`);
  }
  return lines.join('\n');
});
const selectTooltip = computed(() => {
  if (!props.isCurrent) return '设为当前凭证';
  return props.autoRotationEnabled ? '固定当前凭证' : '已是当前凭证';
});
const selectAriaLabel = computed(() => {
  if (!props.isCurrent) return '切换为当前凭证';
  return props.autoRotationEnabled ? '固定当前凭证' : '已是当前凭证';
});
const menuItems = computed<MenuItem[]>(() => {
  const items: MenuItem[] = [];
  if (props.canCheckIn || props.checkinDisabledReason) {
    items.push({
      key: 'checkin',
      label: '签到',
      icon: CalendarCheck,
      disabled: checkinDisabled.value,
      title: checkinTitle.value,
    });
  }
  if (props.canSwitchAccount) {
    items.push({
      key: 'switchAccount',
      label: '切换 CodeBuddy 账号',
      icon: Building2,
      disabled: actionsBlocked.value,
    });
  }
  items.push({
    key: 'refreshQuota',
    label: '刷新额度',
    icon: RefreshCw,
    disabled: actionsBlocked.value || props.credential.is_expired,
    title: props.credential.is_expired ? '凭证已过期，无法刷新额度' : undefined,
  });
  if (props.canEditQuotaProbeMode) {
    items.push({
      key: 'editQuotaProbeMode',
      label: '额度探测方式',
      icon: Pencil,
      disabled: actionsBlocked.value || Boolean(props.quotaProbeModeDisabledReason),
      title: props.quotaProbeModeDisabledReason || undefined,
    });
  }
  items.push({
    key: 'delete',
    label: '删除凭证',
    icon: Trash2,
    disabled: actionsBlocked.value,
    danger: true,
    separatorBefore: true,
  });
  return items;
});

function selectCredential(): void {
  if (!selectDisabled.value) emit('select', props.credential.credential_id);
}

function testCredential(): void {
  if (!testDisabled.value) emit('test', props.credential.credential_id);
}

function handleMenuAction(key: string): void {
  if (actionsBlocked.value) return;
  if (key === 'checkin' && props.canCheckIn && !checkinDisabled.value) {
    emit('checkin', props.credential.credential_id);
  } else if (key === 'switchAccount' && props.canSwitchAccount) {
    emit('switchAccount', props.credential.credential_id);
  } else if (key === 'refreshQuota' && !props.credential.is_expired) {
    emit('refreshQuota', props.credential.credential_id);
  } else if (
    key === 'editQuotaProbeMode' &&
    props.canEditQuotaProbeMode &&
    !props.quotaProbeModeDisabledReason
  ) {
    emit('editQuotaProbeMode', props.credential.credential_id);
  } else if (key === 'delete') {
    deleteModalOpen.value = true;
  }
}

function closeDeleteModal(): void {
  if (!props.isDeleting) deleteModalOpen.value = false;
}

function confirmDelete(): void {
  if (actionsBlocked.value) return;
  deleteModalOpen.value = false;
  emit('delete', props.credential.credential_id);
}
</script>

<template>
  <div class="table-action-group flex items-center justify-start gap-1.5">
    <CTooltip :content="selectTooltip">
      <CButton
        size="sm"
        variant="secondary"
        shape="circle"
        :disabled="selectDisabled"
        :loading="isSelecting"
        :class="['table-action-button', { 'current-credential-action-button': isFixedCurrent }]"
        :aria-label="selectAriaLabel"
        @click="selectCredential"
      >
        <template #icon>
          <CircleCheckBig v-if="isFixedCurrent" :size="14" />
          <MousePointerClick v-else :size="14" />
        </template>
      </CButton>
    </CTooltip>

    <CTooltip content="测试凭证">
      <CButton
        size="sm"
        variant="secondary"
        shape="circle"
        class="table-action-button"
        :loading="isTesting"
        :disabled="testDisabled"
        aria-label="测试凭证"
        @click="testCredential"
      >
        <template #icon><RotateCcw :size="14" /></template>
      </CButton>
    </CTooltip>

    <CActionMenu
      :items="menuItems"
      :disabled="actionsBlocked"
      :loading="isCheckingIn || isRefreshingQuota || isDeleting"
      @select="handleMenuAction"
    />
  </div>

  <CModal
    :open="deleteModalOpen"
    title="删除凭证"
    :closable="!isDeleting"
    @update:open="closeDeleteModal"
  >
    <p class="text-sm text-text">{{ deleteTitle }}</p>
    <template #footer>
      <CButton :disabled="isDeleting" @click="closeDeleteModal">取消</CButton>
      <CButton variant="danger" :loading="isDeleting" @click="confirmDelete">删除</CButton>
    </template>
  </CModal>
</template>
