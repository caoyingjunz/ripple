# Ripple 桌面化实施规格书（P0-P5 落地契约）

> 版本：v1.0 ｜ 日期：2026-09-28 ｜ 依据：`docs/design.md` v1.1（决策①-⑧全部生效）
> 本文是后端 / 前端 / 桌面壳三方并行的**唯一契约**：字段名、事件名、错误码以此为准，不得单方面变更。

## 0. 总目标与范围

按 design.md 把 ripple 从 Web demo 改造为 QQ 类桌面 IM：

- **后端（P1-P4）**：SQLite → MySQL；统一消息模型 + 会话内 seq；注册/搜索/好友/黑名单；会话重构（single|group|bottle 三型 + members 多对多）；未读/已读/撤回/置顶/免打扰/presence；漂流瓶并入统一消息模型
- **前端（P1-P4 web 侧）**：apiBase 抽象、WS 全局单例 + 指数退避重连 + 断线补拉 + 按 message.id 去重；Zustand 状态管理；页面从 6 扩到 ~12
- **桌面壳（P0/P5）**：Electron 壳、WS 下沉主进程、托盘/原生通知/未读角标、`--profile` 同机多开、electron-builder 双平台打包配置
- **不做**：消息队列（§5.6）、代码签名（§4.2）、存量数据迁移（决策⑤）、群主转让/解散（超范围）

## 1. 文件所有权（严格边界，禁止越界）

