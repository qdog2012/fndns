# DNS 管理器

面向飞牛 OS（FNOS）的原生 `.fpk` 应用，用一套中文 WebUI 管理 Cloudflare 与腾讯云 DNSPod 的域名和解析记录。

## 首版能力

- 不限数量的 API 凭据，自定义名称并按创建时间确定同名域名优先级
- Cloudflare API Token、腾讯云 `SecretId + SecretKey`
- AES-256-GCM 本地凭据加密，主密钥与数据库仅存于 FNOS 应用数据目录
- 所有凭据的域名统一列表；同名域名只展示最早凭据中的实例
- 记录查看、新增、编辑、删除，以及单域名内批量删除、批量启用/暂停
- `A / AAAA / CNAME / MX / TXT / NS / SRV / CAA` 及平台扩展类型
- Cloudflare 橙云代理、自动 TTL；DNSPod 解析线路、TTL、优先级、权重和启停
- 三级手动同步：全部凭据、单个凭据、单个域名
- 不自动刷新。缓存超过 6 个月后保留为只读，并明确标记过期
- 修改前校验远端记录；远端状态与缓存不一致时阻止覆盖
- 操作日志保留 6 个月，可按日期、凭据、域名、动作和结果筛选
- 简体中文、FNOS 风格桌面界面、手机浏览器适配

> Cloudflare 没有暂停单条 DNS 记录的 API。应用会明确显示“不支持”，不会通过删除和重建模拟暂停。

## 项目结构

```text
cmd/fndns/             Go 服务入口与内嵌 WebUI
internal/provider/     Cloudflare、腾讯云 DNSPod API 适配器
internal/store/        SQLite 缓存与六个月审计日志
internal/secretbox/    AES-256-GCM 凭据加密
internal/service/      同步、冲突校验、批量操作
internal/httpapi/      本地 REST API 与静态资源服务
webui/                 React + TypeScript 管理界面
packaging/fnos/        manifest、生命周期、向导和桌面入口
scripts/               双架构构建与 .fpk 组装
```

## 构建

需要 Go 1.24+、Node.js 20+、npm、Python 3 和 Pillow。

Windows：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

Linux / macOS：

```bash
./scripts/build.sh
```

构建产物：

```text
dist/com.fndns.manager_1.0.11_x86.fpk
dist/com.fndns.manager_1.0.11_arm.fpk
```

详细安装步骤见 [docs/FNOS_INSTALL.md](docs/FNOS_INSTALL.md)，API 权限建议见 [docs/API_CREDENTIALS.md](docs/API_CREDENTIALS.md)。

## 本地开发

先构建前端并启动 Go 服务：

```powershell
cd webui
npm.cmd install
npm.cmd run build
cd ..
go run ./cmd/fndns --listen 127.0.0.1:18788 --data-dir .\testdata\runtime
```

需要预览完整界面时可加入 `--demo`。演示数据只写入指定的本地数据目录，不会请求真实 DNS 平台。

前端热更新：

```powershell
cd webui
npm.cmd run dev
```

Vite 会把 `/api` 代理到 `127.0.0.1:18788`。

## 测试

```powershell
go test ./...
cd webui
npm.cmd run typecheck
npm.cmd run build
```

真实平台的只读连通性测试会从本机环境变量读取密钥；缺少变量时自动跳过：

```powershell
$env:FNDNS_CLOUDFLARE_TOKEN = "..."
$env:FNDNS_TENCENT_SECRET_ID = "..."
$env:FNDNS_TENCENT_SECRET_KEY = "..."
go test ./integration -v
```

不要把真实密钥写入仓库、命令历史、聊天或截图。

## 安全边界

- FNOS 桌面入口设置 `allUsers: false`，仅管理员可见。
- 正式安装后由 FNOS WebUI 以同站 iframe 打开应用；应用只为该内嵌请求建立浏览器会话，不需要二次登录。
- 服务以 FNOS 专用低权限用户运行，不需要 root，也不申请用户共享目录权限。
- API 凭据不会返回前端，日志中不保存凭据，TXT 等记录值不进入审计详情。
- 修改请求校验浏览器 `Origin`，并设置严格的内容安全策略。
- 正式 FPK 使用当前稳定 FNOS 支持的 iframe 端口入口。浏览器直接访问 `18788` 会返回 404；只有同站 FNOS WebUI iframe 能建立会话，跨站 iframe、无会话资源请求和 API 请求同样被拒绝。
