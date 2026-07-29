# ADR 0003：工程治理工具链与稳定门禁

## 状态

已接受，按开发任务拆分逐步实施。

## 日期

2026-07-28

## 背景

仓库已有 Docker 化 Go 工具链、OpenAPI 生成、Staticcheck、Govulncheck、race test、容器构建和 SBOM，但本地 `make check` 会写回生成物、module 和格式；CI 只提供粗粒度 `quality` 与 `container` Job，且本地和 CI 重复维护命令。仓库还缺少架构边界、Secret、许可证、SAST、容器漏洞和治理一致性门禁。

工程治理需要可复现、可审查且不引入第二技术体系的工具组合。工具选择必须考虑自身供应链风险，不能使用浮动 `latest`、未锁定 Action tag 或在线安装脚本。

## 决策

### 仓库内检查

- Architecture Profile 使用 JSON 和 JSON Schema Draft 2020-12。
- Profile 校验复用 Go 生态的 `github.com/santhosh-tekuri/jsonschema/v6`；接入时固定精确 module 版本。
- 架构依赖检查使用仓库内 Go 工具读取 `go list -deps -json`，不增加功能重叠的第三方依赖检查器。
- 治理、Commit、暂存快照和工具链一致性检查使用仓库内 Go 或 shell 工具，并具备正向和负向测试。
- 本地 Hook 使用版本化 `.githooks/` 和显式安装命令，不引入 Node、Python 或额外 Hook 框架。Hook 只做快速反馈，CI 重新执行权威规则。

### 安全与供应链工具

| 能力 | 工具 | 锁定版本 | 锁定方式 |
| --- | --- | --- | --- |
| Secret 扫描 | Gitleaks | `v8.30.1` | `zricethezav/gitleaks:v8.30.1@sha256:c00b6bd0aeb3071cbcb79009cb16a60dd9e0a7c60e2be9ab65d25e6bc8abbb7f` |
| Go SAST | Gosec | `v2.28.0` | `go run` 精确 module 版本或官方容器 digest |
| Go 许可证清单 | go-licenses | `v2.0.1` | `go run github.com/google/go-licenses/v2@v2.0.1` |
| 容器/SBOM 漏洞 | Grype | `v0.116.0` | 官方容器版本与 digest |
| Go 可达漏洞 | Govulncheck | `v1.6.0` | 保留现有精确 module 版本 |

接入任务必须再次核验 tag 来源，并提交实际容器 digest 或下载校验和。不得使用浮动 `latest`、远程 `curl | sh` 或只锁定可移动 Action tag。

不选用 Trivy 作为本轮容器扫描器。其官方在 2026 年披露发布生态供应链被入侵事件；即使后续版本已修复，本轮优先选择不同发布链并固定不可变摘要。未来替换扫描器必须建立新 ADR，比较检测覆盖、数据库更新、安全历史和迁移成本。

### 许可证政策

扫描器负责生成和校验清单，不代替法律审批。初始允许集合必须在 SEC-502 中由项目 Owner 明确批准；在批准前许可证门禁只能报告，不能标记完成。未知许可证、无许可证和未批准许可证最终都必须阻断。

### 稳定 CI Check ID

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

CI 只调用 Makefile 稳定目标。Check ID 改名必须在同一变更中更新 workflow、治理检查、贡献规范和 GitHub 分支保护。

## 备选方案

- 在 CI 中直接拼接全部命令：会与本地 Makefile 漂移，拒绝采用。
- 使用多个功能重叠的 Secret 或容器扫描器作为永久门禁：增加噪声和维护成本，除非风险矩阵证明必要，否则拒绝采用。
- 为治理工具引入 Node 工具链：当前仓库没有 Node 运行目标，会扩大供应链和本地前置条件，拒绝采用。
- 只写文档、不实现负向测试：无法证明门禁有效，拒绝采用。

## 后果

- 将新增工具下载、漏洞数据库和容器镜像成本，需要通过缓存和职责拆分控制 CI 时间。
- 精确锁定提高可复现性，但必须配套定期更新策略，否则会形成过期风险。
- 本地 Go 检查器需要持续维护测试 fixture；静态 import 检查不能替代语义架构 Review。
- GitHub 分支保护属于外部状态，只有实际启用并完成故障注入后才能标记完成。

## 参考

- Gitleaks: https://github.com/gitleaks/gitleaks
- Gosec: https://github.com/securego/gosec
- go-licenses: https://github.com/google/go-licenses
- Grype: https://github.com/anchore/grype
- Trivy 官方安全公告: https://github.com/aquasecurity/trivy/security/advisories/GHSA-69fq-xp46-6x23
