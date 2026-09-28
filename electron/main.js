'use strict'

// Ripple 桌面壳主进程（CommonJS 纯 JS，无编译步骤）。
// 设计依据：docs/implementation-spec.md §6.1；docs/design.md §4/§6 P0+P5。
//
// 与 spec §6.1 的一处实现偏差（已记录在交付说明）：打包产物加载不用
// loadFile('web/dist/index.html')，而是注册自定义标准协议 ripple://local/
// 承载 web/dist。原因：vite 默认产物为绝对路径（/assets/xxx.js），且 web 侧使用
// BrowserRouter——file:// 下两者分别导致资源 404 与 pushState SecurityError 白屏。
// ripple://local/ 是标准 origin：绝对资源、BrowserRouter 深链/刷新、localStorage
// 均可用；SPA 路径回落到 index.html。若 web 侧将来改为相对 base + HashRouter，
// 本方案同样兼容。dev 模式（RIPPLE_DEV=1）仍直连 http://localhost:5173。

const {
  app,
  BrowserWindow,
  Menu,
  dialog,
  ipcMain,
  powerSaveBlocker,
  protocol
} = require('electron')
const path = require('node:path')
const fs = require('node:fs')

const { createWsClient } = require('./ws-client')
const trayModule = require('./tray')

// ---------------------------------------------------------------------------
// 启动参数
// ---------------------------------------------------------------------------

const SMOKE = process.argv.includes('--smoke')
const DEV = process.env.RIPPLE_DEV === '1'

// --profile=<name> / RIPPLE_PROFILE（默认 default），profile 名做白名单校验
// 防止拼进 userData 路径出现越权字符
function resolveProfile() {
  let name = 'default'
  const fromArg = process.argv.find((a) => a.startsWith('--profile='))
  if (fromArg !== undefined) {
    name = fromArg.slice('--profile='.length)
  } else if (process.env.RIPPLE_PROFILE) {
    name = process.env.RIPPLE_PROFILE
  }
  if (!/^[A-Za-z0-9_-]{1,64}$/.test(name)) {
    dialog.showErrorBox(
      'Ripple',
      `无效的 profile 名称：「${name}」\n只允许字母、数字、下划线、连字符，长度 1-64。`
    )
    app.exit(1)
    return null
  }
  return name
}

// 必须先设置好 userData 再 requestSingleInstanceLock：
// Electron 单实例锁按 userData 目录区分，天然实现 per-profile 同机多开。
app.setName('Ripple')
const PROFILE = resolveProfile()
if (PROFILE === null) {
  /* resolveProfile 内已 app.exit，只是兜底让静态检查知道后续不再执行 */
  throw new Error('invalid profile')
}
const defaultUserData = app.getPath('userData') // setName 之后调用，取 Ripple 的默认目录
app.setPath('userData', path.join(path.dirname(defaultUserData), 'RippleProfiles', PROFILE))

// 深链 ripple:// —— 最小实现：唤起即聚焦主窗
if (process.defaultApp) {
  // 未打包的 dev 形态：带上入口脚本路径才能正确转发命令行
  app.setAsDefaultProtocolClient(
    'ripple',
    process.execPath,
    process.platform === 'win32' ? [path.resolve(process.argv[1])] : []
  )
} else {
  app.setAsDefaultProtocolClient('ripple')
}

// win 通知需要 AppUserModelId，否则系统通知可能不弹
if (process.platform === 'win32') {
  app.setAppUserModelId('com.pixiu.ripple')
}

// ---------------------------------------------------------------------------
// 单实例锁（按 profile 隔离）
// ---------------------------------------------------------------------------

const gotLock = app.requestSingleInstanceLock()
if (!gotLock) {
  dialog.showErrorBox(
    'Ripple',
    `已有一个 profile 为「${PROFILE}」的 Ripple 实例在运行。\n如需同机多开，请用 --profile=<名称> 指定不同 profile。`
  )
  app.exit(1)
}
console.log(`[ripple] profile=${PROFILE} lock=${gotLock} dev=${DEV} smoke=${SMOKE}`)

app.on('second-instance', () => {
  // 同 profile 二次启动（含 win 深链 argv 唤起）：聚焦既有主窗
  showMainWindow()
})

