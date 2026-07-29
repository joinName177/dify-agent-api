# 迁移故障 Runbook

Owner：`@joinName177`。

触发信号：迁移 Job 失败、`schema_migrations.dirty=true`、应用 readiness 报 `MIGRATIONS_NOT_READY`。发布前必须通过 `make migration-manifest-check` 和隔离 PostgreSQL 的 `make migration-check`。

诊断：记录迁移版本、dirty 标志和安全化 PostgreSQL 错误；对照 `migrations/manifest.json`；确认失败发生在 expand/backfill/cutover/contract 的哪一步以及是否持锁。不要输出数据库 URL 或数据行。

禁止：修改已发布 SQL/manifest 摘要、手工把 dirty 改为 false、默认在生产执行 down、在没有备份/锁评估时重跑破坏性 DDL。

恢复：尚未发布且数据库无部分变化时修复为新迁移；已产生部分变化时由数据库 Owner 审查并发布前向修复。只有 manifest 明确 `reversible=true`、数据不会丢失且演练通过时才考虑受控 down。恢复后确认 clean 期望版本、关键约束/索引/外键以及认证 E2E。
