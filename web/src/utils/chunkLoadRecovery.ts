import { readonly, ref, type Ref } from 'vue';
import {
  isNavigationFailure,
  NavigationFailureType,
  type NavigationFailure,
  type RouteLocationNormalized,
  type RouteLocationRaw,
  type Router,
} from 'vue-router';

const RECOVERY_STORAGE_KEY = 'codebuddy2api:chunk-reload-attempted';
const RECOVERY_RECORD_VERSION = 1;
const FALLBACK_RELOAD_STATUS_DELAY_MS = 1_000;
const DYNAMIC_IMPORT_FETCH_ERROR_PATTERN =
  /^(?:Failed to fetch dynamically imported module(?::|$)|error loading dynamically imported module(?::|$)|Importing a module script failed\.?$)/i;
const VITE_CSS_PRELOAD_ERROR_PREFIX = 'Unable to preload CSS for ';

type NavigationMode = 'push' | 'replace';
type NavigationPromise = Promise<NavigationFailure | void>;

interface RecoveryRecord {
  version: typeof RECOVERY_RECORD_VERSION;
  target: string;
  mode: NavigationMode;
}

interface NavigationIntent {
  target: string;
  mode: NavigationMode;
  order: number;
  resume: boolean;
  superseded: boolean;
  chunkFailed: boolean;
  unexpectedReported: boolean;
}

interface StartupRecovery {
  record: RecoveryRecord;
  intent?: NavigationIntent;
}

interface HistoryNavigationAttempt {
  type: 'history';
  target: string;
  order: number;
  route: RouteLocationNormalized | null;
}

type LatestNavigationAttempt =
  { type: 'intent'; intent: NavigationIntent } | HistoryNavigationAttempt;

interface InstallContext {
  generation: number;
  router: Router;
  onUnexpectedNavigationError: (error: unknown) => void;
  preloadErrors: WeakSet<Error>;
  routeIntents: WeakMap<object, NavigationIntent>;
  routeNavigationOrders: WeakMap<object, number>;
  intents: Set<NavigationIntent>;
  latestNavigationOrder: number;
  latestNavigationAttempt: LatestNavigationAttempt | null;
  startupRecovery: StartupRecovery | null;
  automaticReload: 'available' | 'recorded' | 'disabled';
  successfulRouteSettled: boolean;
}

type RecoveryPhase =
  | { type: 'idle' }
  | { type: 'waiting'; record: RecoveryRecord }
  | { type: 'reloading'; record: RecoveryRecord; fallbackReloadPending: boolean }
  | { type: 'failed'; record: RecoveryRecord; fallbackReloadPending: boolean };

interface PageNavigation {
  readonly currentEntry: NavigationHistoryEntry | null;
  reload(): NavigationResult;
}

interface ReloadAttempt {
  record: RecoveryRecord;
}

interface ChunkLoadRecoveryOptions {
  getStorage?: () => Storage;
  getNavigation?: () => PageNavigation | undefined;
  reloadPage?: () => void;
  stopPageLoading?: () => void;
}

interface ChunkLoadRecoveryInstallOptions {
  onUnexpectedNavigationError: (error: unknown) => void;
}

export interface ChunkLoadFailure {
  canStay: boolean;
}

export interface ChunkLoadRecovery {
  readonly failure: Readonly<Ref<ChunkLoadFailure | null>>;
  install(router: Router, options: ChunkLoadRecoveryInstallOptions): () => void;
  push(to: RouteLocationRaw): NavigationPromise;
  replace(to: RouteLocationRaw): NavigationPromise;
  deferReload(): () => void;
  retryReload(): void;
  stayOnCurrentPage(): void;
}

function isRecoveryRecord(value: unknown): value is RecoveryRecord {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) return false;
  const record = value as Record<string, unknown>;
  const keys = Object.keys(record).sort();
  return (
    keys.length === 3 &&
    keys[0] === 'mode' &&
    keys[1] === 'target' &&
    keys[2] === 'version' &&
    record.version === RECOVERY_RECORD_VERSION &&
    (record.mode === 'push' || record.mode === 'replace') &&
    typeof record.target === 'string' &&
    record.target.startsWith('/') &&
    !record.target.startsWith('//') &&
    // oxlint-disable-next-line eslint/no-control-regex -- 恢复目标不得包含 URL 控制字符。
    !/[\u0000-\u001f\u007f]/.test(record.target)
  );
}