app.on('open-url', (event) => {
  event.preventDefault()
  // 边界（双职责并存）：ripple:// 的 OS 级注册只用于「外部深链唤起」——此处仅聚焦
  // 主窗、从不 loadURL 导航；窗口内容始终由 ripple://local/（web/dist 承载，同
  // scheme 不同 host）驱动。二者互不干扰：外部深链不会改变页面，页面导航
  // 也不会触发 open-url。
  showMainWindow()
})

// ---------------------------------------------------------------------------
// 静态资源协议 ripple://local/ → web/dist（见文件头偏差说明）
// ---------------------------------------------------------------------------

const WEB_ROOT = path.join(__dirname, '..', 'web', 'dist')

protocol.registerSchemesAsPrivileged([
  {
    scheme: 'ripple',
    privileges: {
      standard: true, // 标准 origin：支持 localStorage / BrowserRouter pushState
      secure: true,
      supportFetchAPI: true,
      corsEnabled: true
    }
  }
])

const MIME_TYPES = {
  '.html': 'text/html; charset=utf-8',
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.json': 'application/json; charset=utf-8',
  '.map': 'application/json; charset=utf-8',
  '.png': 'image/png',
  '.jpg': 'image/jpeg',
  '.jpeg': 'image/jpeg',
  '.gif': 'image/gif',
  '.webp': 'image/webp',
  '.svg': 'image/svg+xml',
  '.ico': 'image/x-icon',
  '.woff': 'font/woff',
  '.woff2': 'font/woff2',
  '.ttf': 'font/ttf',
  '.wasm': 'application/wasm'
}

async function handleRippleRequest(request) {
  let relPath
  try {
    relPath = decodeURIComponent(new URL(request.url).pathname)
  } catch {
    return new Response('Bad Request', { status: 400 })
  }
  const abs = path.normalize(path.join(WEB_ROOT, relPath))
  // 路径穿越防护：只允许 webRoot 内的文件
  if (abs !== WEB_ROOT && !abs.startsWith(WEB_ROOT + path.sep)) {
    return new Response('Forbidden', { status: 403 })
  }
  let body
  try {
    body = await fs.promises.readFile(abs)
  } catch {
    // 文件不存在（含 SPA 客户端路由路径 / 目录）：回落 index.html
    try {
      body = await fs.promises.readFile(path.join(WEB_ROOT, 'index.html'))
      return new Response(body, { headers: { 'content-type': MIME_TYPES['.html'] } })
    } catch {
      return new Response('web/dist 缺失：请先运行 npm run build:web', { status: 404 })
    }
  }
  const type = MIME_TYPES[path.extname(abs).toLowerCase()] || 'application/octet-stream'
  return new Response(body, { headers: { 'content-type': type } })
}

// ---------------------------------------------------------------------------
// 窗口
// ---------------------------------------------------------------------------

/** @type {BrowserWindow | null} */
let mainWindow = null
let isQuitting = false
let wsClient = null
let psBlockerId = null

function startPowerSave() {
  if (psBlockerId !== null) return
  psBlockerId = powerSaveBlocker.start('prevent-app-suspension')
}

function stopPowerSave() {
  if (psBlockerId === null) return
  powerSaveBlocker.stop(psBlockerId)
  psBlockerId = null
}

// 服务端帧 + 合成的 ws.status 帧 → 广播到所有窗口；壳层附带处理（托盘/保活）
function broadcastFrame(frame) {
  if (!frame || typeof frame !== 'object') return
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed()) win.webContents.send('ripple:ws-event', frame)
  }

  if (frame.type === 'ws.status') {
    // 保活（spec §6.1）：WS 连接期间阻止系统休眠挂起；用户主动断开即解除。
    // reconnecting 期间保持 blocker，确保断网恢复后能立即重连
    if (frame.status === 'connected') startPowerSave()
    else if (frame.status === 'disconnected') stopPowerSave()
  } else if (frame.type === 'message.new') {
    const focused =
      !!mainWindow && !mainWindow.isDestroyed() && mainWindow.isVisible() && mainWindow.isFocused()
    trayModule.onMessageNew(frame, focused)
  }
}

