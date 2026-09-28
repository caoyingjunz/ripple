# Ripple

QQ 类桌面 IM 客户端：Electron 壳 + React 前端 + Go 后端 + MySQL（漂流瓶玩法保留）。

## 产品功能架构

```
┌ 桌面壳 electron/ ─ 托盘/原生通知/未读角标/--profile 同机多开/WS 下沉主进程常驻
│                    └ 以 ripple://local/ 协议加载 web/dist 产物
├ 前端 web/（React 18 + Vite + Zustand，~12 页）
│     登录/注册 · 好友（申请/同意/黑名单）· 会话列表（未读/置顶/免打扰）
│     单聊+群聊（图片/撤回/已读/历史翻页/乐观发送）· 群管理 · 资料编辑 · 漂流瓶
│     WS App 级单例：指数退避重连 + 断线 after_seq 补拉 + message.id 去重
└ 后端 server/（Go + gorilla/mux + WebSocket + MySQL）
      统一消息模型：messages 单表 + 会话内 seq（LAST_INSERT_ID 原子取号）
      会话三型 single|group|bottle + conversation_members 多对多
      可靠投递：落库为唯一真相源，推送只是提示，断线/离线均可补拉
```

## 快速开始

### 0. 数据库（MySQL，与 rainbow 共享实例）

```bash
export RIPPLE_MYSQL_DSN='root:<密码>@tcp(peng:3306)/ripple?charset=utf8mb4&multiStatements=true'
```

启动时自动建表；users 表为空时 seed 演示账号。**该实例为共享实例，只允许使用 `ripple` / `ripple_test` 两个库**；连接信息只走环境变量（凭据参考 rainbow 侧配置，勿写入仓库）。

### 1. 后端

```bash
cd server
go run ./cmd/server        # 默认 :8080；未设置 DSN 时启动即报错
```

### 2. Web 模式

```bash
cd web
npm install
npm run dev                # http://localhost:5173（vite proxy → :8080）
```

### 3. 桌面模式（根目录）

```bash
npm install
npm run dev                # 需先起 web dev；RIPPLE_DEV=1 加载 http://localhost:5173
npm start                  # 加载打包产物 web/dist（先 npm run build:web）
npm run dist:mac           # → dist/ripple-x.x.x-universal.dmg
npm run dist:win           # → dist/ripple Setup x.x.x.exe（NSIS x64）
```

免签名口径：mac 首次打开需「右键 → 打开」；Windows SmartScreen 点「仍要运行」。详见 `docs/desktop-usage.md`。

**同机多开**：`npx electron . --profile=alice` 或 `RIPPLE_PROFILE=bob npx electron .`——每 profile 独立 userData 与登录态，可同时运行互测。

## 演示账号

| 用户名 | 密码 |
|--------|------|
| alice / bob / carol / dave / erin | `demo123` |

（仅 users 表为空时 seed；客户端支持自助注册）

## 功能

- 自助注册登录（JWT 24h）、用户搜索（含 relation 状态）、资料/头像编辑
- 好友：申请（反向 pending 秒通过）/同意/拒绝/删除/拉黑/解除；黑名单后互发拦截（403）
- 单聊 + 群聊：文字/图片；群成员管理（建群/邀请/移出/退群/改名，owner 权限）
- 消息可靠性：会话内 seq 单调递增、断线重连 `after_seq` 增量补拉、按 `message.id` 去重、`client_msg_id` 发送确认（乐观 UI）
- 未读数 / 已读上报（任一端已读即会话已读）/ 2 分钟内撤回 / 置顶 / 免打扰 / 在线状态 presence
- 漂流瓶：扔/捡/匿名 6 轮（别名投影，对外不暴露 sender_id）/ reveal 转私聊；消息统一进 messages 表

## 验证

```bash
# 后端全量测试（e2e：注册→好友→群聊→增量→已读→撤回→黑名单→漂流瓶→WS）
cd server
RIPPLE_TEST_MYSQL_DSN='root:<密码>@tcp(peng:3306)/ripple_test?charset=utf8mb4&multiStatements=true' go test ./...

# 实机冒烟（39 项断言；先按「快速开始」起服务，建议 DSN 指向 ripple_test）
node scripts/smoke.mjs

# 桌面冒烟（窗口真实加载即退出 0）
npx electron . --smoke
```

## 文档

- `docs/design.md` — 桌面化改造方案（背景、分期 P0-P5、已拍板决策记录）
- `docs/implementation-spec.md` — 实施契约（schema / REST / WS 帧的唯一标准）
- `docs/desktop-usage.md` — 桌面端开发/打包/多开/免签名使用说明
