# Ripple 桌面化改造方案（QQ 类 IM 客户端）

> 版本：v1.1 ｜ 日期：2026-09-27 ｜ 性质：评估与方案文档，不含代码实现
> 现状基线：`b3965e0 Init the project`（Web 版「漂流瓶 + 1v1 聊天」demo，Go + SQLite + React 18/Vite）

---

## 1. 背景与目标

- **目标**：把 ripple 从 Web 版 demo 改造成可安装的桌面 IM 客户端（QQ 形态）：多用户自助注册登录（支持同机多开）、加好友、建群与群内联通、发消息
- **输入**：现有 `/Users/caoyuan05/cloud/ripple` 前后端代码
- **成功标准**：可安装客户端（macOS + Windows）；**≥3 个用户**各自注册登录，完成「加好友 → 建群 → 群内互发 → 单聊」全链路；消息不丢（断线/离线可补拉）；同机可多开不同账号互测；可回滚

## 2. 一句话结论

**这不是"给 demo 套个壳"，而是在 demo 骨架上重做一个 IM 的最小可用版本。**
桌面壳本身工作量很小；现状约 80% 的 QQ 化能力为零，真正的工作量集中在 4 处数据模型/推送架构重构，而非补接口。

---

## 3. 现状审计（差距盘点）

### 3.1 现状能力

- 后端 13 个业务端点（登录/会话/消息/上传/漂流瓶），WS 事件 `chat.message` / `bottle.message` / `bottle.revealed`
- JWT(24h) + bcrypt 鉴权；图片上传（仅图片、2MB、内容嗅探）
- 漂流瓶玩法完整：扔瓶 → 随机捡瓶 → 匿名 6 轮 → reveal 转私聊 / close

### 3.2 对照 QQ 类 IM 的缺失能力矩阵

| 能力 | 现状 | 依据 |
|---|---|---|
| 加好友/好友关系/申请同意 | **完全没有**（无 friends 表，会话仅由漂流瓶 reveal 产生） | `db/db.go` conversations 仅二元 |
| 用户搜索/注册 | **完全没有**（只有 seed 演示账号，无注册接口） | `api/router.go` |
| 群聊（建群/成员/群消息） | **完全没有**（conversations 硬编码 user_a/user_b 二元） | `db.go` UNIQUE(a,b) |
| 离线消息可靠性 | 部分有（消息落库可拉，但无 seq/ACK/未读，且有 200 条缺口，见 3.4） | `chat.go` |
| 多端同步 | 部分有（hub 支持同用户多连接，无同步语义） | `ws/hub.go` |
| 在线状态/心跳超时 | 完全没有（pong 非 presence） | `hub.go` |
| 未读计数/已读回执 | 完全没有 | — |
| 撤回/删除/置顶/免打扰 | 完全没有 | — |
| 资料编辑/头像上传 | 完全没有（avatar_color 固定色） | `models.go` |
| 图片传输 | **已有** | `router.go` upload |
| 系统通知 | 部分有（仅 reveal） | — |

### 3.3 可复用资产（无需重写）

JWT+bcrypt 鉴权、users 表、消息落库事务、图片上传与静态服务、gorilla/mux+websocket 骨架、`api/client.ts` 请求封装、AuthContext、ChatRoomPage 气泡 UI、styles.css。
前端与浏览器耦合仅 4 处（localStorage token、location 推导 WS 地址、相对路径 fetch、BrowserRouter），桌面壳复用成本低。

### 3.4 四处"硬骨头"（必须动模型/架构）

1. **单连接数据库**（原 `db.go` `SetMaxOpenConns(1)`）→ IM 并发写必然瓶颈；**已决策切 MySQL 解决（决策②）**
2. **1v1 会话模型**（conversations 二元 + PeerID 推导）→ 群聊要求重构为 members 多对多，是全表重构
3. **三套消息模型分叉**（messages vs bottle_messages，chat.message vs bottle.message）→ 每加一个能力要写三份，必须统一
4. **无投递可靠性层**：SendToUser fire-and-forget、写失败静默丢；且拉取为 `ORDER BY created_at ASC LIMIT 200`——**超 200 条的会话永远取不到最新消息（隐藏缺口）**