| 所有者 | 目录/文件 |
|---|---|
| 后端 agent | `server/**`（go.mod、cmd、internal/*） |
| 前端 agent | `web/**` |
| 桌面 agent | `electron/**`、根 `package.json`、根 `.gitignore`、`docs/desktop-usage.md` |
| 架构师（我） | `docs/*.md`、`README.md`、`.gitcommits`、git 提交 |

任何人不得 git commit / git add（由架构师统一提交）。

## 2. 数据库（MySQL **5.7.44**，共享实例，utf8mb4）

**使用与 rainbow 同一个 MySQL 实例**（host `peng` = 111.124.195.72:3306，root），本方案独占其中的 `ripple` 与 `ripple_test` 两个库（已创建，`ripple` 已置 utf8mb4）。凭据与 rainbow 一致（见 `rainbow/config.yaml`，**不写入本仓库**）。

连接：**只走环境变量** `RIPPLE_MYSQL_DSN`，无默认值；为空时启动报错并提示设置方法（决策②）。格式（密码参考 rainbow 实例）：

```
export RIPPLE_MYSQL_DSN='root:<密码>@tcp(peng:3306)/ripple?charset=utf8mb4&multiStatements=true'
export RIPPLE_TEST_MYSQL_DSN='root:<密码>@tcp(peng:3306)/ripple_test?charset=utf8mb4&multiStatements=true'
```

`multiStatements=true` 供启动时建表。驱动 `github.com/go-sql-driver/mysql`，删除 `modernc.org/sqlite` 依赖及 `server/data/ripple.db` 引用（旧库文件保留在磁盘不动，代码不再使用）。

> **⚠️ 共享实例红线**：该实例同时承载 rainbow / pixiu / baize / fastjufu / jianghu 等库，**只允许操作 `ripple` 与 `ripple_test`**；严禁 DROP DATABASE、严禁触碰其它库；e2e/单测只在 `ripple_test` 内 DROP/重建表。连接池设置：`SetMaxOpenConns(20)`、`SetMaxIdleConns(5)`、`SetConnMaxLifetime(30m)`（解除原单连接瓶颈，又不拖垮共享实例）。

**5.7 兼容性已核对**：DDL 全部兼容（索引长度均 ≤767 字节：username 32、(user_id,friend_id) 288、single_key 332、(conversation_id,seq) 152）；`ORDER BY RAND()`、`LAST_INSERT_ID(expr)` 取号、multiStatements 均可用；排序规则用 `utf8mb4_general_ci`（无 8.0 的 0900_ai_ci）；禁用 8.0 特有语法（窗口函数别名、CTE 仅按需谨慎使用，本方案不需要）。

建表 DDL（`db.Migrate` 执行，幂等 `CREATE TABLE IF NOT EXISTS`；启动时顺带 `UPDATE users SET status='offline'` 清理崩溃残留）：

```sql
CREATE TABLE IF NOT EXISTS users (
  id VARCHAR(36) PRIMARY KEY,
  username VARCHAR(32) NOT NULL UNIQUE,
  display_name VARCHAR(64) NOT NULL,
  password_hash VARCHAR(100) NOT NULL,
  avatar_color VARCHAR(16) NOT NULL DEFAULT '#0d9488',
  avatar_url VARCHAR(255) NULL,
  signature VARCHAR(128) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'offline',
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS friendships (
  id VARCHAR(36) PRIMARY KEY,
  user_id VARCHAR(36) NOT NULL,          -- 发起方
  friend_id VARCHAR(36) NOT NULL,        -- 接收方
  status VARCHAR(16) NOT NULL,           -- pending | accepted | blocked
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL,
  UNIQUE KEY uq_friendship (user_id, friend_id),
  KEY idx_fs_friend (friend_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS conversations (
  id VARCHAR(36) PRIMARY KEY,
  type VARCHAR(8) NOT NULL,              -- single | group | bottle
  name VARCHAR(64) NULL,                 -- 群名
  owner_id VARCHAR(36) NULL,             -- 群主（group 用）
  single_key VARCHAR(83) NULL UNIQUE,    -- single 型专用：升序拼接 'a|b'，幂等去重
  last_seq BIGINT NOT NULL DEFAULT 0,    -- 会话内最大 seq（原子取号）
  last_message_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  KEY idx_conv_time (last_message_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS conversation_members (
  conversation_id VARCHAR(36) NOT NULL,
  user_id VARCHAR(36) NOT NULL,
  role VARCHAR(16) NOT NULL DEFAULT 'member',  -- owner | member
  alias VARCHAR(32) NULL,                     -- bottle 型：扔瓶人/捡瓶人
  last_read_seq BIGINT NOT NULL DEFAULT 0,     -- 已读水位（任一端已读即已读）
  pinned TINYINT NOT NULL DEFAULT 0,
  muted TINYINT NOT NULL DEFAULT 0,
  joined_at BIGINT NOT NULL,
  PRIMARY KEY (conversation_id, user_id),
  KEY idx_cm_user (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS messages (
  id VARCHAR(36) PRIMARY KEY,
  conversation_id VARCHAR(36) NOT NULL,
  seq BIGINT NOT NULL,                   -- 会话内单调递增
  sender_id VARCHAR(36) NOT NULL,
  type VARCHAR(8) NOT NULL,               -- text | image | system
  body TEXT NOT NULL,
  image_url VARCHAR(255) NULL,
  revoked_at BIGINT NULL,
  created_at BIGINT NOT NULL,
  UNIQUE KEY uq_conv_seq (conversation_id, seq),
  KEY idx_msg_time (conversation_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bottles (
  id VARCHAR(36) PRIMARY KEY,
  thrower_id VARCHAR(36) NOT NULL,
  content TEXT NOT NULL,
  status VARCHAR(16) NOT NULL,           -- floating | picked | closed
  picker_id VARCHAR(36) NULL,
  created_at BIGINT NOT NULL,
  picked_at BIGINT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

CREATE TABLE IF NOT EXISTS bottle_threads (
  id VARCHAR(36) PRIMARY KEY,
  bottle_id VARCHAR(36) NOT NULL UNIQUE,
  thrower_id VARCHAR(36) NOT NULL,
  picker_id VARCHAR(36) NOT NULL,
  round_count INT NOT NULL DEFAULT 0,
  max_rounds INT NOT NULL DEFAULT 6,
  revealed TINYINT NOT NULL DEFAULT 0,
  conversation_id VARCHAR(36) NULL,      -- bottle 型会话 id
  closed TINYINT NOT NULL DEFAULT 0,
  created_at BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

`bottle_messages` 表**删除**（统一进 messages）。外键可省（demo 保持轻量），唯一键/索引必须保留。

### 2.1 seq 原子取号（决策②，禁止 MAX+1）

事务内、同一连接（`tx.Exec` + `tx.QueryRow`）：

```go
tx.Exec(`UPDATE conversations SET last_seq = LAST_INSERT_ID(last_seq + 1) WHERE id = ?`, convID)
var seq int64
tx.QueryRow(`SELECT LAST_INSERT_ID()`).Scan(&seq)
```

消息写入流程（`InsertMessage`，REST 与 WS 共用同一条 store 路径）：
1. 校验成员资格（single 型加双向黑名单校验；bottle 型由漂流瓶专用方法另行封装）
2. 事务：取号 → INSERT messages → `UPDATE conversations SET last_message_at=?` → Commit
3. Commit 后广播（推送只是提示，真相源永远是库）

### 2.2 会话/成员语义

- **single**：`GetOrCreateSingle(a,b)` 用 `single_key=升序 a|b` INSERT，撞唯一键则回查既有 id（幂等）。bottle reveal 产生的私聊同规则
- **group**：创建即写 members（owner + member_ids 去重去自己，上限 50 人），并发 system 消息「XXX 创建了群聊「name」」；加人 system「XXX 邀请 A、B 加入群聊」；移人 system「XXX 移除了 A」；退出 system「A 退出了群聊」；改名 system「XXX 修改群名为「name」」
- **bottle**：捡瓶时创建（members 两条，alias=扔瓶人/捡瓶人），漂流瓶消息全部进该 conversation，匿名靠**对外投影**（见 §4 bottle 部分），不靠独立表
- 聊天列表 `GET /api/conversations` 只含 `type IN ('single','group')`，bottle 型只经漂流瓶接口访问

### 2.3 好友关系状态机（单行模型：`(user_id=发起方, friend_id, status)`）

- 查询视角 A 看 B：`blocked`=A 屏蔽了 B（行 user_id=A,status=blocked）；`accepted`=任一方向 accepted；`pending_in`=B→A 待处理；`pending_out`=A→B 待处理
- A 申请 B：反向 pending 存在 → 直接置 accepted（互加秒通过，返回 accepted）；已好友/已屏蔽/对方屏蔽我 → 409
- block：upsert 行 `(me → them, blocked)` 并删除反向行；unblock：删除该行（回到 none，不恢复好友）
- 删好友：删除任一方向 accepted 行
- **黑名单拦截**：single 会话发消息时双向 blocked 校验（403）；漂流瓶 pick 排除与我有屏蔽关系（双向）的瓶子

## 3. REST API 契约

错误统一 `{"error":"..."}`；鉴权同现状（`Authorization: Bearer`，JWT 24h，middleware 不变）。**字段名以本表为准**。

### 3.1 鉴权与资料

| 方法/路径 | 请求 | 成功响应 | 错误 |
|---|---|---|---|
| POST /api/auth/login | `{username,password}` | 200 `{token,user:User}` | 401 凭据错误 |
| POST /api/auth/register | `{username,password,display_name?}` | 201 `{token,user:User}` | 400 格式不符（username `^[a-z0-9_]{3,32}$`、password ≥6、display_name ≤32 默认=username）；409 用户名已存在 |
| GET /api/me | — | 200 User | 401 |
| PUT /api/me | `{display_name?,signature?}` | 200 User | 400 |
| POST /api/me/avatar | multipart `file`（图片≤2MB，逻辑同 media/upload） | 200 `{avatar_url}` | 400/413 |

`User = {id,username,display_name,avatar_color,avatar_url,signature,status}`（avatar_url 可 null；status=online|offline）

### 3.2 用户与好友

| 方法/路径 | 说明 | 成功响应 |
|---|---|---|
| GET /api/users/search?q= | username/display_name 模糊匹配，排除自己，≤20 条 | 200 `{users:[{user:User,relation}]}`，relation=none/pending_out/pending_in/accepted/blocked |
| GET /api/friends | 好友全景 | 200 `{friends:[User],pending_in:[FriendRequest],pending_out:[FriendRequest],blocked:[User]}` |
| POST /api/friends/requests | `{user_id}` | 201 `{status:'pending',friend:User}`；反向 pending 自动通过 → 200 `{status:'accepted',friend:User}`；404 用户不存在；409 不可申请 |
| POST /api/friends/requests/{id}/accept | 收件人本人 | 200 `{friend:User}`；404/409 |
| POST /api/friends/requests/{id}/decline | 收件人本人 | 200 `{status:'declined'}` |
| DELETE /api/friends/{user_id} | 删好友（或撤销自己发起的 pending） | 200 `{status:'removed'}` |
| POST /api/friends/{user_id}/block | 拉黑 | 200 `{status:'blocked'}`；409 已屏蔽；404 |
| DELETE /api/friends/{user_id}/block | 解除拉黑 | 200 `{status:'unblocked'}` |

`FriendRequest = {id,user:User,created_at}`

### 3.3 会话

| 方法/路径 | 说明 | 成功响应 |
|---|---|---|
| GET /api/conversations | 我参与的 single+group，按 pinned DESC、COALESCE(last_message_at,created_at) DESC | 200 `{conversations:[ConversationSummary]}` |
| POST /api/conversations | `{type:'single',user_id}`（幂等）→ 200/201 Summary；`{type:'group',name,member_ids:[]}` → 201 Summary（name 1-32 字） | 400/404 |
| GET /api/conversations/{id} | single/group 明细 | 200 `{conversation:Summary,members:[{user:User,role,alias,joined_at}]}`；bottle 型 404 |
| PUT /api/conversations/{id} | `{name}`（group，owner） | 200 Summary |
| POST /api/conversations/{id}/members | `{user_ids:[]}`（owner；user_ids ≤50；已成员跳过） | 200 `{members:[User]}` |
| DELETE /api/conversations/{id}/members/{uid} | owner 移人；uid==owner → 403 | 200 `{status:'removed'}` |
| POST /api/conversations/{id}/leave | owner 不可退（403） | 200 `{status:'left'}` |
| GET /api/conversations/{id}/messages | 参数：`after_seq`（增量，seq>after_seq ASC）、`before_seq`（翻历史，seq<before_seq 取最新 limit 条后转 ASC）、`limit`（默认 50，≤100）。都不带 → 最新 limit 条（ASC） | 200 `{messages:[Message],has_more:bool}`（has_more=结果集外还有同方向更旧/更新） |
| POST /api/conversations/{id}/messages | `{type:'text'|'image',body,image_url?}` | 201 Message；403 非成员/黑名单；404 会话不存在 |
| POST /api/conversations/{id}/read | `{seq}` | 200 `{last_read_seq}`（只升不降） |
| POST /api/messages/{id}/revoke | 仅发送者、≤2 分钟 | 200 `{message:Message}`（body 清空、revoked_at=now、image_url=null）；403 超时/非本人 |
| POST /api/media/upload | multipart `file`（现状逻辑保留） | 200 `{url:"/uploads/xxx"}` |

```
ConversationSummary = {
  id, type:'single'|'group', name:string|null,
  peer:User|null,               // single 用
  owner_id:string|null,         // group 用
  member_count:number,          // group 用（single=2）
  unread:number,                // max(last_seq-last_read_seq,0)
  last_seq:number, last_read_seq:number,
  last_message_preview:string|null,   // 文本取 body 前 40 字；image→[图片]；system 取 body；revoked→[已撤回]
  last_message_at:number|null,
  pinned:boolean, muted:boolean,
  created_at:number
}
Message = {id,conversation_id,seq,sender_id,type:'text'|'image'|'system',body,image_url:string|null,revoked_at:number|null,created_at}
```

### 3.4 漂流瓶（并入统一模型）

| 方法/路径 | 说明 | 成功响应 |
|---|---|---|
| POST /api/bottles | `{content}`（非空） | 201 Bottle |
| POST /api/bottles/pick | 随机捡（MySQL `ORDER BY RAND()`；排除自己的瓶与双向黑名单的瓶）；事务内：瓶→picked、建 bottle 会话+members(alias)、建 thread、瓶内容作为第一条消息（sender=扔瓶人） | 201 BottleThreadView；404 无瓶可捡；409 并发抢瓶 |
| GET /api/bottles/mine | 我扔的+我捡的，含 thread 状态 | 200 `{bottles:[Bottle]}`（Bottle 增补 `conversation_id,unread`，picked 后必有 thread_id） |
| GET /api/bottles/threads/{id} | 会话成员才可见 | 200 BottleThreadView |
| POST /api/bottles/threads/{id}/messages | `{body}`；round_count≥max_rounds → 409 max rounds reached；closed/revealed → 409 thread closed | 201 BottleMessageView(mine=true) |
| POST /api/bottles/threads/{id}/reveal | 建私聊（GetOrCreateSingle）+ system 消息；thread revealed=1,closed=1；瓶 closed | 200 `{conversation_id,peer:User}` |
| POST /api/bottles/threads/{id}/close | 关闭匿名对话 | 200 `{status:'closed'}` |

```
BottleThreadView = {id,bottle_id,bottle_content,my_alias,round_count,max_rounds,revealed,closed,conversation_id,created_at,messages:[BottleMessageView]}
BottleMessageView = {id,conversation_id,seq,alias,mine,type:'text',body,created_at}   // 无 sender_id（匿名投影）
```

**匿名投影规则**：bottle 会话的消息对两端的对外响应/推送一律用 `BottleMessageView`（alias 来自 sender 的 conversation_members.alias，mine 按收件人视角计算）。messages 表内 sender_id 照存（真相源），但 bottle 型的任何 API/WS 输出都不暴露真实身份。

## 4. WS 协议（hub 重写要点）

连接：`/api/ws?token=`（不变）。**服务端心跳超时**：`conn.SetReadDeadline(70s)`，收到任意客户端帧即续期；超时/读错误 → 清理连接 → presence 变 offline。写并发：每连接 `sync.Mutex` 保护 WriteMessage（gorilla 并发写会 panic）。客户端每 25s 发 `{type:'ping'}`，服务端回 `{type:'pong'}`。

Hub 内存维护 `map[uid]connCount`；0→1 置 online（UPDATE users + 广播 presence.change），→0 置 offline 并广播。广播范围：**全体在线用户**（demo 规模）。

帧定义（`type` 字段取值）：

```
C→S:
  {type:'ping'}
  {type:'message.send',client_msg_id,conversation_id,msg_type:'text'|'image',body,image_url}
      → 走与 REST 相同的 InsertMessage 路径；成功后：发送连接回 message.ack，全体成员（含发送者其它端）收 message.new

S→C:
  {type:'pong'}
  {type:'message.ack',client_msg_id,message:Message}                       // 仅发送连接
  {type:'message.new',message:Message}                                      // 全体成员
  {type:'message.revoked',conversation_id,message_id,revoked_at}
  {type:'message.read',conversation_id,user_id,seq}                        // 已读水位推进（发给其它成员）
  {type:'presence.change',user_id,online:boolean}
  {type:'friend.request',action:'pending'|'accepted',request:{id,user:User,created_at}}
      // pending→发给收件人；accepted→发给原申请人（request.user=新好友）
  {type:'conversation.updated',conversation_id,event:'created'|'renamed'|'members_added'|'member_removed'|'left',actor_id,user_ids?}
      // 发给受影响成员；前端收到即重拉会话列表/明细
  {type:'bottle.message',thread_id,message:BottleMessageView}              // 漂流瓶新消息（两端各自的 mine 视角）
  {type:'bottle.revealed',thread_id,conversation_id,peer:User}              // 双端各收到对方视角的 peer
  {type:'error',message}
```

message.send 失败（403 黑名单/非成员等）→ 发送连接回 `{type:'error',client_msg_id,message}`（带 client_msg_id 便于前端置灰重发）。

已读语义（design §8）：**任一端已读即会话已读**——水位存 conversation_members.last_read_seq（用户级），不按端拆分。

## 5. 前端契约（web/）

技术栈：React 18 + Vite + react-router v6 不变，**新增 Zustand**。`npm run build`（tsc -b + vite build）必须零错误通过。

### 5.1 apiBase 抽象（收敛浏览器耦合）

- `src/api/base.ts`：`getApiBase()/setApiBase()`，localStorage key `ripple_server_base`，默认 `""`（web dev 同源走 vite proxy）；可选构建期覆盖 `import.meta.env.VITE_API_BASE`
- 所有 fetch 用 `getApiBase() + path`；上传返回的相对 url `/uploads/x` 渲染时拼 base
- **桌面模式识别**：`window.ripple` 存在即为桌面（见 §6.2 契约）；LoginPage 在桌面模式下显示「服务器地址」输入框（必填，默认记住值或 `http://127.0.0.1:8080`），登录时 `setApiBase` 持久化
- token 仍存 localStorage（桌面端每 profile 独立 userData，天然隔离）

### 5.2 WS 全局单例（App 级，替代每页 useWS）

`src/ws/` 实现传输层抽象 + 单例 `WsClient`：

```ts
interface Transport {
  connect(apiBase: string, token: string): void
  disconnect(): void
  send(payload: unknown): void
  onEvent(cb: (frame: unknown) => void): () => void   // 返回取消订阅函数
}
```

- **BrowserTransport**：原生 WebSocket，`ws(s)://host/api/ws?token=`；自动重连指数退避（1s 起倍增，上限 30s，抖动 ±20%）；连上/断开时合成内部帧 `{type:'ws.status',status:'connected'|'reconnecting'}` 抛给上层
- **ElectronTransport**：`window.ripple.ws.*` 直通（见 §6.2），不做自身重连（主进程负责），透传主进程的 `ws.status` 帧
- 顶层按 `window.ripple ? ElectronTransport : BrowserTransport` 选择；登录后 connect、退出登录 disconnect
- **断线补拉**：收到 `ws.status:connected`（且非首次启动）→ 对每个会话用已知 last_seq 调 `GET messages?after_seq=` 增量补齐；**按 message.id 去重**（WS 推送与补拉重叠）

### 5.3 状态（Zustand stores）

- `sessionStore`：token/user、login/register/logout
- `conversationsStore`：会话列表 + 未读 + 每会话 last_seq 水位 + 打开中会话消息（含发送中 pending 消息与 ack 合并）；message.new/ack/revoked/read、conversation.updated → 本地合并 + 节流重拉列表
- `friendsStore`：好友/申请/黑名单 + friend.request 事件合并
- `presenceStore`：在线 uid 集合

### 5.4 路由与页面（从 6 页 → ~12 页）

```
/login /register
/                      Home（导航卡片：聊天/好友/漂流瓶 + 个人信息入口）
/chat                  会话列表（未读角标、置顶、免打扰标记、搜索过滤）
/chat/:id              聊天室（single：对端资料+在线点；group：群名+成员抽屉；图片发送；
                       打开及收到新消息上报已读；自己消息 2 分钟内可撤回；历史向上翻页 before_seq；
                       断线重连增量补拉 + 去重；乐观发送：pending 气泡 ← message.ack 按 client_msg_id 替换）
/friends               好友页（Tab：好友/申请/黑名单 + 搜索用户 + 加好友/同意/拒绝/删除/拉黑/解除；
                       好友行「发消息」→ POST conversations{single} → 跳 /chat/:id）
/group/create          选好友建群（名称 + 多选）
/conversation/:id/info 群资料（成员列表/改名/邀请/移出/退出群聊）
/profile               编辑资料（昵称/签名/头像上传裁剪不做，原图上传即可）
/bottle                漂流瓶海（扔瓶/捡瓶/我的瓶子列表含未读角标）
/bottle/thread/:id     匿名对话（alias 气泡、轮次进度、reveal/关闭）
```

视觉沿用现有 `styles.css` 风格（简洁 QQ 蓝调），新增样式追加在同文件，不引 UI 组件库。

## 6. 桌面壳契约（electron/ + 根 package.json）

### 6.1 主进程（`electron/main.js`，CommonJS，无需编译）

- **多开**：启动时解析 `--profile=<name>` / `RIPPLE_PROFILE`（默认 `default`）→ `app.setPath('userData', join(app.getPath('userData'),'..','RippleProfiles',profile))` → **先 setPath 再 requestSingleInstanceLock**（锁按 userData 区分，天然 per-profile）；抢锁失败 → 提示已有同 profile 实例并退出
- **窗口**：`webPreferences:{preload, contextIsolation:true, nodeIntegration:false, backgroundThrottling:false}`；dev 时 `loadURL('http://localhost:5173')`（`RIPPLE_DEV=1`），打包时 `loadFile('web/dist/index.html')`
- **WS 下沉主进程**：`electron/ws-client.js` 原生 WebSocket 客户端，指数退避重连；connect 参数由渲染层经 IPC 传入（`ripple:ws-connect {apiBase,token}`）；事件 `webContents.send('ripple:ws-event', frame)` 广播到所有窗口；帧含 `{type:'ws.status',status:'connected'|'disconnected'|'reconnecting'}` 供渲染层触发补拉
- **托盘**：`electron/tray.js`，全局引用持有（防 GC）；菜单：显示主窗 / 开机自启（`app.setLoginItemSettings` toggle）/ 退出；未读角标：mac `app.dock.setBadge(String(n))`，win `win.flashFrame(true)`（收到 message.new 且窗口失焦时）
- **原生通知**：`new Notification().show()`（message.new 且窗口失焦且该会话未免打扰）；点击 → 主窗 `show()+focus()`
- **保活**：WS 连接期间 `powerSaveBlocker.start('prevent-app-suspension')`，断开即 stop
- **深链**：`app.setAsDefaultProtocolClient('ripple')`，`open-url` → 聚焦主窗（最小实现）
- **`--smoke` 模式**：窗口 `ready-to-show` 后 1s 自动 `app.exit(0)`，供 CI/无交互冒烟验证（`ELECTRON_ENABLE_LOGGING=1 npx electron . --smoke`）

### 6.2 preload（`electron/preload.js`，contextBridge）

```js
window.ripple = {
  isDesktop: true,
  platform: process.platform,            // 'darwin' | 'win32' | ...
  ws: {
    connect(apiBase, token)               // ipcRenderer.invoke('ripple:ws-connect', {apiBase, token})
    disconnect()                          // ipcRenderer.invoke('ripple:ws-disconnect')
    send(payload)                         // 主进程 WS.send
    onEvent(cb) → () => void              // 订阅 'ripple:ws-event'，返回取消函数
  }
}
```

渲染层登出 → `ws.disconnect()`。**此对象形状是前后端 agent 的共享契约，不得变更。**

### 6.3 打包（根 `package.json` + `electron-builder.yml`）

- 依赖：devDeps `electron`、`electron-builder`（版本用 `npm view electron version` 查实际最新稳定并锁定）
- scripts：`dev`（启动 web dev + electron dev，说明文档写清两条命令亦可）、`build:web`（cd web && npm ci && npm run build）、`dist:mac`（先 build:web 再 electron-builder --mac --universal）、`dist:win`（--win --x64 nsis）
- 免签名：mac `identity: null`；win 不签名；`CSC_IDENTITY_AUTO_DISCOVERY=false`；配置内预留签名环境变量占位（§4.2）
- 产物：`dist/ripple-x.x.x-universal.dmg`、`dist/ripple Setup x.x.x.exe`
- 图标：仓库内生成一个简单 PNG（可用 node 脚本程序化生成纯色+圆角，不引美工资源），electron-builder 以 PNG 自动派生各尺寸

## 7. 验证标准（各 agent 必须自证，架构师复核）

### 后端
1. `go build ./...`、`go vet ./...` 零错误
2. `RIPPLE_TEST_MYSQL_DSN=<peng 实例/ripple_test 库> go test ./...` 全绿（实例已就绪；测试库每次 DROP 相关表后重建 schema，保证可重复）
3. `server/internal/e2e/e2e_test.go` 覆盖 success criteria（P2/P3/P4 核心）：
   - 3 用户注册登录 → 搜索 → 申请 → 同意 → 互为好友
   - 建群（3 人）→ 群发消息互相可见 → 加人/退群 → 改名
   - single 会话消息、after_seq 增量拉取不重不丢、发超 5 条（>limit 用例证明 200 条缺口已修）、未读数、已读上报、撤回（双端 revoked）
   - 黑名单后不可发（403）
   - 漂流瓶：扔 → 捡 → 匿名消息（投影无 sender_id）→ 6 轮上限 → reveal 产生私聊
   - WS：双用户连接，message.send → ack + 两个 message.new；presence 上/下线广播
4. seed：users 表为空时建 alice/bob/carol/dave/erin（密码 demo123），已有数据跳过

### 前端
1. `cd web && npm install && npm run build` 零错误（tsc -b + vite build）
2. `npm run dev` 可起（架构师抽查）；无 window.ripple 时全部功能走 BrowserTransport，vite proxy 生效

### 桌面壳
1. `npm --prefix . install`（根）成功；`node --check electron/*.js` 通过
2. `npm run build:web && npx electron . --smoke` 退出码 0（窗口创建即成功，自动退出）
3. `npm run dist:mac` 能产出 dmg（若网络/时间受限，至少完成 electron-builder 配置与 dry-run `--dir`，并在交付说明中注明）

## 8. 风险与回滚

- 全部改动在 git 工作区（基线 `b3965e0`），架构师统一提交前可随时 `git checkout -- .` 回退；旧 `server/data/ripple.db` 不删除（决策⑤：切库后作废，人工处理）
- MySQL 不可用 → 服务启动即报错（fail-fast），不会静默写错库
- 桌面壳纯增量（electron/ + 根 package.json），移除即回 Web 形态
