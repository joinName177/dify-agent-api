# 发布、停止放量与回滚

Owner：`@joinName177`。应用与迁移是独立制品；只从受保护提交构建，镜像使用不可变 digest、SBOM 和 provenance。

发布顺序：完成所有适用 Required Checks和 Owner Review；确认配置 Schema、OpenAPI digest、迁移 manifest 与 release id；对数据库变化执行 expand → backfill → cutover → contract，contract 不与依赖旧结构的应用同批；先迁移、再小流量 API、再前端，观察 readiness/错误/延迟后放量。

停止条件：迁移 dirty/失败、readiness 非 200、认证 E2E 失败、5xx/延迟明显回归、Secret 或高危可修复漏洞。停止后保留现场证据但不保留凭据。

回滚：优先回滚应用到与当前数据库兼容的不可变镜像；前端回滚到支持当前 contract id/digest 的静态制品。生产数据库不默认执行 down；破坏性变化使用前向修复。恢复后核对 `/livez`、`/readyz`、`/version`、集成与 E2E。

依赖升级：Dependabot 每周仅创建 Go module、GitHub Actions 和 Docker PR，无写 main 或自动合并权限。工具版本仍由 Makefile 唯一拥有；Action PR 必须重新固定 commit SHA，容器必须固定 digest。主版本、认证、PostgreSQL、OpenAPI 生成器和构建链升级需 ADR/迁移说明，并审查 release notes、许可证、漏洞、生成漂移和性能。无人处理或连续失败两周时由 Owner 暂停该组并建立有期限的处置记录，不能静默忽略。

当前限制：默认分支保护、完整 `security-check`、生产遥测/SLO、跨仓发布和备份恢复尚未演练；因此本文是可执行流程基线，不是生产就绪证明。
