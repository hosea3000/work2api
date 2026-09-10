# admin-web-frontend 变更规范（增量）

## MODIFIED Requirements

### Requirement: API Key 页可用
管理台 API 密钥页面 MUST 支持创建（名称唯一）、列表（key 隐藏显示 + 随时复制）、删除。列表每行 SHALL 以掩码形式展示 key 并提供复制按钮，点击复制即写入剪贴板。创建成功后 SHALL 直接刷新列表（不再有"仅显示一次、关闭后无法查看"的强制告警）。创建时名称重复 SHALL 提示"名称已存在"且不创建。

#### Scenario: 随时复制
- **WHEN** 管理员在列表某行点击复制按钮
- **THEN** 该行完整 key 写入剪贴板

#### Scenario: key 隐藏显示
- **WHEN** 管理员查看列表
- **THEN** key 以掩码形式展示，不直接显示完整明文

#### Scenario: 名称重复提示
- **WHEN** 创建时输入的名称与现有 key 重复
- **THEN** 页面提示"名称已存在"，不创建新 key

#### Scenario: 创建后列表可复制
- **WHEN** 管理员创建成功
- **THEN** 列表刷新，新 key 出现在列表中且可复制，无需一次性保存
