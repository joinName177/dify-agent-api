# ADR 0001：本地凭据与服务端会话

- 状态：已接受
- 日期：2026-07-24

## 背景

首个真实业务场景要求用户注册、登录和内置超级管理员。当前没有可接入的企业身份提供方，因此原架构中的 OIDC-only 假设无法交付这一能力。

## 决策

采用邮箱与密码认证。密码使用 Argon2id（19 MiB、2 次迭代、并行度 1）和每条记录独立随机盐存储。登录成功后签发短期随机 access token 与长期随机 refresh token；数据库只保存 token 摘要。access token 仅保存在 SPA 内存，refresh token 使用 HttpOnly、SameSite=Lax Cookie，并在生产开启 Secure。刷新操作在数据库事务内轮换 token，旧 refresh token 立即失效。

超级管理员凭据由部署平台 Secret 注入。服务启动持有 PostgreSQL advisory lock 并幂等创建管理员；已存在的普通用户邮箱不会被自动提升。

## 后果

- 服务必须保护凭据端点免受暴力尝试，并监控认证失败与限流拒绝。
- 密码重置、邮箱验证、MFA 与全会话管理尚未包含在首个切片中，启用公网注册前必须按风险计划补齐。
- 将来接入 OIDC 时保留服务端 `Identity` 边界，通过新 ADR 迁移，不让业务 handler 解析身份协议细节。
