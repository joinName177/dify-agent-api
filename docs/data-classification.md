# 数据分类与处理规则

Owner：`@joinName177`。分类适用于 API、日志、trace、数据库、测试、CI 制品与支持材料；新增字段必须在 OpenAPI/迁移评审时确定分类。

| 等级 | 本服务数据 | 允许位置 | 禁止事项 | 保留/删除 |
| --- | --- | --- | --- | --- |
| Restricted | 明文密码、access/refresh token、Cookie、数据库和管理员凭据 | 仅请求内存或受管 Secret 注入；数据库只保存 Argon2id/SHA-256 派生值 | 日志、trace、metric、错误、fixture、快照、前端持久化、Git | 明文请求结束即释放；token 摘要随会话生命周期清理 |
| Confidential | 邮箱、显示名、用户 UUID、认证失败事件、IP/UA（未来若采集） | PostgreSQL 最小字段、授权 API 响应、受控运维系统 | 公共日志、未脱敏告警、跨环境复制 | 当前用户/会话表尚无自动保留任务；上线前必须确定删除与备份策略 |
| Internal | 内部错误、连接池和迁移状态、request/trace id、构建 commit | 结构化日志、trace、metric、受控 CI | 向客户端返回内部错误或数据库 URL | 由观测平台策略控制；仓库当前不能证明生产保留期限 |
| Public | OpenAPI、contract id、release id、非敏感版本信息、健康状态 | API、文档、制品元数据 | 混入环境变量或 Secret | 随发布记录保留 |

控制事实：密码哈希位于 `internal/modules/user/infrastructure/credentials/password.go`；token 只存摘要的写入位于 `internal/modules/user/infrastructure/postgres/repository.go`；HTTP 安全错误映射位于 `internal/modules/user/transport/http/handler.go`；`make secret-check` 扫描 Git 历史。当前缺口是数据删除、备份保留、密钥轮换和审计事件存储，它们在风险矩阵中保持开放，不能由本分类文档视为已解决。
