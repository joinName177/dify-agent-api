# 工程治理改造基线

## 记录时间

2026-07-28，分支 `main`。

## 工作区

执行基线前，已跟踪文件无修改；存在本轮新增但尚未提交的任务拆分文档：

```text
?? docs/engineering-governance-modernization-task-breakdown.md
```

## 工具链和契约

- `go.mod`：`go 1.26.0`，`toolchain go1.26.5`。
- 本地构建镜像：`golang:1.26.5-alpine3.23`，使用 SHA-256 digest。
- race 镜像：`golang:1.26.5-bookworm`，使用 SHA-256 digest。
- OpenAPI digest：`49184e9d62e836a155fd2f6ed555107cfdb657f8f2dd5a7dd6dc7cd36b56ae2d`。
- 已有迁移版本：`000001`、`000002`，均有 up/down 文件。

## 现有命令验证

执行：

```text
make check
```

结果：通过。实际执行 generate、digest、`go mod tidy`、`gofmt -w`、vet、Staticcheck、Govulncheck、unit test、race test、容器构建和受限路径 `git diff --exit-code`。

复核结果：

- 命令包含写回操作，因此不是非修改型验证命令。
- 本次执行后，生成物、OpenAPI digest、`go.mod`、`go.sum`、`cmd` 和 `internal` 没有已跟踪差异。
- Govulncheck 报告代码可达漏洞为 0；依赖 module 中存在 15 个未被当前代码调用的已知漏洞信号，需要后续安全门禁持续跟踪，不能解释为依赖完全无漏洞。
- 现有 `make check` 不执行 E2E、PostgreSQL 集成、迁移验证、配置 Schema、架构、许可证、Secret 或 SAST 检查。

## CI 和 GitHub 外部状态

- workflow 只有 `quality` 和 `container` 两个 Job。
- `quality` 重复维护生成器和分析器版本，没有调用统一 Makefile 检查入口。
- GitHub 仓库默认分支为 `main`，当前连接身份具有 admin 权限。
- GitHub Branches 设置明确显示没有 Classic Branch Protection。
- GitHub Rulesets 设置明确显示没有创建任何 Ruleset。
- 因此 main 当前没有由 Branch Protection 或 Ruleset 提供的合并阻断证据。

## 基线结论

当前代码和容器构建可以通过既有检查，但门禁具有修改工作区、CI 粒度粗、命令重复和关键治理/安全/集成验证缺失的问题。后续任务必须以本基线区分存量缺口和改造回归。

