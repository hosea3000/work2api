<script setup lang="ts">
import { computed, h, reactive, ref } from 'vue';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { Pause, Play } from '@lucide/vue';
import CAlert from './ui/CAlert.vue';
import CButton from './ui/CButton.vue';
import CCard from './ui/CCard.vue';
import CDataTable, { type Column } from './ui/CDataTable.vue';
import CRadioGroup from './ui/CRadioGroup.vue';
import CRadioButton from './ui/CRadioButton.vue';
import CTag from './ui/CTag.vue';
import { adminApi } from '../api/admin';
import type { CredentialProvider, CredentialRecord, CredentialsResponse } from '../types';
import { useToast } from '../composables/useToast';
import CredentialActions from './CredentialActions.vue';
import RefreshButton from './RefreshButton.vue';
import { filterCredentials, type CredentialFilterTab } from '../utils/credentialsFilter';
import { useSessionStore } from '../stores/session';
import { adminQueryKeys } from '../utils/adminQueryKeys';

const props = defineProps<{ provider: CredentialProvider; title: string }>();

const queryClient = useQueryClient();
const session = useSessionStore();
const queryKeys = adminQueryKeys(session.username);
const toast = useToast();

const testingIds = reactive(new Set<string>());
const checkingInIds = reactive(new Set<string>());
const selectingId = ref<string | null>(null);
const deletingId = ref<string | null>(null);

const filterTab = ref<CredentialFilterTab>('all');
const credentialFilterOrder: Record<CredentialFilterTab, number> = { all: 0, valid: 1, expired: 2 };
const credentialFilterTransition = ref('credential-slide-left');

const credentialsQuery = useQuery({
  queryKey: queryKeys.credentials(props.provider),
  queryFn: () => adminApi.credentials(props.provider),
});

const allCredentials = computed<CredentialRecord[]>(
  () => credentialsQuery.data.value?.credentials || [],
);
const currentId = computed(() => credentialsQuery.data.value?.current.credential_id || '');
const autoRotationEnabled = computed(
  () => credentialsQuery.data.value?.auto_rotation_enabled === true,
);
const autoRotationKnown = computed(
  () => typeof credentialsQuery.data.value?.auto_rotation_enabled === 'boolean',
);
const rotationToggleLabel = computed(() =>
  autoRotationEnabled.value ? '关闭自动轮换' : '开启自动轮换',
);
const loadError = computed(() => credentialsQuery.error.value);

const rows = computed(() =>
  filterCredentials(
    [...allCredentials.value].sort((a, b) => Number(a.is_expired) - Number(b.is_expired)),
    filterTab.value,
  ),
);

const credentialCounts = computed(() => {
  const list = allCredentials.value;
  return {
    all: list.length,
    valid: list.filter((item) => !item.is_expired).length,
    expired: list.filter((item) => item.is_expired).length,
  };
});

function invalidate() {
  return Promise.all([
    queryClient.invalidateQueries({ queryKey: queryKeys.credentials(props.provider) }),
    queryClient.invalidateQueries({ queryKey: queryKeys.status }),
  ]);
}

const selectMutation = useMutation({
  mutationFn: (credentialId: string) => adminApi.selectCredential(props.provider, credentialId),
  onMutate: (credentialId: string) => {
    selectingId.value = credentialId;
  },
  onSuccess: async (data) => {
    toast.success(data.auto_rotation_disabled_by_select ? '已切换凭证，自动轮换已关闭' : '已切换凭证');
    await invalidate();
  },
  onSettled: (_d, _e, credentialId) => {
    if (selectingId.value === credentialId) selectingId.value = null;
  },
});

const deleteMutation = useMutation({
  mutationFn: (credentialId: string) => adminApi.deleteCredential(props.provider, credentialId),
  onMutate: (credentialId: string) => {
    deletingId.value = credentialId;
  },
  onSuccess: async (_data, credentialId) => {
    toast.success('凭证已删除');
    queryClient.setQueryData<CredentialsResponse>(queryKeys.credentials(props.provider), (old) => {
      if (!old) return old;
      return { ...old, credentials: old.credentials.filter((c) => c.credential_id !== credentialId) };
    });
    await invalidate();
  },
  onSettled: (_d, _e, credentialId) => {
    if (deletingId.value === credentialId) deletingId.value = null;
  },
});

const testMutation = useMutation({
  mutationFn: (credentialId: string) => adminApi.testCredential(props.provider, credentialId),
  onMutate: (credentialId: string) => {
    testingIds.add(credentialId);
  },
  onSuccess: (result) => {
    if (result.ok) toast.success('凭证可用');
    else toast.error(`测试失败：${result.detail || `HTTP ${result.status_code}`}`);
  },
  onSettled: (_d, _e, credentialId) => {
    testingIds.delete(credentialId);
  },
});

