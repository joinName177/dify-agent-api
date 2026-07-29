# 需求与风险矩阵

Owner 默认是当前唯一维护者 `@joinName177`。自动验证列只引用已经存在的代码或命令；依赖 Docker/PostgreSQL 的命令须在 CI 或本地 daemon 可用时执行。

| 需求/风险 | 控制 | 自动验证 | 人工验证 | 残余风险 | Owner |
| --- | --- | --- | --- | --- | --- |
| 密码泄漏或弱存储 | Argon2id 随机盐、明文不持久化、参数封闭校验 | `make test`、`make gosec`、`make secret-check` | 生产 Secret 注入与日志平台抽查 | 密码重置、MFA、密钥轮换未实现 | @joinName177 |
| 管理员初始化冲突 | 邮箱锁与 fail-closed 角色检查 | `make integration-test`、`make e2e-test` | 首次发布核对唯一管理员身份 | 单一 bootstrap 凭据轮换流程未建立 | @joinName177 |
| token 泄漏/replay | 随机不透明 token、DB 仅摘要、refresh 原子轮换、HttpOnly Cookie | `make integration-test`、`make e2e-test` | 浏览器 Cookie/缓存策略审查 | 无全会话管理和主动清理任务 | @joinName177 |
| CORS/跨站请求 | 启动时 HTTPS origin 校验、精确 allowlist、状态请求再次校验 origin | `make config-check`、`make http-contract-test` | 部署配置对照真实前端域名 | 代理层策略不在仓库控制内 | @joinName177 |
| 滥用和资源耗尽 | body limit、超时、连接池上限、进程级限流 | `make http-contract-test`、`make config-check` | 容量测试、网关级分布式限流 | 当前限流器是单实例内存态，尚无容量基线 | @joinName177 |
| PII 外泄 | 安全错误信封、日志不记录 body/token、数据分类 | `make http-contract-test`、`make test` | 观测平台字段与保留策略审查 | 邮箱删除/保留和审计制度未实现 | @joinName177 |
| 迁移篡改或失败 | append-only manifest、真实 up/down/up、启动版本检查 | `make migration-check` | 发布前锁/耗时评估；不可逆变更审批 | 备份恢复演练未完成 | @joinName177 |
| OpenAPI/实现漂移 | digest、生成漂移、真实响应 Schema 验证 | `make generated-check`、`make http-contract-test` | 前端兼容发布演练 | 跨仓演练尚未执行 | @joinName177 |
| PostgreSQL 不可用/并发语义错误 | readiness、显式事务、真实约束和并发测试 | `make integration-test`、`make e2e-test` | 故障注入与恢复演练 | 无生产 HA、容量和 RTO/RPO 证据 | @joinName177 |
| 供应链/Secret | 精确版本或摘要、Govulncheck、Gosec、Gitleaks、Grype 子目标 | `make toolchain-check`、`make source-security-check`、`make container-security-check` | 许可证 Owner 审批、无修复漏洞处置 | 许可证 allowlist 未批准，完整 security-check 未启用 | @joinName177 |
| 所有权与审批绕过 | CODEOWNERS 路由、目标 Required Check 列表 | `make governance-check`、`make toolchain-check` | GitHub branch protection/ruleset 实查 | 当前只有一个维护者且外部保护未启用，无法职责分离 | @joinName177 |