function loadApp() {
  if (DEV) {
    mainWindow.loadURL('http://localhost:5173/').catch(() => {
      dialog.showErrorBox('Ripple dev', '无法连接 http://localhost:5173，请先在 web/ 目录启动 npm run dev。')
    })
    return
  }
  if (!fs.existsSync(path.join(WEB_ROOT, 'index.html'))) {
    const html =
      '<!doctype html><meta charset="utf-8"><title>Ripple</title>' +
      '<body style="font:14px/1.6 sans-serif;color:#333;padding:40px">' +
      '<h2>未找到前端构建产物</h2><p>请先在项目根目录运行 <code>npm run build:web</code>，' +
      '或以开发模式启动：<code>RIPPLE_DEV=1 npm run dev</code>。</p></body>'
    mainWindow.loadURL('data:text/html;charset=utf-8,' + encodeURIComponent(html))
    return
  }
  // 打包形态：自定义标准协议承载 web/dist（见文件头偏差说明）
  mainWindow.loadURL('ripple://local/').catch((err) => {
    dialog.showErrorBox('Ripple', '页面加载失败：' + err)
  })
}

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1200,
    height: 800,
    minWidth: 900,
    minHeight: 640,
    title: 'Ripple',
    show: false,
    backgroundColor: '#f5f7fa',
    webPreferences: {
      preload: path.join(__dirname, 'preload.js'),
      contextIsolation: true,
      nodeIntegration: false,
      backgroundThrottling: false
    }
  })

  mainWindow.once('ready-to-show', () => {
    mainWindow.show()
    if (SMOKE) {
      // 无交互冒烟：窗口就绪 1s 后退出（spec §6.1）
      setTimeout(() => app.exit(0), 1000)
    }
  })
  mainWindow.webContents.on('did-fail-load', (_e, code, desc, url, isMain) => {
    console.error('[ripple] did-fail-load', code, desc, url)
    if (SMOKE && isMain) app.exit(1) // 冒烟模式下加载失败必须以非零退出码暴露
  })
  mainWindow.webContents.on('did-finish-load', () => {
    console.log('[ripple] loaded:', mainWindow && mainWindow.webContents.getURL())
  })
  if (SMOKE) {
    // 兜底：15s 仍未 ready-to-show 退出（即视为挂死），以非零退出码暴露
    setTimeout(() => app.exit(1), 15000)
  }

  // 关窗=最小化到托盘（托盘「退出」/ Cmd+Q / before-quit 才真正退出）
  mainWindow.on('close', (event) => {
    if (!isQuitting && trayModule.hasTray()) {
      event.preventDefault()
      mainWindow.hide()
    }
  })
  mainWindow.on('focus', () => trayModule.onWindowFocused())

  loadApp()
}

function showMainWindow() {
  if (mainWindow && !mainWindow.isDestroyed()) {
    if (mainWindow.isMinimized()) mainWindow.restore()
    mainWindow.show()
    mainWindow.focus()
  }
}

function setApplicationMenu() {
  if (process.platform === 'darwin') {
    Menu.setApplicationMenu(
      Menu.buildFromTemplate([
        { role: 'appMenu' },
        { role: 'editMenu' },
        { role: 'viewMenu' },
        { role: 'windowMenu' }
      ])
    )
  } else {
    Menu.setApplicationMenu(null)
  }
}

// ---------------------------------------------------------------------------
// IPC 契约（与 preload.js 严格配对，spec §6.2）
// ---------------------------------------------------------------------------

function registerIpc() {
  ipcMain.handle('ripple:ws-connect', (_event, cfg) => {
    if (!cfg || typeof cfg !== 'object') return { ok: false, error: 'invalid payload' }
    return wsClient.connect(cfg.apiBase, cfg.token)
  })
  ipcMain.handle('ripple:ws-disconnect', () => {
    wsClient.disconnect()
    return { ok: true }
  })
  ipcMain.on('ripple:ws-send', (_event, payload) => {
    wsClient.send(payload)
  })
}

// ---------------------------------------------------------------------------
// 生命周期
// ---------------------------------------------------------------------------

app.whenReady().then(() => {
  if (!gotLock) return // 理论不可达：抢锁失败时早已 exit
  protocol.handle('ripple', handleRippleRequest)
  setApplicationMenu()
  createWindow()
  trayModule.initTray(() => mainWindow)
  wsClient = createWsClient({ onFrame: broadcastFrame })
  registerIpc()

  app.on('activate', () => showMainWindow()) // mac dock 图标点击
})

app.on('before-quit', () => {
  isQuitting = true
  if (wsClient) wsClient.disconnect()
  stopPowerSave()
})

app.on('window-all-closed', () => {
  app.quit()
})
