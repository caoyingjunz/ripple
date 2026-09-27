# Ripple

Web-first 聊天 + 漂流瓶演示应用（React + Go + SQLite）。

## 快速开始

### 后端

```bash
cd server
go run ./cmd/server
```

默认监听 `http://localhost:8080`，数据库文件 `server/data/ripple.db`，上传目录 `../uploads`。

环境变量（可选）：`RIPPLE_ADDR`、`RIPPLE_DB`、`RIPPLE_UPLOAD`、`RIPPLE_JWT_SECRET`。

### 前端

```bash
cd web
npm install
npm run dev
```

浏览器打开 Vite 提示的地址（默认 `http://localhost:5173`）。

## 演示账号

| 用户名 | 密码 |
|--------|------|
| alice / bob / carol / dave / erin | `demo123` |

建议开两个浏览器（或一个普通窗口 + 一个隐私窗口）分别登录不同账号体验捡瓶与私聊。

## 功能

- 演示账号登录（JWT）
- 首页双入口：聊天 / 漂流瓶
- 一对一文字与图片消息（WebSocket 实时）
- 扔瓶、捡瓶、匿名来回（最多 6 轮）、公开身份转私聊
