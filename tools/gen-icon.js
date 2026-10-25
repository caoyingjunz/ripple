#!/usr/bin/env node
'use strict'

// 程序化生成 Ripple 图标（RGBA PNG，圆角方块，QQ 蓝调）。
// 零美术资源依赖：手写 PNG 编码（签名 + IHDR + IDAT + IEND，CRC32 自实现），
// 仅用 node 内置 zlib 压缩像素数据。electron-builder 以该 PNG 自动派生
// mac icns / win ico 各尺寸。
//
// 用法：node tools/gen-icon.js [--size N] [--radius-ratio R] [--out path]
//   无参数 = 512x512、圆角比例 96/512、输出 assets/icon.png（与历史版本逐字节一致）
//   radius-ratio 0 = 全出血方图（maskable / apple-touch 用）
//   渲染函数供 tools/gen-web-icons.js require 复用（见 module.exports）

const zlib = require('node:zlib')
const fs = require('node:fs')
const path = require('node:path')

const DEFAULT_SIZE = 512
const DEFAULT_RADIUS_RATIO = 96 / 512 // 0.1875，与桌面图标同形
const RGB = [30, 144, 255] // #1E90FF

// ---- CRC32（PNG 每个 chunk 要求）----
const CRC_TABLE = (() => {
  const t = new Uint32Array(256)
  for (let n = 0; n < 256; n++) {
    let c = n
    for (let k = 0; k < 8; k++) c = c & 1 ? 0xedb88320 ^ (c >>> 1) : c >>> 1
    t[n] = c >>> 0
  }
  return t
})()

function crc32(buf) {
  let c = 0xffffffff
  for (let i = 0; i < buf.length; i++) c = CRC_TABLE[(c ^ buf[i]) & 0xff] ^ (c >>> 8)
  return (c ^ 0xffffffff) >>> 0
}

function chunk(type, data) {
  const len = Buffer.alloc(4)
  len.writeUInt32BE(data.length, 0)
  const body = Buffer.concat([Buffer.from(type, 'ascii'), data])
  const crc = Buffer.alloc(4)
  crc.writeUInt32BE(crc32(body), 0)
  return Buffer.concat([len, body, crc])
}

// ---- 渲染：圆角方块 + 1px 边缘抗锯齿（SDF 一采样近似）----
// size: 边长（像素）；radiusRatio: 圆角半径比例，radius = round(size * radiusRatio)，0 = 全出血
function renderIconPng(options = {}) {
  const size = options.size == null ? DEFAULT_SIZE : options.size
  const radiusRatio = options.radiusRatio == null ? DEFAULT_RADIUS_RATIO : options.radiusRatio
  const rgb = options.rgb || RGB
  const radius = Math.round(size * radiusRatio)

  function roundRectSDF(px, py) {
    const half = size / 2
    const dx = Math.abs(px - half + 0.5) - (half - radius)
    const dy = Math.abs(py - half + 0.5) - (half - radius)
    const qx = Math.max(dx, 0)
    const qy = Math.max(dy, 0)
    return Math.min(Math.max(dx, dy), 0) + Math.sqrt(qx * qx + qy * qy) - radius
  }

  function buildRawPixels() {
    const raw = Buffer.alloc(size * (size * 4 + 1)) // 每行首字节 filter=0
    for (let y = 0; y < size; y++) {
      const rowStart = y * (size * 4 + 1)
      raw[rowStart] = 0
      for (let x = 0; x < size; x++) {
        const sd = roundRectSDF(x, y)
        let alpha
        if (sd <= -0.5) alpha = 255
        else if (sd >= 0.5) alpha = 0
        else alpha = Math.round((0.5 - sd) * 255)
        const off = rowStart + 1 + x * 4
        raw[off] = rgb[0]
        raw[off + 1] = rgb[1]
        raw[off + 2] = rgb[2]
        raw[off + 3] = alpha
      }
    }
    return raw
  }

  const signature = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])

  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(size, 0) // width
  ihdr.writeUInt32BE(size, 4) // height
  ihdr[8] = 8 // bit depth
  ihdr[9] = 6 // color type: RGBA
  ihdr[10] = 0 // compression: deflate
  ihdr[11] = 0 // filter: none
  ihdr[12] = 0 // interlace: none

  const idat = zlib.deflateSync(buildRawPixels(), { level: 9 })

  return Buffer.concat([
    signature,
    chunk('IHDR', ihdr),
    chunk('IDAT', idat),
    chunk('IEND', Buffer.alloc(0))
  ])
}

function parseArgs(argv) {
  const opts = { size: DEFAULT_SIZE, radiusRatio: DEFAULT_RADIUS_RATIO, out: null }
  for (let i = 0; i < argv.length; i++) {
    const arg = argv[i]
    if (arg === '--size') opts.size = Number(argv[++i])
    else if (arg === '--radius-ratio') opts.radiusRatio = Number(argv[++i])
    else if (arg === '--out') opts.out = argv[++i]
    else {
      console.error(`unknown argument: ${arg}`)
      console.error('usage: node tools/gen-icon.js [--size N] [--radius-ratio R] [--out path]')
      process.exit(1)
    }
  }
  if (!Number.isInteger(opts.size) || opts.size <= 0) {
    console.error('--size 必须是正整数')
    process.exit(1)
  }
  if (!Number.isFinite(opts.radiusRatio) || opts.radiusRatio < 0) {
    console.error('--radius-ratio 必须是非负数')
    process.exit(1)
  }
  if (opts.out != null && typeof opts.out !== 'string') {
    console.error('--out 需要路径参数')
    process.exit(1)
  }
  return opts
}

function main() {
  const opts = parseArgs(process.argv.slice(2))
  const png = renderIconPng({ size: opts.size, radiusRatio: opts.radiusRatio })
  const outPath = opts.out
    ? path.resolve(process.cwd(), opts.out)
    : path.join(__dirname, '..', 'assets', 'icon.png')
  fs.mkdirSync(path.dirname(outPath), { recursive: true })
  fs.writeFileSync(outPath, png)
  console.log('generated:', outPath, png.length, 'bytes')
}

if (require.main === module) main()

module.exports = { renderIconPng, DEFAULT_SIZE, DEFAULT_RADIUS_RATIO, RGB }
