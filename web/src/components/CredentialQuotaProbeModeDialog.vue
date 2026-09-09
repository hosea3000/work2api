<script setup lang="ts">
import { ref, watch } from 'vue';
import { useMutation, useQueryClient } from '@tanstack/vue-query';
import type { CredentialQuotaProbeMode, CredentialRecord, CredentialsResponse } from '../types';
import { adminApi } from '../api/admin';
import { useSessionStore } from '../stores/session';
import { useToast } from '../composables/useToast';
import { adminQueryKeys } from '../utils/adminQueryKeys';
import CButton from './ui/CButton.vue';
import CModal from './ui/CModal.vue';
import CRadioButton from './ui/CRadioButton.vue';
import CRadioGroup from './ui/CRadioGroup.vue';

const props = defineProps<{
  open: boolean;
  credential: CredentialRecord | null;
}>();
const emit = defineEmits<{ close: []; updating: [value: boolean] }>();
const session = useSessionStore();
const queryClient = useQueryClient();
const queryKeys = adminQueryKeys(session.username);
const toast = useToast();
const mode = ref<CredentialQuotaProbeMode>('personal');
let updateSucceeded = false;

function reset(): void {
  mode.value = props.credential?.quota_probe_mode ?? 'personal';
}

watch(
  () => [props.open, props.credential?.credential_id] as const,
  ([open, credentialId], previous) => {
    if (open && (!previous?.[0] || credentialId !== previous[1])) reset();
  },
  { immediate: true },
);

const updateMutation = useMutation({
  mutationFn: (selectedMode: CredentialQuotaProbeMode) =>
    adminApi.updateCredentialQuotaProbeMode(props.credential!.credential_id, selectedMode),
  networkMode: 'always',
  onMutate: () => {
    updateSucceeded = false;
    emit('updating', true);
  },
  onSuccess: async (result, selectedMode) => {
    updateSucceeded = true;
    queryClient.setQueryData<CredentialsResponse>(queryKeys.credentials, (old) => {
      if (!old) return old;
      return {
        ...old,
        credentials: old.credentials.map((credential) =>
          credential.credential_id === result.credential.credential_id
            ? result.credential
            : credential,
        ),
      };
    });
    toast.success(
      selectedMode === 'enterprise' ? '已切换为企业版额度探测' : '已切换为个人版额度探测',
    );
    await queryClient.invalidateQueries({ queryKey: queryKeys.credentials });
  },
  onSettled: () => {
    emit('updating', false);
    if (updateSucceeded) emit('close');
    updateSucceeded = false;
  },
});

function close(): void {
  if (!updateMutation.isPending.value) emit('close');
}

function submit(): void {
  if (!props.credential || updateMutation.isPending.value) return;
  updateMutation.mutate(mode.value);
}
</script>

<template>
  <CModal
    :open="open"
    title="额度探测方式"
    :closable="!updateMutation.isPending.value"
    @update:open="close"
  >
    <p class="mb-4 text-sm text-muted">
      此设置只影响额度探测，不会改变真实账号类型，也不会影响聊天、模型查询、凭证测试或签到。
    </p>
    <CRadioGroup v-model="mode" aria-label="额度探测方式">
      <CRadioButton value="personal">个人版</CRadioButton>
      <CRadioButton value="enterprise">企业版</CRadioButton>
    </CRadioGroup>
    <p class="mt-3 text-xs text-muted">
      {{ mode === 'enterprise' ? '读取企业总额度。' : '读取个人套餐额度。' }}
    </p>
    <template #footer>
      <CButton :disabled="updateMutation.isPending.value" @click="close">取消</CButton>
      <CButton
        variant="primary"
        :loading="updateMutation.isPending.value"
        :disabled="!credential"
        @click="submit"
      >
        保存并探测
      </CButton>
    </template>
  </CModal>
</template>
