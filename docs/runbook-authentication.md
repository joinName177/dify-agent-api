# 认证故障 Runbook

Owner：`@joinName177`。

触发信号：登录/refresh 401 或 5xx 激增、refresh replay、管理员无法登录、注册冲突异常。先按 release id、route、status、errorCode 和 request id 聚合；只使用 token 的不可逆事件标识，不复制 Authorization、Cookie、密码或完整邮箱。

诊断顺序：确认 `/livez`、`/readyz`、`/version`；核对近期 API/迁移/配置发布；检查数据库可用性和 `schema_migrations` 是否为 clean version 2；使用隔离测试账号执行 `make e2e-test`。若仅错误凭据增加，检查客户端/攻击流量，不重置真实用户密码作为探针。

禁止：输出 token/密码、直接修改 session/user 表、把普通用户改成管理员、关闭 origin/auth/限流检查、复用生产数据做测试。

缓解与恢复：停止新版本放量；应用可回滚到数据库兼容版本；疑似 token/管理员 Secret 泄漏时先撤销/轮换再恢复；数据库变化遵循发布回滚文档。恢复后执行认证 E2E，确认旧 refresh replay 和 logout 后 access 均为 401，并记录影响窗口与后续控制。
