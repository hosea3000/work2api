<script setup lang="ts">
import { computed, ref } from 'vue';
import { useQueryClient } from '@tanstack/vue-query';
import { Copy, ExternalLink } from '@lucide/vue';
import CAlert from '../components/ui/CAlert.vue';
import CButton from '../components/ui/CButton.vue';
import CCard from '../components/ui/CCard.vue';
import CInput from '../components/ui/CInput.vue';
import CRadioGroup from '../components/ui/CRadioGroup.vue';
import CRadioButton from '../components/ui/CRadioButton.vue';
import CTag from '../components/ui/CTag.vue';
import { traeLoginApi } from '../api/admin';
import { useOAuthPolling } from '../composables/useOAuthPolling';
import { useClipboard } from '../composables/useClipboard';
import { useToast } from '../composables/useToast';
import CredentialPoolCard from '../components/CredentialPoolCard.vue';
import { useSessionStore } from '../stores/session';
import { adminQueryKeys } from '../utils/adminQueryKeys';

const queryClient = useQueryClient();
const session = useSessionStore();
const queryKeys = adminQueryKeys(session.username);
const toast = useToast();
const { copy } = useClipboard();

const providerTab = ref<'codebuddy' | 'trae'>('codebuddy');

async function invalidateCredentials() {
  await Promise.all([
    queryClient.invalidateQueries({ queryKey: queryKeys.credentials('codebuddy') }),
    queryClient.invalidateQueries({ queryKey: queryKeys.credentials('trae') }),
    queryClient.invalidateQueries({ queryKey: queryKeys.status }),
  ]);
}

/* ─── CodeBuddy OAuth 设备授权轮询 ─── */
const {
  authUrl,
  starting,
  polling,
  elapsedSeconds,
  manualOpenRequired,
  start: startOAuth,
  cancel,
  openAuthUrl,
} = useOAuthPolling({ onSuccess: () => void invalidateCredentials() });
const authInProgress = computed(() => starting.value || polling.value);

function start(): void {
  void startOAuth();
}