---

## 4. 桌面壳选型：Electron（已拍板）

### 4.1 对比

| 维度 | Electron（44.x 稳定） | Tauri 2（2.12） |
|---|---|---|
| 安装包 / 装后体积 | 60-100MB / 200-400MB | 3-10MB / 10-20MB |
| 空载内存 | ~100-250MB | ~40-80MB |
| React18+Vite 复用 | 产物直接进壳 | 同样可以 |
| 托盘/角标/自启/单实例/深链/更新 | 全部成熟 API | 官方插件基本齐，事件联动需写 Rust |
| 后台长连接保活（IM 关键） | backgroundThrottling:false、powerSaveBlocker，文档化 | WebView2 隐藏即 suspend；macOS App Nap 无公开关闭 API，WS 需下沉 Rust 自研 |

**选 Electron 理由**：现有 React 产物直接复用；团队已有 Electron 项目经验；本项目核心风险是「长期后台常驻长连接」，Electron 控制手段成熟。牺牲的是 Tauri 的体积/内存优势（小 5-10 倍）——除非"小安装包"本身是产品卖点，否则不值。

### 4.2 发布链路（内部版本口径，见 §9 决策①）

- **不做**代码签名与公证 → mac 首次打开需「右键 → 打开」绕过 Gatekeeper；Windows 触发 SmartScreen 点「仍要运行」
- 将来对外分发时的费用参考：Apple Developer $99/年（公证含 DMG 单独装订）；Windows 证书 2023 起强制硬件密钥、2026-03 起有效期 15 个月/年续（Azure Trusted Signing ~$120/年 或传统证书 $200-700/年）
- electron-builder 配置预留签名环境变量，届时只补证书不改链路

---

## 5. 目标架构

### 5.1 数据模型（从 1v1 到 IM）

| 表 | 变更 | 说明 |
|---|---|---|
| `users` | 加 avatar_url / signature / status | 真实头像（替换固定色）、资料编辑 |
| `friendships` | **新建** | (user_id, friend_id, status)，status=pending/accepted/blocked 统一承载申请+同意+黑名单 |
| `conversations` | **重构** | 去掉 a/b 二元字段 → (id, type: single\|group, name, owner_id, created_at) |
| `conversation_members` | **新建** | (conversation_id, user_id, role, alias, joined_at, last_read_seq)；群成员与私聊统一 |
| `messages` | **重构** | 统一消息表：加 seq（会话内单调递增）、revoked_at；漂流瓶消息并入 |
| `bottle_*` | **保留但降级** | 只存玩法元数据；消息统一进 messages，匿名别名放 conversation_members.alias |

关键点：**三套消息模型合一**（匿名性靠"对外投影隐藏真实身份"实现，不靠独立消息表）。

### 5.2 消息可靠性（当前最大隐患）

1. 服务端：消息先落库拿 seq，再推 WS；推送只是"有新消息"提示，真相源永远是库
2. 客户端：每会话维护 last_seq，登录/重连带 last_seq 增量拉取（`GET /messages?after_seq=&limit=`）
3. 去重：客户端按 message.id 去重，解决推送与补拉重叠
4. 同步修复 200 条 ASC 缺口 → **离线消息由此天然成立**，不需要额外离线队列

### 5.3 在线状态与多端

- 服务端心跳超时判定（现状 ping 无超时）+ presence 广播
- hub 已支持同用户多连接，补"每端独立 last_seq"
- 单机阶段不做 Redis；横向扩展留到有真实压力（避免过早设计，触发条件见 §5.6）

### 5.4 前端

- WS 从"每页一条、无重连"改为 **App 级全局单例** + 指数退避重连 + 断线补拉
- 引入 Zustand（或同类）管理会话列表/未读数/好友列表
- 收敛 4 处浏览器耦合点 → 桌面壳下用环境注入的 base URL

