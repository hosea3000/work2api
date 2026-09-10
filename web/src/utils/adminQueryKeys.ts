/** 为所有按系统用户隔离的管理台数据生成统一查询键。 */
export function adminQueryKeys(username: string) {
  const root = ['admin', username] as const;

  return {
    all: root,
    status: [...root, 'status'] as const,
    credentials: (provider: string) => [...root, 'credentials', provider] as const,
    apiKeys: [...root, 'api-keys'] as const,
    settings: [...root, 'settings'] as const,
    playgroundModels: (provider: string) => [...root, 'playground', provider, 'models'] as const,
    statsOverview: (params: unknown) => [...root, 'stats', 'overview', params] as const,
  };
}
