#!/usr/bin/env node
'use strict'

// 程序化生成 Ripple 桌面壳图标（512x512 RGBA PNG，圆角方块，QQ 蓝调）。
// 零美术资源依赖：手写 PNG 编码（签名 + IHDR + IDAT + IEND，CRC32 自实现），
// 仅用 node 内置 zlib 压缩像素数据。electron-builder 以该 PNG 自动派生
// mac icns / win ico 各尺寸。
//
// 用法：node tools/gen-icon.js  （输出 assets/icon.png）

const zlib = require('node:zlib')
const fs = require('node:fs')
const path = require('node:path')

const SIZE = 512
const RADIUS = 96
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

// ---- 像素生成：圆角方块 + 1px 边缘抗锯齿（SDF 一采样近似）----
function roundRectSDF(px, py) {
  const half = SIZE / 2
  const dx = Math.abs(px - half + 0.5) - (half - RADIUS)
  const dy = Math.abs(py - half + 0.5) - (half - RADIUS)
  const qx = Math.max(dx, 0)
  const qy = Math.max(dy, 0)
  return Math.min(Math.max(dx, dy), 0) + Math.sqrt(qx * qx + qy * qy) - RADIUS
}

function buildRawPixels() {
  const raw = Buffer.alloc(SIZE * (SIZE * 4 + 1)) // 每行首字节 filter=0
  for (let y = 0; y < SIZE; y++) {
    const rowStart = y * (SIZE * 4 + 1)
    raw[rowStart] = 0
    for (let x = 0; x < SIZE; x++) {
      const sd = roundRectSDF(x, y)
      let alpha
      if (sd <= -0.5) alpha = 255
      else if (sd >= 0.5) alpha = 0
      else alpha = Math.round((0.5 - sd) * 255)
      const off = rowStart + 1 + x * 4
      raw[off] = RGB[0]
      raw[off + 1] = RGB[1]
      raw[off + 2] = RGB[2]
      raw[off + 3] = alpha
    }
  }
  return raw
}

function main() {
  const signature = Buffer.from([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])

  const ihdr = Buffer.alloc(13)
  ihdr.writeUInt32BE(SIZE, 0) // width
  ihdr.writeUInt32BE(SIZE, 4) // height
  ihdr[8] = 8 // bit depth
  ihdr[9] = 6 // color type: RGBA
  ihdr[10] = 0 // compression: deflate
  ihdr[11] = 0 // filter: none
  ihdr[12] = 0 // interlace: none

  const idat = zlib.deflateSync(buildRawPixels(), { level: 9 })

  const png = Buffer.concat([
    signature,
    chunk('IHDR', ihdr),
    chunk('IDAT', idat),
    chunk('IEND', Buffer.alloc(0))
  ])

  const outDir = path.join(__dirname, '..', 'assets')
  fs.mkdirSync(outDir, { recursive: true })
  const outPath = path.join(outDir, 'icon.png')
  fs.writeFileSync(outPath, png)
  console.log('generated:', outPath, png.length, 'bytes')
}

main()