const checkinMutation = useMutation({
  mutationFn: (credentialId: string) => adminApi.dailyCheckin(props.provider, credentialId),
  onMutate: (credentialId: string) => {
    checkingInIds.add(credentialId);
  },
  onSuccess: (result) => {
    const message =
      result.success && (result.code === 0 || result.code == null)
        ? `签到成功${result.credit != null ? `，权益额度：${result.credit}` : ''}`
        : `${result.code ?? '未知'}：${result.message}`;
    if (result.success) toast.success(message);
    else toast.error(message);
  },
  onSettled: async (_d, _e, credentialId) => {
    checkingInIds.delete(credentialId);
    await invalidate();
  },
});

const toggleRotationMutation = useMutation({
  mutationFn: () => adminApi.toggleRotation(props.provider),
  onSuccess: async (data) => {
    toast.success(data.auto_rotation_enabled ? '自动轮换已启用' : '自动轮换已暂停');
    await invalidate();
  },
});

const hasActiveTests = computed(() => testingIds.size > 0);
const writeInProgress = computed(
  () => selectMutation.isPending.value || deleteMutation.isPending.value,
);

const columns: Column<CredentialRecord>[] = [
  {
    title: '状态',
    key: 'status',
    width: 110,
    render: (row) => {
      const active = row.credential_id === currentId.value;
      const expired = row.is_expired;
      return h(
        CTag,
        { type: active && expired ? 'error' : active ? 'success' : expired ? 'error' : 'default' },
        { default: () => (active && expired ? '当前 · 已过期' : active ? '当前' : expired ? '过期' : '可用') },
      );
    },
  },
  {
    title: '用户',
    key: 'email',
    minWidth: 200,
    render: (row) =>
      row.nickname || row.preferred_username || row.email || row.user_id || '-',
  },
  { title: 'Token', key: 'token_display', minWidth: 160, className: 'mono' },
  { title: '剩余', key: 'time_remaining_str', width: 110 },
  {
    title: '操作',
    key: 'actions',
    width: 168,
    render: (row) =>
      h(CredentialActions, {
        credential: row,
        isCurrent: row.credential_id === currentId.value,
        autoRotationEnabled: autoRotationEnabled.value,
        isTesting: testingIds.has(row.credential_id),
        isSelecting: selectingId.value === row.credential_id,
        isDeleting: deletingId.value === row.credential_id,
        writeInProgress: writeInProgress.value,
        hasActiveTests: hasActiveTests.value,
        isSchedulable: true,
        canTest: true,
        canCheckIn: !row.is_expired,
        isCheckingIn: checkingInIds.has(row.credential_id),
        checkinDisabledReason: row.is_expired ? '凭证已过期，无法签到' : undefined,
        onSelect: (id: string) => selectMutation.mutate(id),
        onTest: (id: string) => testMutation.mutate(id),
        onDelete: (id: string) => deleteMutation.mutate(id),
        onCheckin: (id: string) => checkinMutation.mutate(id),
      }),
  },
];

const tableColumns = columns as unknown as Column[];
const tableRows = computed(() => rows.value as unknown as Record<string, unknown>[]);
</script>

<template>
  <CCard :title="title">
    <template #header-extra>
      <div class="toolbar-actions">
        <CButton
          :loading="toggleRotationMutation.isPending.value"
          :disabled="!autoRotationKnown || writeInProgress || hasActiveTests"
          @click="toggleRotationMutation.mutate()"
        >
          <template #icon>
            <Pause v-if="autoRotationEnabled" :size="16" />
            <Play v-else :size="16" />
          </template>
          {{ rotationToggleLabel }}
        </CButton>
        <RefreshButton :query="credentialsQuery" />
      </div>
    </template>

    <CAlert v-if="loadError" type="error" class="mb-3">凭证加载失败</CAlert>
    <CRadioGroup v-model="filterTab" class="mb-3">
      <CRadioButton value="all">全部 ({{ credentialCounts.all }})</CRadioButton>
      <CRadioButton value="valid">可用 ({{ credentialCounts.valid }})</CRadioButton>
      <CRadioButton value="expired">过期 ({{ credentialCounts.expired }})</CRadioButton>
    </CRadioGroup>
    <CDataTable
      :columns="tableColumns"
      :data="tableRows"
      row-key="credential_id"
      :loading="credentialsQuery.isLoading.value || credentialsQuery.isFetching.value"
      :error="credentialsQuery.isError.value"
      :bordered="false"
      size="small"
    >
      <template #empty>暂无{{ provider === 'trae' ? ' TRAE ' : ' CodeBuddy ' }}凭证</template>
    </CDataTable>
  </CCard>
</template>