### 5.5 接口与 WS 协议草案（实施时细化）

```
新增/重构 REST：
POST   /api/auth/register            注册（现状没有）
GET    /api/users/search?q=          用户搜索
POST   /api/friends/requests         发好友申请
POST   /api/friends/requests/{id}/accept | /decline
GET    /api/friends                  好友列表（含黑名单分组）
PUT    /api/me                       资料编辑
POST   /api/me/avatar                头像上传
POST   /api/conversations            建群
POST   /api/conversations/{id}/members | DELETE .../members/{uid}
GET    /api/conversations/{id}/messages?after_seq=&limit=   增量拉取
POST   /api/conversations/{id}/read  已读上报
POST   /api/messages/{id}/revoke     撤回

WS 事件统一：
C→S  message.send {client_msg_id, conversation_id, type, body}
S→C  message.ack   {client_msg_id, id, seq}          （发送确认）
S→C  message.new   {message}                          （新消息提示）
S→C  message.revoked / message.read / presence.change / friend.request / conversation.updated
```

### 5.6 消息队列：现阶段不引入

可靠投递已由「MySQL 落库为唯一真相源 + 会话内 seq + 客户端 last_seq 增量补拉 + 按 message.id 去重」保证，MQ 无必须承担的职责：

| 常说的 MQ 用途 | 是否需要 | 原因 |
|---|---|---|
| 消息不丢 | 否 | 已由落库 + seq 补拉保证，离线消息天然成立 |
| 投递重试 | 否 | 客户端重连补拉已覆盖；队列重投反而引入重复与乱序 |
| 削峰填谷 | 否 | IM 为单条小写入，MySQL 可承载；当前无突发大流量 |
| 跨节点广播 | 暂不需要 | 当前单后端实例 |
| 离线推送 / 异步任务 | 暂不需要 | 无 APNs/FCM、无转码需求 |

现在引入 MQ 是净负债：多一个中间件与运维成本，且需额外解决幂等与顺序问题（而 IM 最需要顺序）。

**触发条件（出现再上，不提前）**：后端多实例水平扩展 → 上 **Redis Pub/Sub**（非 Kafka）；需离线推送/邮件通知 → 队列化；千人大群广播、图片转码 → 队列；消息全文检索/审计事件流 → 才轮到 Kafka 类。
**推荐演进路线**：Redis 优先——一份中间件同时解决「跨节点广播 + presence 在线状态 + 多端路由」，比 Kafka/RabbitMQ 更贴合 IM 下一步。

---

## 6. 分期实施计划

| 阶段 | 内容 | 规模 | 依赖 |
|---|---|---|---|
| **P0 壳层先行** | Electron 壳打进现有 demo；WS 下沉主进程；单实例锁**按 profile 区分（支持同机多开）**；托盘/开机自启/原生通知/未读角标/深链 | **S**，可与后端并行 | 无，立即可启动 |
| **P1 消息可靠性地基** | 切 MySQL（go-sql-driver、DSN 走环境变量）；消息表统一+seq（last_seq 原子取号）；WS 单例+重连+ACK（client_msg_id）；增量拉取接口；修 200 条缺口 | **M** | — |
| **P2 账号与好友** | 注册、用户搜索、好友申请/同意/删除/黑名单、资料编辑+头像上传 | **M**（~8 接口 + 2-3 页面） | P1 |
| **P3 会话重构+群聊** | conversations 重构+members；私聊迁移；建群/群成员管理/群消息广播/群设置 | **L，全计划最大头** | P1、P2 |
| **P4 IM 完整体验** | 未读数/已读回执、撤回、置顶/免打扰、多端同步、漂流瓶并入统一消息模型 | **M-L** | P1 seq + P3 新会话模型 |
| **P5 内部分发打包** | 双平台打包出安装包 + 分发使用说明（免签名口径） | **S-M** | P0 |

