# Go 工程代码与质量规范

## 1. 事实源

本文件定义 Go 代码、配置、静态检查、测试和工具规则。服务端范围、模块和依赖方向以 `docs/go-backend-architecture.md` 为准；可由工具表达的规则必须以可执行配置和测试为最终依据。

## 2. Go 代码

- 代码必须通过仓库锁定工具链的 `gofmt`、`go vet`、Staticcheck 和测试。
- 包、文件、类型和函数只承担一个可清晰描述的职责；按业务能力和架构边界拆分，不为满足指标创建无语义碎片。
- 禁止 `utils`、`common`、`manager`、`helpers` 等无法表达责任边界的泛化 package。
- 名称表达业务角色、单位和生命周期；集合使用复数，时间和容量值在名称中标明单位。
- 优先使用早返回和具名函数降低嵌套。复杂分支应提取为可测试策略，不压缩为难以调试的一行表达式。
- 生产请求路径禁止使用 `panic`、`log.Fatal` 或 `os.Exit`；进程入口只在无法安全启动时返回明确错误并退出。
- 不因方便增加全局可变状态、Service Locator、隐式 registry 或隐藏反向依赖的 callback。
- 包依赖必须满足 `docs/go-backend-architecture.md` 的可执行矩阵并通过 `make architecture-check`；禁止通过类型别名、回调注册或全局变量隐藏反向依赖。
- 配置键、必填项和默认值必须同时满足 `deploy/config.schema.json` 与运行时代码，并通过 `make config-check`；该检查由 `architecture-check` 聚合进入 CI 和 `verify`。

## 3. Context、并发与资源

- 所有 I/O 接收并传播 `context.Context`，不得用 `context.Background()` 截断请求取消链。
- HTTP、数据库和外部调用明确超时、取消、有限重试、幂等性和资源释放。
- Goroutine 必须有所有者、退出条件和测试；禁止无界创建、无人读取的 channel 和泄漏 ticker。
- 共享状态必须通过所有权、锁或不可变数据明确保护，并通过 race test 验证。
- 事务只覆盖必要数据库操作，不包含远程 I/O、长计算或无界等待。

## 4. 错误与安全

- 错误保留稳定分类和因果链；使用 `%w` 包装可处理的底层原因。
- HTTP 错误通过统一映射返回稳定 `errorCode` 和安全消息，不返回 SQL、堆栈、内部 URL、下游原文或 Secret。
- 密码、token、Authorization、Cookie、数据库凭据和完整 PII 不得进入日志、trace、metric、错误、fixture 或快照。
- 所有外部输入在边界完成结构和业务校验；不得以“理论上不会发生”省略负向路径。
- 认证和权限只信任服务端验证后的 Identity；不得信任请求中的角色、租户或 capability。

## 5. 配置与常量

- 配置只从部署注入读取，解析为不可变结构并在启动前 fail-closed。
- `deploy/config.schema.json`、Go Config 和环境变量读取清单必须保持一致。
- 配置契约检查必须解析 Go 语法结构，不得依赖源码排版或正则猜测配置读取行为。
- Secret 不得进入源码、默认值、`.env.example`、OpenAPI、容器层或前端 Runtime Config。
- 状态、错误码、权限、路由、配置键、超时、容量和协议字段由唯一 owner 定义，不在多个包复制。

## 6. HTTP、OpenAPI 和数据库

- OpenAPI 是请求、响应、认证和错误信封的唯一 transport 契约；Go 类型和 server 接口由锁定生成器产生。
- Handler 只解析请求、读取已验证身份、调用 Application 并映射响应；不得直接执行 SQL 或定义业务状态机。
- Repository 使用参数化 SQL和显式事务；生产与集成测试只以 PostgreSQL 语义为准。
- 生产 PostgreSQL URL 必须提供非空用户名和密码并使用 `sslmode=verify-full`；仅建立加密但不验证证书或主机名不满足生产门槛。
- refresh Cookie 在生产和未知策略下必须默认启用 `Secure`；仅 development/test 可通过显式策略允许本机 HTTP。
- `migrations/manifest.json` 是已发布迁移的追加式摘要基线。已发布迁移不可修改或重排；新增迁移必须使用连续六位版本、明确 `reversible`，并通过 `make migration-manifest-check`。不可逆变化使用前向修复，不为通过测试伪造危险 down migration。
- 应用期望迁移版本由 manifest 生成到 `generated/migrations/version.gen.go`，不得在运行时代码中手工复制版本号。
- 生成文件只位于 `generated/`，带禁止手改标识并由零漂移检查治理。

## 7. 测试

- 测试公开行为和稳定契约，不依赖私有实现、执行顺序、真实时间或公网网络。
- 每项高风险能力覆盖成功、失败、边界、权限、并发、取消、清理和恢复。
- 修复缺陷必须有能在旧实现稳定失败的回归测试。
- 禁止长期 `t.Skip`、无界 retry、空断言、无主 baseline 和扩大排除范围来提高覆盖率数字。
- 数据库约束、事务、锁、迁移和并发必须使用真实 PostgreSQL 集成测试，不以 mock 替代。
- API E2E 必须启动真实 `cmd/api` 进程、轮询 readiness，并在成功和失败路径清理进程及临时文件；不得用进程内 Router 冒充启动链路验证。
- 生产行为变更先运行受影响测试，再运行权威聚合门禁；race test 是共享状态变更的必经检查。

## 8. 依赖与生成工具

- 新依赖必须解决已确认问题；标准库或现有依赖足够时不新增包装库。
- 依赖、生成器、分析器、基础镜像和 Action 使用精确版本或不可变摘要。
- 升级必须审查官方变更、许可证、漏洞、生成漂移、测试和运行影响。
- 删除功能时同步删除依赖、配置、类型、测试、文档和 CI 分支。
- 不使用长期 fork、修改依赖源码或永久 replace 掩盖上游问题。

## 9. 完成门禁

检查必须与风险相称，并以仓库实际存在的非修改型命令为准。文档或治理变更至少执行治理检查和 `git diff --check`；生产代码变更必须执行受影响测试和完整 `make verify`。任何 warning、skip、flaky、未运行项或外部阻塞都必须明确记录，不能把部分验证描述为完成。
