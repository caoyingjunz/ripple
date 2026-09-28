'use strict'

// preload（spec §6.2）：通过 contextBridge 暴露 window.ripple。
// ⚠️ 此对象形状是前后端共享契约（implementation-spec §6.2），不得变更：
//   { isDesktop: true, platform, ws: { connect(apiBase, token), disconnect(),
//      send(payload), onEvent(cb) → 取消订阅函数 } }

const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('ripple', {
  isDesktop: true,
  platform: process.platform, // 'darwin' | 'win32' | ...
  ws: {
    connect: (apiBase, token) => ipcRenderer.invoke('ripple:ws-connect', { apiBase, token }),
    disconnect: () => ipcRenderer.invoke('ripple:ws-disconnect'),
    send: (payload) => ipcRenderer.send('ripple:ws-send', payload),
    onEvent: (cb) => {
      if (typeof cb !== 'function') return () => {}
      const listener = (_event, frame) => cb(frame)
      ipcRenderer.on('ripple:ws-event', listener)
      return () => ipcRenderer.removeListener('ripple:ws-event', listener)
    }
  }
})