**规模口径**（不给具体排期，取决于投入人数）：后端新增/重构约 5 张表 + 20 个接口 + 4 个核心模块（auth/chat/bottle/ws hub）；前端从 6 页扩到约 12-14 页并引入状态管理；壳层 1 套工程。整体量级≈在 demo 骨架上重写后端一半、前端翻倍。

---

## 7. 客户端打包与分发

一台 macOS 即可完成双平台产物（反向不成立：Windows 打不出 mac 包）：

```
npm run dist:mac     →  dist/ripple-x.x.x-universal.dmg   （Intel+Apple Silicon 一份包）
npm run dist:win     →  dist/ripple Setup x.x.x.exe       （NSIS，x64）
```

- electron-builder 自动下载对应平台 Electron 预编译包；免签名场景在 macOS 上交叉打包 Windows 无需任何证书
- CI（GitHub Actions mac runner）可跑同样命令
- 免签名使用注意（写入使用说明）：mac 首次打开「右键 → 打开」；Windows SmartScreen 点「仍要运行」；功能无影响

## 8. 风险与缓解

| 风险 | 缓解 |
|---|---|
| P3 是破坏性变更，现有 conversations/messages 不兼容 | **已拍板不考虑迁移（决策⑤）**：schema 直接改，存量 `server/data/ripple.db` 删除后由 seed 重建（人工操作） |
| SQLite 并发天花板 | **已由决策②解除**（切 MySQL）；代价是引入外部 MySQL 实例依赖（连接信息走环境变量，本机实例不可用时用远程实例） |
| 已读语义多端歧义 | 实施前定义清楚"单端已读 vs 全端已读"（建议：任一端已读即会话已读） |
| 无更新通道的已分发客户端失去迭代能力 | 内部阶段接受手动重装；对外分发时补 electron-updater + 静态更新源 |
| 后台保活 | P0 即把 WS 下沉主进程（两框架通用的稳妥解，避免渲染层被系统节流） |
| Electron 托盘引用被 GC | 已知坑，开发规范里注明持有全局引用 |

## 9. 已拍板决策记录（2026-09-27）

1. **交付范围**：先做可用的内部版本（免签名，不做公证/自动更新；对外分发留后）
2. **后端存储**：**MySQL**（用户 09-27 二次拍板，覆盖最初的 SQLite 决定）。连接信息只走环境变量 `RIPPLE_MYSQL_DSN`，不写入仓库；seq 用 `conversations.last_seq` 的 `LAST_INSERT_ID` 原子取号，禁止 `SELECT MAX(seq)+1`
3. **桌面壳**：Electron
4. **漂流瓶**：保留并并入统一消息模型（匿名别名放 conversation_members.alias）
5. **迁移**：不考虑存量迁移，schema 直接改；存量 `server/data/ripple.db` 删除后由 seed 重建（人工操作，不写迁移脚本）
6. **同机多开**：客户端支持 `--profile`/`RIPPLE_PROFILE`，每 profile 独立 userData 与登录态，不同 profile 可同时运行
7. **账号开通**：客户端自助注册（新增注册页 + `/api/auth/register`）
8. **消息队列**：现阶段不引入，触发条件见 §5.6

## 10. 验证方式

- P0：双平台安装包可安装启动；最小化到托盘后收消息有系统通知且连接不断（挂机 >30min 验证）
- P1：杀进程重启后消息按 seq 增量补齐不重不丢；超 200 条会话能看到最新消息
- P2/P3：**多账号（≥3）端到端**——各自注册→搜索→加好友→同意→建群→第三者入群→群内互发、单聊同时可用；同机多开（不同 profile）互测；黑名单后不可再发
- P4：未读数准确、撤回双端消失、双端登录消息一致
- P5：全新机器按使用说明完成安装与首次打开

## 11. 回滚方案

- 分期实施每期独立分支与提交，P1/P3 破坏性变更前打 tag；出问题回退 tag 并以 seed 重建数据
- 壳层（P0）纯增量，可直接移除 electron 目录回到纯 Web 形态
