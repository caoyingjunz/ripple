#!/usr/bin/env node
'use strict'

// 批量生成 web/public/icons 下的 PWA 图标（复用 tools/gen-icon.js 的渲染器）。
// 用法：npm run gen:icons
//
// 规格（见实施计划 §5.4）：
//   icon-192 / icon-512            —— 桌面同款圆角（radius-ratio 0.1875）
//   icon-512-maskable              —— 全出血方图（radius-ratio 0），满足 Android 遮罩可裁剪区
//   apple-touch-icon (180x180)     —— 全出血不透明方图，防 iOS web clip 黑角

const fs = require('node:fs')
const path = require('node:path')
const { renderIconPng, DEFAULT_RADIUS_RATIO } = require('./gen-icon.js')

const OUT_DIR = path.join(__dirname, '..', 'web', 'public', 'icons')

const TARGETS = [
  { file: 'icon-192.png', size: 192, radiusRatio: DEFAULT_RADIUS_RATIO },
  { file: 'icon-512.png', size: 512, radiusRatio: DEFAULT_RADIUS_RATIO },
  { file: 'icon-512-maskable.png', size: 512, radiusRatio: 0 },
  { file: 'apple-touch-icon.png', size: 180, radiusRatio: 0 }
]

fs.mkdirSync(OUT_DIR, { recursive: true })
for (const target of TARGETS) {
  const png = renderIconPng({ size: target.size, radiusRatio: target.radiusRatio })
  const outPath = path.join(OUT_DIR, target.file)
  fs.writeFileSync(outPath, png)
  console.log('generated:', outPath, png.length, 'bytes')
}
