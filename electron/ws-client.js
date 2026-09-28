'use strict'

// 主进程原生 WebSocket 客户端（WS 下沉主进程，避免渲染层被系统节流断连）。
// - connect(apiBase, token)：apiBase 形如 http(s)://host[:port]（可带路径前缀），推导为 ws(s)://host/api/ws?token=
// - 指数退避重连：1s 起倍增，上限 30s；连上后重置
// - 客户端心跳：每 25s 发 {type:'ping'}（spec §4，桌面模式下由主进程代发）
// - 所有收到的服务端帧原样经 onFrame 抛给 main.js 广播；连接状态变化额外合成
//   {type:'ws.status', status:'connected'|'disconnected'|'reconnecting'} 帧（渲染层以此触发断线补拉）
//
// 说明：Electron 44 主进程自带全局 WebSocket（Node ≥22 内置，无需第三方依赖）。

const INITIAL_RETRY_MS = 1000
const MAX_RETRY_MS = 30000
const PING_INTERVAL_MS = 25000

function deriveWsUrl(apiBase, token) {
  let u
  try {
    u = new URL(String(apiBase || ''))
  } catch {
    return null
  }
  if (u.protocol !== 'http:' && u.protocol !== 'https:') return null
  u.protocol = u.protocol === 'https:' ? 'wss:' : 'ws:'
  const prefix = u.pathname.replace(/\/+$/, '') // apiBase 若带路径前缀则保留（默认为空）
  u.pathname = prefix + '/api/ws'
  u.search = 'token=' + encodeURIComponent(String(token || ''))
  return u.toString()
}

function createWsClient({ onFrame }) {
  let ws = null
  let active = false // 用户意图：已 connect 且未 disconnect
  let url = null
  let retryDelay = INITIAL_RETRY_MS
  let retryTimer = null
  let pingTimer = null

  function emit(frame) {
    if (typeof onFrame === 'function') onFrame(frame)
  }

  function setStatus(status) {
    emit({ type: 'ws.status', status })
  }

  function clearTimers() {
    if (retryTimer) {
      clearTimeout(retryTimer)
      retryTimer = null
    }
    if (pingTimer) {
      clearInterval(pingTimer)
      pingTimer = null
    }
  }

  function startPing() {
    stopPing()
    pingTimer = setInterval(() => {
      if (ws && ws.readyState === WebSocket.OPEN) {
        try {
          ws.send(JSON.stringify({ type: 'ping' }))
        } catch {
          /* 发送失败交给 onclose 走重连 */
        }
      }
    }, PING_INTERVAL_MS)
  }

  function stopPing() {
    if (pingTimer) {
      clearInterval(pingTimer)
      pingTimer = null
    }
  }

  function scheduleReconnect() {
    if (!active) return
    setStatus('reconnecting')
    const delay = retryDelay
    retryDelay = Math.min(retryDelay * 2, MAX_RETRY_MS)
    retryTimer = setTimeout(() => {
      if (active) open()
    }, delay)
  }

  function open() {
    if (!active || !url) return
    try {
      ws = new WebSocket(url)
    } catch {
      scheduleReconnect()
      return
    }
    ws.onopen = () => {
      retryDelay = INITIAL_RETRY_MS
      setStatus('connected')
      startPing()
    }
    ws.onmessage = (ev) => {
      let frame
      try {
        frame = JSON.parse(ev.data)
      } catch {
        return // 非 JSON 帧忽略
      }
      if (frame && typeof frame === 'object') emit(frame)
    }
    ws.onclose = () => {
      stopPing()
      ws = null
      scheduleReconnect() // 非用户主动断开即重连
    }
    ws.onerror = () => {
      /* 具体错误码不细分：统一由 onclose 触发退避重连 */
    }
  }

  // tear down 当前连接；emitStatus=true 表示用户主动断开，广播 disconnected
  function teardown(emitStatus) {
    clearTimers()
    if (ws) {
      const w = ws
      ws = null
      w.onopen = w.onmessage = w.onerror = w.onclose = null
      try {
        w.close()
      } catch {
        /* ignore */
      }
    }
    if (emitStatus) setStatus('disconnected')
  }

  return {
    connect(apiBase, token) {
      const u = deriveWsUrl(apiBase, token)
      if (!u) return { ok: false, error: 'invalid apiBase: ' + String(apiBase) }
      url = u
      active = true
      retryDelay = INITIAL_RETRY_MS
      teardown(false) // 换 token / 换服务器：先拆旧连接但不广播 disconnected
      open()
      return { ok: true, url }
    },
    disconnect() {
      active = false
      teardown(true)
    },
    send(payload) {
      if (ws && ws.readyState === WebSocket.OPEN) {
        try {
          ws.send(typeof payload === 'string' ? payload : JSON.stringify(payload))
        } catch {
          /* ignore */
        }
      }
    },
    isConnected() {
      return !!ws && ws.readyState === WebSocket.OPEN
    }
  }
}

module.exports = { createWsClient, deriveWsUrl }
