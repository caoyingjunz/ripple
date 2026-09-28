'use strict'

// 托盘 + 原生通知 + 未读角标 + 开机自启。
// 已知坑（design §8）：托盘实例必须全局引用持有，局部变量会被 GC 导致托盘消失，
// 故 tray 保存在模块级变量；main.js 也持有 initTray 的返回值。
//
// 首版已知取舍（后续迭代，见 docs/desktop-usage.md）：
// - 通知不做免打扰（muted）过滤：壳层不持有会话级 muted 状态，首版收到 message.new
//   且主窗失焦即通知；会话免打扰过滤留给渲染层通过 IPC 下发后再实现
// - 未读角标按「失焦期间收到 message.new 计数」，主窗获得焦点即清零
//   （真实未读数以渲染层 conversationsStore 为准，壳层计数是近似值）

const { app, Tray, Menu, Notification, nativeImage } = require('electron')
const path = require('node:path')

let tray = null
let unreadCount = 0

function iconPath() {
  return path.join(__dirname, '..', 'assets', 'icon.png')
}

// 托盘图标尺寸按平台取（mac 18px 状态栏 / win 32px），由 512px 源图缩放
function buildTrayIcon() {
  const size = process.platform === 'darwin' ? 18 : 32
  return nativeImage.createFromPath(iconPath()).resize({ width: size, height: size })
}

// main.js 注入的主窗引用（函数形式避免持有 stale 引用）
let mainWindowRef = null

function setBadge(count) {
  if (process.platform !== 'darwin' || !app.dock) return
  if (count > 0) {
    app.dock.setBadge(count >= 100 ? '99+' : String(count))
  } else {
    app.dock.setBadge('')
  }
}

function notify(title, body) {
  if (!Notification.isSupported()) return
  const n = new Notification({ title, body })
  n.on('click', () => {
    const w = mainWindowRef && mainWindowRef()
    if (w && !w.isDestroyed()) {
      w.show()
      w.focus()
    }
  })
  n.show()
}

function messagePreview(message) {
  if (!message) return ''
  if (message.type === 'image') return '[图片]'
  const body = typeof message.body === 'string' ? message.body : ''
  return body.length > 60 ? body.slice(0, 60) + '…' : body
}

function initTray(getMainWindow) {
  mainWindowRef = getMainWindow
  tray = new Tray(buildTrayIcon())
  tray.setToolTip('Ripple')

  function showMainWindow() {
    const w = mainWindowRef && mainWindowRef()
    if (w && !w.isDestroyed()) {
      w.show()
      w.focus()
    }
  }

  function quitApp() {
    app.quit()
  }

  function rebuildMenu() {
    const { openAtLogin } = app.getLoginItemSettings()
    const menu = Menu.buildFromTemplate([
      { label: '显示 Ripple', click: showMainWindow },
      { type: 'separator' },
      {
        label: '开机自启',
        type: 'checkbox',
        checked: openAtLogin,
        click: (item) => {
          app.setLoginItemSettings({ openAtLogin: item.checked })
          rebuildMenu()
        }
      },
      { type: 'separator' },
      { label: '退出', click: quitApp }
    ])
    tray.setContextMenu(menu)
  }
  rebuildMenu()

  tray.on('click', showMainWindow) // win/linux 左键点击直接唤起；mac 保留默认行为
  return tray
}

// 收到 message.new 且主窗失焦：未读角标 + 原生通知 + win 闪任务栏
function onMessageNew(frame, windowFocused) {
  const message = frame && frame.message
  if (!message || windowFocused) return
  unreadCount++
  setBadge(unreadCount)

  const w = mainWindowRef && mainWindowRef()
  if (process.platform === 'win32' && w && !w.isDestroyed()) {
    w.flashFrame(true)
    w.once('focus', () => {
      if (!w.isDestroyed()) w.flashFrame(false)
    })
  }
  notify('Ripple · 新消息', messagePreview(message))
}

// 主窗获得焦点：未读角标清零（近似口径，见文件头说明）
function onWindowFocused() {
  unreadCount = 0
  setBadge(0)
}

function hasTray() {
  return !!tray
}

module.exports = { initTray, onMessageNew, onWindowFocused, hasTray }