function formatElapsed(totalSeconds: number): string {
  const m = Math.floor(totalSeconds / 60);
  const s = totalSeconds % 60;
  return `${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
}

/* ─── TRAE 网页登录 ─── */
const traeStarting = ref(false);
const traePolling = ref(false);
const traeElapsedSeconds = ref(0);
const traeLoginUrl = ref('');
const traePendingId = ref('');
const traePollTimer = ref<ReturnType<typeof setInterval> | null>(null);
const traeImportUrl = ref('');
const traeImporting = ref(false);
const TRAE_POLL_INTERVAL_MS = 2000;
const TRAE_POLL_MAX_SECONDS = 180;

const traeInProgress = computed(() => traeStarting.value || traePolling.value);

function stopTraePolling(): void {
  if (traePollTimer.value) {
    clearInterval(traePollTimer.value);
    traePollTimer.value = null;
  }
}

async function startTraeLogin(): Promise<void> {
  if (traeInProgress.value) return;
  traeStarting.value = true;
  try {
    const res = await traeLoginApi.start();
    traeLoginUrl.value = res.login_url;
    traePendingId.value = res.pending_id;
    traePolling.value = true;
    traeElapsedSeconds.value = 0;
    window.open(res.login_url, '_blank');
    stopTraePolling();
    traePollTimer.value = setInterval(pollTraeResult, TRAE_POLL_INTERVAL_MS);
  } catch (err) {
    toast.error(`生成登录链接失败：${err instanceof Error ? err.message : '未知错误'}`);
  } finally {
    traeStarting.value = false;
  }
}

async function pollTraeResult(): Promise<void> {
  traeElapsedSeconds.value += TRAE_POLL_INTERVAL_MS / 1000;
  if (traeElapsedSeconds.value > TRAE_POLL_MAX_SECONDS) {
    stopTraePolling();
    traePolling.value = false;
    toast.error('登录超时（3 分钟），请重试');
    return;
  }
  try {
    const res = await traeLoginApi.result(traePendingId.value);
    if (res.state === 'success') {
      stopTraePolling();
      traePolling.value = false;
      traePendingId.value = '';
      toast.success(`TRAE 账号已添加：${res.nickname || res.uid || ''}`);
      await invalidateCredentials();
    } else if (res.state === 'failed') {
      stopTraePolling();
      traePolling.value = false;
      traePendingId.value = '';
      toast.error(`登录失败：${res.error || '未知错误'}`);
    }
  } catch {
    stopTraePolling();
    traePolling.value = false;
    traePendingId.value = '';
  }
}

async function cancelTraeLogin(): Promise<void> {
  stopTraePolling();
  traePolling.value = false;
  const pendingId = traePendingId.value;
  traePendingId.value = '';
  if (pendingId) {
    try {
      await traeLoginApi.cancel(pendingId);
    } catch {
      // ignore
    }
  }
}

function openTraeLoginUrl(): void {
  if (traeLoginUrl.value) window.open(traeLoginUrl.value, '_blank');
}

async function copyTraeLoginUrl(): Promise<void> {
  if (!traeLoginUrl.value) return;
  try {
    await copy(traeLoginUrl.value);
    toast.success('登录链接已复制');
  } catch {
    toast.error('复制失败');
  }
}

async function importTraeCallback(): Promise<void> {
  const url = traeImportUrl.value.trim();
  if (!url || traeImporting.value) return;
  traeImporting.value = true;
  try {
    await traeLoginApi.import(url);
    traeImportUrl.value = '';
    toast.success('TRAE 凭证已导入');
    await invalidateCredentials();
  } catch (err) {
    toast.error(`导入失败：${err instanceof Error ? err.message : '回调 URL 无效'}`);
  } finally {
    traeImporting.value = false;
  }
}
</script>

<template>
  <div class="section-grid flex flex-col gap-4">
    <CRadioGroup v-model="providerTab">
      <CRadioButton value="codebuddy">CodeBuddy</CRadioButton>
      <CRadioButton value="trae">TRAE</CRadioButton>
    </CRadioGroup>

    <template v-if="providerTab === 'codebuddy'">
      <CCard title="CodeBuddy 登录认证" class="credential-auth-card">
        <div class="credential-auth-card-content flex flex-col gap-3">
          <div v-if="!authInProgress" class="credential-auth-idle flex flex-col gap-3">
            <p class="text-sm opacity-70">
              登录 CodeBuddy 后，认证凭证会自动保存到 CodeBuddy 凭证池。
            </p>
            <CButton
              variant="primary"
              :loading="starting || polling"
              class="credential-auth-start-button"
              @click="start"
            >
              <template #icon><ExternalLink :size="16" /></template>
              开始认证
            </CButton>
          </div>
          <div v-else class="credential-auth-pending flex flex-col gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <CTag type="warning">等待登录认证</CTag>
              <CTag type="warning">已等待 {{ formatElapsed(elapsedSeconds) }}</CTag>
            </div>
            <CAlert type="info">
              <template v-if="manualOpenRequired && authUrl">
                登录页未能自动打开，请点击“打开登录页”继续认证。
              </template>
              <template v-else>
                请在打开的 CodeBuddy 登录页中完成认证，完成后此处会自动更新。
              </template>
            </CAlert>
            <div class="flex flex-wrap items-center gap-2">
              <CButton :disabled="!authUrl" @click="openAuthUrl">
                <template #icon><ExternalLink :size="16" /></template>
                打开登录页
              </CButton>
              <CButton :disabled="!authUrl" @click="copy(authUrl, '认证链接已复制')">
                <template #icon><Copy :size="16" /></template>
                复制链接
              </CButton>
              <CButton @click="cancel">取消认证</CButton>
            </div>
          </div>
        </div>
      </CCard>
      <CredentialPoolCard provider="codebuddy" title="CodeBuddy 凭证池" />
    </template>

    <template v-else>
      <CCard title="TRAE 登录" class="credential-auth-card">
        <div class="credential-auth-card-content flex flex-col gap-3">
          <div v-if="!traeInProgress" class="credential-auth-idle flex flex-col gap-3">
            <p class="text-sm opacity-70">
              跳转到 TRAE 官网完成登录，凭证会自动导入 TRAE 凭证池。
            </p>
            <CButton
              variant="primary"
              :loading="traeStarting"
              class="credential-auth-start-button"
              @click="startTraeLogin"
            >
              <template #icon><ExternalLink :size="16" /></template>
              添加账号（TRAE 登录）
            </CButton>
          </div>
          <div v-else class="credential-auth-pending flex flex-col gap-3">
            <div class="flex flex-wrap items-center gap-2">
              <CTag type="warning">等待 TRAE 登录</CTag>
              <CTag type="warning">已等待 {{ formatElapsed(traeElapsedSeconds) }}</CTag>
            </div>
            <CAlert type="info">
              请在打开的 TRAE 登录页完成登录；若浏览器提示回调连接失败（远程部署），
              可复制地址栏完整 URL 在下方粘贴导入。
            </CAlert>
            <div class="flex flex-wrap items-center gap-2">
              <CButton :disabled="!traeLoginUrl" @click="openTraeLoginUrl">
                <template #icon><ExternalLink :size="16" /></template>
                打开登录页
              </CButton>
              <CButton :disabled="!traeLoginUrl" @click="copyTraeLoginUrl">
                <template #icon><Copy :size="16" /></template>
                复制链接
              </CButton>
              <CButton @click="cancelTraeLogin">取消登录</CButton>
            </div>
          </div>
          <div class="mt-2 border-t border-current/10 pt-3">
            <p class="mb-2 text-sm opacity-70">回调未自动到达？粘贴地址栏完整回调 URL：</p>
            <CInput
              v-model="traeImportUrl"
              type="password"
              placeholder="http://127.0.0.1:18080/authorize?refreshToken=..."
            />
            <CButton
              class="mt-2"
              :loading="traeImporting"
              :disabled="!traeImportUrl.trim()"
              @click="importTraeCallback"
            >
              导入
            </CButton>
          </div>
        </div>
      </CCard>
      <CredentialPoolCard provider="trae" title="TRAE 凭证池" />
    </template>
  </div>
</template>

<style scoped>
@media (min-width: 1024px) {
  .credential-auth-card :deep(.c-card-body),
  .credential-auth-card-content,
  .credential-auth-idle {
    display: flex;
    flex: 1;
    flex-direction: column;
  }

  .credential-auth-start-button {
    align-self: flex-start;
    margin-top: auto;
  }
}
</style>