function navigationModeForPush(to: RouteLocationRaw): NavigationMode {
  return typeof to === 'object' && to.replace === true ? 'replace' : 'push';
}

function isResourcePreloadError(error: Error): boolean {
  return (
    (error instanceof TypeError && DYNAMIC_IMPORT_FETCH_ERROR_PATTERN.test(error.message)) ||
    error.message.startsWith(VITE_CSS_PRELOAD_ERROR_PREFIX)
  );
}

/**
 * 恢复 Vite 构建更新后失效的路由页面 chunk。
 *
 * 只有通过 push()/replace() 发起的导航才携带历史语义；初始导航和浏览器历史导航
 * 失败时使用 replace。恢复记录跨一次刷新保存，成功、重定向或导航中止后立即消费。
 */
export function createChunkLoadRecovery({
  getStorage = () => window.sessionStorage,
  getNavigation = () => window.navigation as Navigation | undefined,
  reloadPage = window.location.reload.bind(window.location),
  stopPageLoading = window.stop.bind(window),
}: ChunkLoadRecoveryOptions = {}): ChunkLoadRecovery {
  const failureState = ref<ChunkLoadFailure | null>(null);
  let context: InstallContext | null = null;
  let generation = 0;
  let phase: RecoveryPhase = { type: 'idle' };
  let reloadDeferrals = 0;
  let fallbackReloadStatusTimer: number | undefined;
  let activeReloadAttempt: ReloadAttempt | null = null;

  function removeRecoveryRecord(): void {
    try {
      getStorage().removeItem(RECOVERY_STORAGE_KEY);
    } catch {
      // sessionStorage 可能被浏览器策略禁用；内存状态仍可继续收敛。
    }
  }

  function writeRecoveryRecord(record: RecoveryRecord): boolean {
    try {
      getStorage().setItem(RECOVERY_STORAGE_KEY, JSON.stringify(record));
      return true;
    } catch {
      return false;
    }
  }

  function readRecoveryRecord(): RecoveryRecord | null {
    let storage: Storage;
    let serialized: string | null;
    try {
      storage = getStorage();
      serialized = storage.getItem(RECOVERY_STORAGE_KEY);
    } catch {
      return null;
    }
    if (serialized === null) return null;

    try {
      const parsed: unknown = JSON.parse(serialized);
      if (isRecoveryRecord(parsed)) return parsed;
      storage.removeItem(RECOVERY_STORAGE_KEY);
    } catch {
      // 损坏记录或拒绝删除的 sessionStorage 都不能阻止应用启动。
      try {
        storage.removeItem(RECOVERY_STORAGE_KEY);
      } catch {
        // 浏览器拒绝清理时仅在本次文档内忽略该记录。
      }
    }
    return null;
  }

  function cancelReloadAttempt(): void {
    activeReloadAttempt = null;
    if (fallbackReloadStatusTimer === undefined) return;
    window.clearTimeout(fallbackReloadStatusTimer);
    fallbackReloadStatusTimer = undefined;
  }

  function clearPendingFallbackReload(): void {
    if ((phase.type === 'reloading' || phase.type === 'failed') && phase.fallbackReloadPending) {
      phase = { ...phase, fallbackReloadPending: false };
    }
  }

  function stopPendingPageLoad(): void {
    if (phase.type !== 'reloading' && phase.type !== 'failed') return;
    const hasPendingFallbackReload = phase.fallbackReloadPending;
    if (activeReloadAttempt === null && !hasPendingFallbackReload) return;
    const pendingPhase = phase;
    stopPageLoading();
    if (phase === pendingPhase && hasPendingFallbackReload) {
      phase = { ...pendingPhase, fallbackReloadPending: false };
    }
  }

  function pausePendingReload(): void {
    if (phase.type !== 'reloading' && phase.type !== 'failed') return;
    if (activeReloadAttempt === null && !phase.fallbackReloadPending) return;
    const record = phase.record;
    cancelReloadAttempt();
    phase = { type: 'waiting', record };
    failureState.value = null;
    stopPageLoading();
  }

  function showFailure(
    activeContext: InstallContext,
    record: RecoveryRecord,
    fallbackReloadPending = false,
  ): void {
    cancelReloadAttempt();
    phase = { type: 'failed', record, fallbackReloadPending };
    failureState.value = { canStay: activeContext.successfulRouteSettled };
  }

  function startReload(activeContext: InstallContext, record: RecoveryRecord): void {
    phase = { type: 'reloading', record, fallbackReloadPending: false };
    failureState.value = null;
    const reloadAttempt: ReloadAttempt = { record };
    activeReloadAttempt = reloadAttempt;

    let navigationCommitted: Promise<NavigationHistoryEntry> | undefined;
    try {
      const navigation = getNavigation();
      if (navigation !== undefined && navigation.currentEntry !== null) {
        const { committed, finished } = navigation.reload();
        navigationCommitted = committed;
        void finished?.catch(() => undefined);
      } else {
        reloadPage();
      }
    } catch {
      showFailure(activeContext, reloadAttempt.record);
      return;
    }

    if (activeReloadAttempt !== reloadAttempt) return;
    if (navigationCommitted !== undefined) {
      void navigationCommitted.catch(() => {
        if (activeReloadAttempt === reloadAttempt) {
          showFailure(activeContext, reloadAttempt.record);
        }
      });
      return;
    }

    // 旧浏览器无法区分 reload 被 beforeunload 取消与入口响应缓慢。超时只展示
    // 恢复入口；只有用户明确留页、重试或改为其他导航时才停止待处理刷新。
    phase = { type: 'reloading', record, fallbackReloadPending: true };
    fallbackReloadStatusTimer = window.setTimeout(() => {
      fallbackReloadStatusTimer = undefined;
      if (activeReloadAttempt === reloadAttempt) {
        showFailure(activeContext, reloadAttempt.record, true);
      }
    }, FALLBACK_RELOAD_STATUS_DELAY_MS);
  }

  function runWaitingReload(activeContext: InstallContext): void {
    if (phase.type !== 'waiting' || reloadDeferrals > 0) return;
    startReload(activeContext, phase.record);
  }

  function consumeStartupRecovery(activeContext: InstallContext): void {
    if (activeContext.startupRecovery === null) return;
    activeContext.startupRecovery = null;
    removeRecoveryRecord();
    activeContext.automaticReload = 'available';
  }

  function supersedeOtherIntents(
    activeContext: InstallContext,
    currentIntent?: NavigationIntent,
  ): void {
    for (const intent of activeContext.intents) {
      if (intent !== currentIntent) intent.superseded = true;
    }
  }

  function cancelRecoverableNavigation(
    activeContext: InstallContext,
    cancelReloadInProgress = false,
  ): void {
    supersedeOtherIntents(activeContext);

    if (activeContext.startupRecovery !== null) {
      consumeStartupRecovery(activeContext);
    }

    const shouldCancel =
      phase.type === 'waiting' ||
      (phase.type === 'failed' && (!phase.fallbackReloadPending || cancelReloadInProgress)) ||
      (cancelReloadInProgress && phase.type === 'reloading');
    if (shouldCancel) {
      stopPendingPageLoad();
      cancelReloadAttempt();
      removeRecoveryRecord();
      phase = { type: 'idle' };
      failureState.value = null;
      if (activeContext.automaticReload !== 'disabled') {
        activeContext.automaticReload = 'available';
      }
    }
  }

  function notifyUnexpected(
    activeContext: InstallContext,
    error: unknown,
    intent?: NavigationIntent,
  ): void {
    if (intent?.unexpectedReported) return;
    if (intent) intent.unexpectedReported = true;
    try {
      activeContext.onUnexpectedNavigationError(error);
    } catch (reportingError) {
      // 错误展示失败不能替换 Router 原本要返回给调用方的错误。
      console.error(reportingError);
    }
  }

  function requestRecovery(activeContext: InstallContext, record: RecoveryRecord): void {
    const effectiveRecord = activeContext.startupRecovery?.record ?? record;
    if (phase.type !== 'idle') {
      if (writeRecoveryRecord(effectiveRecord)) {
        phase = { ...phase, record: effectiveRecord };
        if (activeReloadAttempt !== null) activeReloadAttempt.record = effectiveRecord;
      } else {
        stopPendingPageLoad();
        removeRecoveryRecord();
        showFailure(activeContext, effectiveRecord);
      }
      return;
    }

    if (activeContext.automaticReload !== 'available') {
      writeRecoveryRecord(effectiveRecord);
      showFailure(activeContext, effectiveRecord);
      return;
    }

    activeContext.automaticReload = 'recorded';
    if (!writeRecoveryRecord(effectiveRecord)) {
      showFailure(activeContext, effectiveRecord);
      return;
    }

    phase = { type: 'waiting', record: effectiveRecord };
    runWaitingReload(activeContext);
  }

  function findIntent(
    activeContext: InstallContext,
    to: RouteLocationNormalized,
  ): NavigationIntent | undefined {
    const redirectedFrom = to.redirectedFrom?.fullPath;
    for (const intent of activeContext.intents) {
      if (
        intent.superseded ||
        (intent.target !== to.fullPath && intent.target !== redirectedFrom)
      ) {
        continue;
      }
      return intent;
    }
    return undefined;
  }

  function finishIntent(activeContext: InstallContext, intent: NavigationIntent): void {
    activeContext.intents.delete(intent);
  }

  function startNavigation(
    activeContext: InstallContext,
    to: RouteLocationRaw,
    callMode: NavigationMode,
    recoveryMode: NavigationMode,
    resume: boolean,
  ): { intent: NavigationIntent; promise: NavigationPromise } {
    const pendingFallbackFailure =
      !resume && phase.type === 'failed' && phase.fallbackReloadPending ? phase : null;
    if (!resume) cancelRecoverableNavigation(activeContext);

    const consumePendingFallbackFailure = (intent?: NavigationIntent): void => {
      if (
        pendingFallbackFailure !== null &&
        phase === pendingFallbackFailure &&
        !intent?.superseded
      ) {
        cancelRecoverableNavigation(activeContext, true);
      }
    };

    let target: string;
    try {
      target = activeContext.router.resolve(to).fullPath;
    } catch (error) {
      consumePendingFallbackFailure();
      notifyUnexpected(activeContext, error);
      throw error;
    }

    const intent: NavigationIntent = {
      target,
      mode: recoveryMode,
      order: ++activeContext.latestNavigationOrder,
      resume,
      superseded: false,
      chunkFailed: false,
      unexpectedReported: false,
    };
    activeContext.intents.add(intent);

    let routerPromise: ReturnType<Router['push']>;
    try {
      routerPromise = activeContext.router[callMode](to);
    } catch (error) {
      finishIntent(activeContext, intent);
      consumePendingFallbackFailure(intent);
      notifyUnexpected(activeContext, error, intent);
      throw error;
    }
    activeContext.latestNavigationAttempt = { type: 'intent', intent };

    const promise = routerPromise.then(
      (result) => {
        finishIntent(activeContext, intent);
        if (
          context === activeContext &&
          phase.type !== 'idle' &&
          !intent.superseded &&
          (isNavigationFailure(result, NavigationFailureType.aborted) ||
            isNavigationFailure(result, NavigationFailureType.duplicated))
        ) {
          cancelRecoverableNavigation(activeContext, true);
        }
        consumePendingFallbackFailure(intent);
        return result;
      },
      (error: unknown) => {
        finishIntent(activeContext, intent);
        if (intent.chunkFailed) return undefined;
        consumePendingFallbackFailure(intent);
        notifyUnexpected(activeContext, error, intent);
        throw error;
      },
    );
    return { intent, promise };
  }

  function beginStartupResume(
    activeContext: InstallContext,
    startupRecovery: StartupRecovery,
  ): void {
    try {
      const navigation = startNavigation(
        activeContext,
        startupRecovery.record.target,
        startupRecovery.record.mode,
        startupRecovery.record.mode,
        true,
      );
      startupRecovery.intent = navigation.intent;
      // 守卫可能先于本恢复器中止导航，导致 beforeEach 无法绑定 intent；Promise
      // 完成是这一路径消费记录的最终屏障。chunk 失败则必须保留记录供手动重试。
      void navigation.promise.then(
        () => {
          if (
            context === activeContext &&
            !navigation.intent.chunkFailed &&
            activeContext.startupRecovery === startupRecovery
          ) {
            consumeStartupRecovery(activeContext);
          }
        },
        () => undefined,
      );
    } catch {
      consumeStartupRecovery(activeContext);
    }
  }

  function install(
    router: Router,
    { onUnexpectedNavigationError }: ChunkLoadRecoveryInstallOptions,
  ): () => void {
    if (context !== null) throw new Error('chunk 加载恢复器已安装');

    const startupRecord = readRecoveryRecord();
    const activeContext: InstallContext = {
      generation: ++generation,
      router,
      onUnexpectedNavigationError,
      preloadErrors: new WeakSet<Error>(),
      routeIntents: new WeakMap<object, NavigationIntent>(),
      routeNavigationOrders: new WeakMap<object, number>(),
      intents: new Set<NavigationIntent>(),
      latestNavigationOrder: 0,
      latestNavigationAttempt: null,
      startupRecovery: startupRecord === null ? null : { record: startupRecord },
      automaticReload: startupRecord === null ? 'available' : 'recorded',
      successfulRouteSettled: false,
    };
    context = activeContext;
    phase = { type: 'idle' };
    failureState.value = null;
    reloadDeferrals = 0;
    activeReloadAttempt = null;

    const handlePreloadError = (event: VitePreloadErrorEvent) => {
      activeContext.preloadErrors.add(event.payload);
    };
    const handlePageHide = () => {
      cancelReloadAttempt();
      clearPendingFallbackReload();
    };
    const removeHistoryNavigation = router.options.history.listen((to) => {
      activeContext.latestNavigationAttempt = {
        type: 'history',
        target: router.resolve(to).fullPath,
        order: ++activeContext.latestNavigationOrder,
        route: null,
      };
      supersedeOtherIntents(activeContext);
    });
    const removeBeforeEach = router.beforeEach((to) => {
      const latestNavigationAttempt = activeContext.latestNavigationAttempt;
      const intent = findIntent(activeContext, to);
      let navigationOrder: number;
      if (
        latestNavigationAttempt?.type === 'history' &&
        latestNavigationAttempt.route === null &&
        [to.fullPath, to.redirectedFrom?.fullPath].includes(latestNavigationAttempt.target)
      ) {
        latestNavigationAttempt.route = to;
        navigationOrder = latestNavigationAttempt.order;
      } else if (intent !== undefined) {
        navigationOrder = intent.order;
      } else {
        navigationOrder = ++activeContext.latestNavigationOrder;
      }
      activeContext.routeNavigationOrders.set(to, navigationOrder);
      if (intent !== undefined) activeContext.routeIntents.set(to, intent);
      supersedeOtherIntents(activeContext, intent);
      const resumeIntent = activeContext.startupRecovery?.intent;
      if (resumeIntent?.superseded) {
        consumeStartupRecovery(activeContext);
      }
    });
    const removeError = router.onError((error, to) => {
      const intent = activeContext.routeIntents.get(to);
      const navigationOrder = activeContext.routeNavigationOrders.get(to);
      if (
        error instanceof Error &&
        activeContext.preloadErrors.has(error) &&
        isResourcePreloadError(error)
      ) {
        if (intent) intent.chunkFailed = true;
        if (
          intent?.superseded ||
          (navigationOrder !== undefined && navigationOrder < activeContext.latestNavigationOrder)
        ) {
          return;
        }
        requestRecovery(activeContext, {
          version: RECOVERY_RECORD_VERSION,
          target: intent?.target ?? to.redirectedFrom?.fullPath ?? to.fullPath,
          mode: intent?.mode ?? 'replace',
        });
        return;
      }

      if (intent?.resume) consumeStartupRecovery(activeContext);
      notifyUnexpected(activeContext, error, intent);
    });
    const removeAfterEach = router.afterEach((to, _from, failure) => {
      const intent = activeContext.routeIntents.get(to);
      const latestNavigationAttempt = activeContext.latestNavigationAttempt;
      const isCurrentAbortedNavigation =
        intent === undefined
          ? latestNavigationAttempt !== null &&
            latestNavigationAttempt.type === 'history' &&
            (latestNavigationAttempt.route === to ||
              (latestNavigationAttempt.route === null &&
                [to.fullPath, to.redirectedFrom?.fullPath].includes(
                  latestNavigationAttempt.target,
                )))
          : !intent.superseded;
      if (!failure) activeContext.successfulRouteSettled = true;

      if (
        phase.type !== 'idle' &&
        isCurrentAbortedNavigation &&
        isNavigationFailure(failure, NavigationFailureType.aborted)
      ) {
        cancelRecoverableNavigation(activeContext, true);
        return;
      }

      if (!failure && phase.type !== 'idle' && (intent === undefined || !intent.resume)) {
        cancelRecoverableNavigation(activeContext, true);
        return;
      }

      const startupRecovery = activeContext.startupRecovery;
      if (startupRecovery === null) return;
      if (startupRecovery.intent !== undefined) {
        if (intent !== startupRecovery.intent || startupRecovery.intent.superseded) return;
        consumeStartupRecovery(activeContext);
        return;
      }

      if (failure) {
        consumeStartupRecovery(activeContext);
        return;
      }
      if (
        startupRecovery.record.target === to.fullPath ||
        startupRecovery.record.target === to.redirectedFrom?.fullPath
      ) {
        consumeStartupRecovery(activeContext);
        return;
      }
      beginStartupResume(activeContext, startupRecovery);
    });

    window.addEventListener('vite:preloadError', handlePreloadError);
    window.addEventListener('pagehide', handlePageHide);
    let active = true;

    return () => {
      if (!active) return;
      active = false;
      window.removeEventListener('vite:preloadError', handlePreloadError);
      window.removeEventListener('pagehide', handlePageHide);
      cancelReloadAttempt();
      removeHistoryNavigation();
      removeBeforeEach();
      removeAfterEach();
      removeError();
      activeContext.intents.clear();
      context = null;
      phase = { type: 'idle' };
      failureState.value = null;
      reloadDeferrals = 0;
      activeReloadAttempt = null;
    };
  }

  function activeInstall(): InstallContext {
    if (context === null) throw new Error('chunk 加载恢复器尚未安装');
    return context;
  }

  function push(to: RouteLocationRaw): NavigationPromise {
    const activeContext = activeInstall();
    const mode = navigationModeForPush(to);
    return startNavigation(activeContext, to, 'push', mode, false).promise;
  }

  function replace(to: RouteLocationRaw): NavigationPromise {
    const activeContext = activeInstall();
    return startNavigation(activeContext, to, 'replace', 'replace', false).promise;
  }

  function deferReload(): () => void {
    const activeContext = activeInstall();
    const leaseGeneration = activeContext.generation;
    reloadDeferrals += 1;
    if (reloadDeferrals === 1) pausePendingReload();
    let released = false;

    return () => {
      if (released) return;
      released = true;
      if (context?.generation !== leaseGeneration) return;
      reloadDeferrals -= 1;
      runWaitingReload(activeContext);
    };
  }

  function retryReload(): void {
    const activeContext = context;
    if (activeContext === null || phase.type !== 'failed') return;
    const record = phase.record;
    stopPendingPageLoad();
    writeRecoveryRecord(record);
    phase = { type: 'waiting', record };
    failureState.value = null;
    runWaitingReload(activeContext);
  }

  function stayOnCurrentPage(): void {
    const activeContext = context;
    if (activeContext === null || phase.type !== 'failed' || !failureState.value?.canStay) return;
    stopPendingPageLoad();
    removeRecoveryRecord();
    activeContext.startupRecovery = null;
    activeContext.automaticReload = 'disabled';
    phase = { type: 'idle' };
    failureState.value = null;
  }

  return {
    failure: readonly(failureState),
    install,
    push,
    replace,
    deferReload,
    retryReload,
    stayOnCurrentPage,
  };
}

export const chunkLoadRecovery = createChunkLoadRecovery();
