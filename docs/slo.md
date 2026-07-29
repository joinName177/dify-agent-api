# SLO 可测量性状态

Owner：`@joinName177`。本文件区分“目标”和“已经可计算的 SLO”。当前代码只导出 HTTP trace，并有结构化访问日志；没有 metric exporter、生产遥测后端、仪表盘或告警配置。因此下列目标全部是待实施候选，当前不得用于宣称服务已满足 SLO。

| 候选 SLI/SLO | 窗口与排除 | 所需数据源 | 建议告警 | 当前状态 |
| --- | --- | --- | --- | --- |
| API 成功可用性 ≥ 99.9% | 28 天；排除 `/livez`、客户端主动取消和批准维护窗 | 按 route/status 的服务端请求计数 | 1h/6h 多窗口错误预算消耗 | 未实现 metric 与生产后端 |
| 认证 p95 < 300ms、p99 < 800ms | 5 分钟和 28 天；按 register/login/refresh 分开 | 服务端直方图，不记录邮箱/token | p99 连续 15 分钟超阈值 | 未实现 metric |
| 5xx 比例 < 0.5% | 5 分钟；按稳定 errorCode 聚合 | HTTP 状态/errorCode 计数 | 5 分钟 >2% 或预算快速消耗 | 仅日志可临时查询，不足以证明 SLO |
| DB 连接池饱和 < 80% | 5 分钟；排除维护窗 | pgx acquired/max、等待时长 | 连续 10 分钟 ≥80% | 未实现 metric |
| 发布失败率 < 10% | 滚动 20 次发布 | 部署平台 release id、回滚和 E2E 结果 | 任一自动回滚立即告警 | 部署平台未接入 |

实施门槛：新增 OpenTelemetry metric provider/exporter；稳定 route/error 字典；脱敏测试；生产 backend、dashboard 和告警 Owner；至少一个完整窗口的数据质量核验。在这些条件全部满足前，OPS-702 保持未完成。trace 和日志只使用 request/trace id、route、status、duration 与安全错误码，禁止密码、Authorization、Cookie、数据库 URL和完整邮箱。
