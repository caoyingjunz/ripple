# Ripple 桌面端使用说明（Electron 壳 · P0/P5）

> 适用：`electron/` 桌面壳 + 根 `package.json` + `electron-builder.yml`。
> 壳层职责：承载 web 前端产物、WS 下沉主进程（防系统节流断连）、托盘 / 原生通知 / 未读角标、`--profile` 同机多开、双平台打包。
> 设计依据：`docs/design.md` §4/§6/§7，`docs/implementation-spec.md` §6。

## 1. 前置准备

- Node ≥ 20（开发机实测 v23.7.0）；web 构建需要 `web/` 依赖（见 §2）
- **Electron 版本口径**：当前锁定 `electron@37.10.3`（37 线最新稳定，Node 22）。原因：主力开发机为 macOS 12.6（Intel），Electron 38+/44 的框架强链接 macOS 13+ 的 `SMAppService`，二进制在本机直接 SIGABRT 无法运行；37（Chromium 138，最后支持 macOS 12 的内核）实测本机正常。将来全员上 macOS 13+ 后只需改根 `package.json` 一个版本号即可升级（壳代码零改动，所用 API 均为 37 时代稳定接口）。注意：electron-builder 打出的包在**用户侧**以包内自带 Electron 运行时为准，本机只能实际运行 x64 包（universal 包的 arm64 半边需 Apple Silicon 机器验证）
- 国内网络下载 Electron 二进制走镜像（首次 `npm install` 前导出）：

```bash
export ELECTRON_MIRROR=https://npmmirror.com/mirrors/electron/
export ELECTRON_BUILDER_BINARIES_MIRROR=https://npmmirror.com/mirrors/electron-builder-binaries/
npm install   # 根目录；只装 electron + electron-builder 两个 devDependencies
```

## 2. 开发模式（两条命令）

桌面壳 dev 模式直接加载 Vite dev server（`http://localhost:5173`），代理、热更新与纯浏览器开发一致：

```bash
# 终端 1：启动 web dev server（需 server 在 8080 运行，vite proxy 同源代理 /api）
cd web && npm install && npm run dev

# 终端 2：启动桌面壳（根目录）
npm run dev        # 等价于 RIPPLE_DEV=1 electron .
```

- Windows（cmd）下手动设环境变量：`set RIPPLE_DEV=1 && npx electron .`
- 仅启动壳（加载打包形态的 `web/dist`）：`npm start`
- 冒烟验证（无交互，窗口就绪 1 秒后自动退出，退出码 0 即通过）：

```bash
npx electron . --smoke          # 建议 ELECTRON_ENABLE_LOGGING=1 以便看渲染层日志
```

## 3. 打包分发（P5）

```bash
npm run dist:mac   # 构建前端 + electron-builder --mac --universal
npm run dist:win   # 构建前端 + electron-builder --win --x64（NSIS）
```

产物（`directories.output: dist`，内部版本**免签名**，design 决策①）：

| 平台 | 产物 | 首次打开 |
|---|---|---|
| macOS（Intel+Apple Silicon 一份包） | `dist/ripple-<version>-universal.dmg` | 右键 App →「打开」绕过 Gatekeeper |
| Windows x64 | `dist/ripple Setup <version>.exe` | SmartScreen →「更多信息」→「仍要运行」 |

- 打包链路自检（不出安装包，仅验证配置与产物结构）：`npx electron-builder --dir --mac`
- 出包内容：`electron/**`（壳）+ `web/dist/**`（前端产物）+ `assets/**`（图标）；生产依赖为零，体积主体为 Electron 运行时
- 对外分发（将来）：补证书后启用 `electron-builder.yml` 文件头预留的签名环境变量位（`CSC_LINK` / `CSC_NAME`），链路不变

## 4. 同机多开（--profile）

每个 profile 拥有独立的 `userData`（localStorage、登录态、WS 连接随之隔离），Electron 单实例锁按 userData 区分——**不同 profile 可同时运行，同一 profile 只允许一个实例**：

```bash
npm start -- --profile=alice
RIPPLE_PROFILE=bob npm start          # 另一个账号同时在线
```

- 同 profile 重复启动会弹提示并退出；深链 `ripple://` 唤起同样只聚焦既有实例
- profile 命名规则：`^[A-Za-z0-9_-]{1,64}$`（防注入 userData 路径）
- userData 实际位置（macOS）：`~/Library/Application Support/RippleProfiles/<profile>`；Windows：`%APPDATA%\RippleProfiles\<profile>`

## 5. 桌面端登录与服务器地址

桌面模式下（`window.ripple` 存在），登录页会出现「服务器地址」输入框（默认记住上次值），指向 ripple server 的 HTTP 地址（如 `http://127.0.0.1:8080`）。REST 请求由渲染层直接访问该地址（服务端已开 CORS），WebSocket 由主进程代连（见 §6）。

## 6. 桌面壳与 web 前端的边界（契约摘要）

- 前端通过 `window.ripple`（`electron/preload.js`）识别桌面模式：`isDesktop` / `platform` / `ws.connect(apiBase, token)` / `ws.disconnect()` / `ws.send(payload)` / `ws.onEvent(cb)`
- **WS 下沉主进程**：连接、25s 心跳、指数退避重连（1s 起倍增上限 30s）全在主进程；服务端帧原样转发到渲染层，连接状态变化额外下发 `{type:'ws.status',status:'connected'|'disconnected'|'reconnecting'}`，渲染层据此触发断线补拉
- **后台保活**：WS 连接期间主进程持有 `powerSaveBlocker('prevent-app-suspension')`，主动断开即释放
- 关闭窗口 = 最小化到托盘；托盘菜单：显示主窗 / 开机自启 / 退出

## 7. 已知取舍（首版，留给后续迭代）

1. **通知不做免打扰过滤**：壳层不持有会话级 muted 状态，收到 `message.new` 且主窗失焦即弹原生通知；后续可由渲染层经 IPC 下发免打扰会话集合再过滤
2. **未读角标为近似值**：按「主窗失焦期间收到 message.new」计数，主窗获得焦点即清零；精确未读以渲染层会话列表为准（后续可加 IPC 同步/清除角标）
3. `bottle.message` 首版不触发系统通知（只有 `message.new` 会）
4. 开机自启（托盘菜单开关）基于 `app.setLoginItemSettings`，**未打包的 dev 形态下设置不持久**，打包安装后才真正生效
5. 深链 `ripple://` 为最小实现：唤起仅聚焦主窗，不携带参数路由
6. WS 客户端逻辑已随壳交付，但与真实 server 的联调验证依赖后端/前端 agent 的产出落地后进行（P0 整体验收项）

## 8. 回滚

壳层为纯增量改动：删除 `electron/`、`tools/gen-icon.js`、`electron-builder.yml`、根 `package.json` 即回到纯 Web 形态（design §11）。
