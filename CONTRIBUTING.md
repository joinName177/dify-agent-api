# 贡献规范

## 1. 规范定位

本文件是 Git、Commit、Pull Request、Review 和合并流程的唯一规范。架构边界见 `docs/go-backend-architecture.md`，代码和工具规则见 `engineering-standards.md`，Agent 工作方式见 `AGENTS.md`。

CI 和受保护分支设置是权威门禁。本地 Hook 只提供快速反馈，不得拥有 CI 中不存在的隐藏规则。

## 2. 开始变更

- 从最新受保护主分支创建短生命周期分支，不直接向主分支提交。
- 分支名使用 `<type>/<ticket-id>-<short-kebab-description>`；没有工单时省略 ticket id。
- 一个 PR 只解决一个可审查目标，描述中明确范围和非目标。
- 开工前确认工作区、模块、契约、数据库、Owner 和验收条件。

允许的 type：

```text
feat | fix | refactor | perf | test | docs | build | ci | chore | revert
```

## 3. Commit 规范

Commit 和 PR 标题使用：

```text
type(scope)!: imperative summary
```

- `type` 使用第 2 节的封闭集合。
- `scope` 使用以下封闭集合；新增 scope 必须同时修改检查器和本规范：

```text
user | auth | http | config | db | openapi | build | ci | deps | docs |
security | observability | release | governance | test | architecture |
migration | api | platform
```
- summary 使用祈使语气并描述结果，禁止 `update stuff`、`fix issue` 等无信息文本。
- 破坏性变更使用 `!`，正文说明删除的旧契约、迁移步骤和发布影响。
- 不在 Commit、PR、日志或测试证据中保存 Secret、访问凭据或真实个人数据。

默认使用 Squash Merge。最终 Squash Commit 标题必须与已校验的 PR 标题一致。

首次克隆后运行 `make install-hooks`，把本仓库的 `core.hooksPath` 指向版本化 `.githooks/`。pre-commit 只读取 Git index：检查暂存 Go 文件格式、对暂存差异执行 Secret 扫描，并在 Go 或 module 文件变化时用暂存快照运行测试。commit-msg 与 CI 使用同一个 `make commit-policy` 入口。Hook 不会自动暂存或改写文件。

## 4. 本地命令契约

稳定命令按 `docs/engineering-governance-modernization-task-breakdown.md` 的依赖顺序实现。已存在的 Make target 是当前可执行事实；尚未落地的目标不得在 PR 中声称已验证。

最终命令分为两类：

- 修改型：`make prepare`、`make generate`、`make tidy`、`make fmt`。
- 非修改型：`make governance-check`、`make format-check`、`make mod-check`、`make generated-check`、`make architecture-check`、测试、安全、构建检查和 `make verify`。

`make verify` 必须是非修改型权威聚合门禁。执行前后版本化工作区状态必须一致，不得调用生成、格式化或依赖整理的写回命令。

日常先运行 `make verify`。只有需要主动同步生成物、模块元数据或格式时才运行 `make prepare`，随后再次运行 `make verify`。`make check` 是迁移期兼容别名；当仓库文档、CI 和外部调用者均改用 `make verify` 后删除。

## 5. Pull Request

PR 描述必须包含：

1. 目标、范围和明确非目标。
2. 受影响的模块、公开契约、身份、权限、配置、数据库和发布能力。
3. 关键决策及拒绝的备选；达到 ADR 条件时链接 ADR。
4. 成功、失败、边界、安全、并发、取消和恢复测试证据。
5. OpenAPI、digest、生成物、Schema、迁移、架构和运维文档的同步情况。
6. 发布、停止放量、回滚或前向修复方式。

禁止空 PR 描述、只贴截图、只写“测试通过”或把关键决策留在即时通讯中。

## 6. Review 与合并

- 作者和审查者使用 `docs/code-review.md` 收集证据。
- 架构、身份、权限、数据分类、OpenAPI、迁移、供应链或发布变化必须由对应 Owner 审查。
- 新提交使已有批准失效；所有讨论必须解决。
- Required Checks 全部通过、无未完成占位符且所需批准有效后才能合并。
- 禁止管理员绕过、直接推送、force push、`--no-verify` 规避 CI，或把失败检查改为 optional。
- 合并后删除分支；Release 只从受保护提交和可验证制品创建。

目标 Required Check ID 为：

```text
governance-check
format-check
generated-code-drift
module-check
static-analysis
architecture-check
unit-race-test
migration-check
integration-test
e2e-test
security-check
container-build
```

在对应 Job 实现并于默认分支成功运行前，不得提前将其配置为 Required Check。名称变更必须同步更新 workflow、治理检查、分支保护和本文。

## 7. 依赖、契约和迁移

- 依赖和工具使用精确版本或不可变摘要；一个 PR 不混合无关升级和业务功能。
- OpenAPI 是唯一 HTTP transport 来源；生成物不得手改，digest 和生成漂移必须验证。
- 已发布迁移只追加，不修改、重命名或重新排序。
- 破坏性数据库变更采用 expand、backfill、cutover、contract；生产回滚不得默认执行 down migration。
- 主版本、认证、数据库、OpenAPI 生成器和构建链升级必须建立 ADR 或显式迁移说明。
