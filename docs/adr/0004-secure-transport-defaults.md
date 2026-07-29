# ADR 0004：生产传输验证与 Cookie 安全默认值

## 状态

已接受。

## 日期

2026-07-29

## 背景

原配置只拒绝生产 PostgreSQL 的 `sslmode=disable`，因此 `require` 虽然加密连接，却不验证证书信任和数据库主机名。refresh Cookie 则通过布尔参数决定 `Secure`，其零值会静默降级为不安全行为。两者都允许一次遗漏配置或调用参数绕过预期的生产安全边界。

## 决策

- 生产 `DIFY_AGENT_DATABASE_URL` 必须包含非空用户名、密码和 `sslmode=verify-full`。缺失参数以及 `disable`、`require`、`verify-ca` 均在启动前 fail-closed。
- development/test 仍可为隔离的本机 PostgreSQL 使用较低 TLS 模式；该例外不得进入生产。
- refresh Cookie 使用显式 `CookiePolicy`。零值、未知值和 staging/production 均启用 `Secure`；只有 development/test 可显式选择本机 HTTP 策略。
- 配置错误不得回显数据库 URL 或凭据。正向、负向、静态安全和进程级 E2E 测试共同验证该决策。

## 备选方案

- 生产允许 `sslmode=require`：只有加密，没有服务端身份验证，无法防止错误信任或中间人，拒绝采用。
- 保留布尔 Cookie 参数并依赖调用方计算：零值不是安全默认值，新增调用点容易误用，拒绝采用。
- 所有环境都强制 TLS/Secure Cookie：会破坏当前隔离 localhost 开发和 HTTP E2E；在具备本地证书自动化前不采用。

## 后果

- 仍使用 `sslmode=require` 或没有可信 CA/正确主机名的生产部署将无法启动，必须先完成数据库证书和连接配置迁移。
- 本机 HTTP 只能通过明确的 development/test 环境启用，新增环境默认保持安全。
- 该决策不解决数据库凭据轮换、客户端证书认证、全会话管理或边缘 TLS 配置；这些能力需要独立需求和运维方案。
