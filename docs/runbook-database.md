# PostgreSQL 故障 Runbook

Owner：`@joinName177`。

触发信号：`/readyz` 返回 `DATABASE_UNAVAILABLE`、连接池等待/错误增加、事务超时。先确认 `/livez` 是否仍正常，再从部署平台检查 PostgreSQL 健康、连接数、存储和网络；使用 Secret 引用名而非打印完整 URL。

禁止：在生产运行 `DROP`/`TRUNCATE`、手工编辑 `schema_migrations`、无限重试、临时放大连接池掩盖饱和、把生产快照复制到非生产。

缓解：停止放量和写流量；若数据库平台故障，按平台切换/恢复流程处理；若新版本连接行为异常，回滚应用但保持数据库向后兼容。仓库当前没有经过验证的备份恢复、HA 或 RTO/RPO 流程，因此需要数据库平台 Owner 参与，不能声称自动恢复。

恢复验证：数据库 ping、clean migration version 2、`/readyz` 200；在隔离环境执行 `make integration-test` 与 `make e2e-test`；确认无重复管理员、session replay 或未提交事务。
