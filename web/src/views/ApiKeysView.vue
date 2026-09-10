<script setup lang="ts">
import { computed, h, ref } from 'vue';
import { useMutation, useQuery, useQueryClient } from '@tanstack/vue-query';
import { Copy, Plus, Trash2 } from '@lucide/vue';
import CAlert from '../components/ui/CAlert.vue';
import CButton from '../components/ui/CButton.vue';
import CCard from '../components/ui/CCard.vue';
import CDataTable, { type Column } from '../components/ui/CDataTable.vue';
import CInput from '../components/ui/CInput.vue';
import CInputGroup from '../components/ui/CInputGroup.vue';
import CPopconfirm from '../components/ui/CPopconfirm.vue';
import CTooltip from '../components/ui/CTooltip.vue';
import { adminApi } from '../api/admin';
import type { ApiKeyRecord } from '../types';
import { useClipboard } from '../composables/useClipboard';
import { useToast } from '../composables/useToast';
import RefreshButton from '../components/RefreshButton.vue';
import { formatDeleteConfirm } from '../utils/apiKeyText';
import { useSessionStore } from '../stores/session';
import { adminQueryKeys } from '../utils/adminQueryKeys';

const queryClient = useQueryClient();
const session = useSessionStore();
const queryKeys = adminQueryKeys(session.username);
const toast = useToast();
const { copy } = useClipboard();
const name = ref('');
const MAX_API_KEY_NAME_LENGTH = 80;
const nameLength = computed(() => name.value.length);
const actionButtonClass = 'table-action-button';

const apiKeysQuery = useQuery({
  queryKey: queryKeys.apiKeys,
  queryFn: adminApi.apiKeys,
});

const rows = computed(() => apiKeysQuery.data.value?.api_keys || []);

function formatMinuteTimestamp(value: number): string {
  return new Date(value * 1000).toLocaleString(undefined, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  });
}

function maskKey(key: string): string {
  if (!key) return '••••••••';
  return key.length > 8 ? `${key.slice(0, 3)}••••••••${key.slice(-4)}` : '••••••••';
}

const createMutation = useMutation({
  mutationFn: () => adminApi.createApiKey(name.value.trim()),
  onSuccess: async (created) => {
    name.value = '';
    toast.success(`API Key「${created.name}」已创建，可在列表复制`);
    await queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys });
  },
  onError: (error) => {
    toast.error(error instanceof Error ? error.message : '创建 API Key 失败');
  },
});

const deleteMutation = useMutation({
  mutationFn: adminApi.deleteApiKey,
  onSuccess: async () => {
    toast.success('API Key 已删除');
    await queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys });
  },
});

function handleCreate() {
  if (createMutation.isPending.value) return;
  if (!name.value.trim()) {
    toast.warning('请输入 API Key 名称');
    return;
  }
  if (name.value.trim().length > MAX_API_KEY_NAME_LENGTH) {
    toast.warning('API Key 名称不能超过 80 个字符');
    return;
  }
  createMutation.mutate();
}

function handleCreateFromEnter(event: KeyboardEvent): void {
  if (event.isComposing) return;
  handleCreate();
}

const columns: Column<ApiKeyRecord>[] = [
  { title: '名称', key: 'name', minWidth: 140 },
  {
    title: 'Key',
    key: 'key',
    minWidth: 220,
    render: (row) =>
      h('div', { class: 'flex items-center gap-2' }, [
        h('span', { class: 'mono' }, maskKey(row.key)),
        h(
          CTooltip,
          { content: '复制 API Key' },
          {
            default: () =>
              h(
                CButton,
                {
                  size: 'sm',
                  variant: 'secondary',
                  shape: 'circle',
                  class: actionButtonClass,
                  'aria-label': '复制 API Key',
                  onClick: () => void copy(row.key, 'API Key 已复制'),
                },
                { icon: () => h(Copy, { size: 14 }) },
              ),
          },
        ),
      ]),
  },
  {
    title: '创建时间',
    key: 'created_at',
    minWidth: 180,
    render: (row) => new Date(row.created_at * 1000).toLocaleString(),
  },
  {
    title: '最近使用',
    key: 'last_used_at',
    minWidth: 180,
    render: (row) => (row.last_used_at ? formatMinuteTimestamp(row.last_used_at) : '-'),
  },
  {
    title: '操作',
    key: 'actions',
    width: 96,
    align: 'left',
    headerClassName: 'table-action-header',
    render: (row) =>
      h('div', { class: 'table-action-group flex items-center justify-start gap-1.5' }, [
        h(
          CTooltip,
          { content: '删除 API Key' },
          {
            default: () =>
              h(
                CPopconfirm,
                {
                  title: formatDeleteConfirm(row.name),
                  confirmVariant: 'danger',
                  onConfirm: () => deleteMutation.mutate(row.id),
                },
                {
                  default: () =>
                    h(
                      CButton,
                      {
                        size: 'sm',
                        variant: 'secondary',
                        shape: 'circle',
                        class: actionButtonClass,
                        'aria-label': '删除 API Key',
                      },
                      { icon: () => h(Trash2, { size: 14 }) },
                    ),
                },
              ),
          },
        ),
      ]),
  },
];

// CDataTable 当前为非泛型组件，传 props 时需 cast 为其默认 Record<string, unknown> 形态
const tableColumns = columns as unknown as Column[];
const tableRows = computed(() => rows.value as unknown as Record<string, unknown>[]);
</script>

<template>
  <div class="section-grid">
    <CCard title="创建 API Key">
      <div class="flex flex-col gap-4">
        <div>
          <CInputGroup>
            <CInput
              v-model="name"
              placeholder="名称"
              :maxlength="MAX_API_KEY_NAME_LENGTH"
              @enter="handleCreateFromEnter"
            />
            <CButton
              variant="primary"
              :loading="createMutation.isPending.value"
              :disabled="createMutation.isPending.value"
              @click="handleCreate"
            >
              <template #icon>
                <Plus :size="16" />
              </template>
              生成
            </CButton>
          </CInputGroup>
          <div class="me-1 mt-1 text-right text-xs text-muted">{{ nameLength }}/80</div>
        </div>
      </div>
    </CCard>

    <CCard title="已创建的 API Key">
      <template #header-extra>
        <RefreshButton :query="apiKeysQuery" />
      </template>

      <CAlert v-if="apiKeysQuery.isError.value" type="error" class="mb-3">
        <div class="toolbar">
          <span>加载 API Key 列表失败</span>
          <RefreshButton :query="apiKeysQuery" label="重试" size="sm" />
        </div>
      </CAlert>

      <CDataTable
        :columns="tableColumns"
        :data="tableRows"
        row-key="id"
        :loading="apiKeysQuery.isLoading.value || apiKeysQuery.isFetching.value"
        :error="apiKeysQuery.isError.value"
        :bordered="false"
        size="small"
      >
        <template #empty>暂无 API Key，点击上方创建</template>
      </CDataTable>
    </CCard>
  </div>
</template>
