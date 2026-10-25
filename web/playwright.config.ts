import { defineConfig } from "@playwright/test";

/**
 * Ripple 移动端 H5 E2E 配置（S6）。
 *
 * - 浏览器：系统 Chrome（channel:"chrome"），不下载 Playwright 自带浏览器。
 * - 视口：双移动 project（390 / 360，isMobile + hasTouch）+ 桌面回归 project。
 * - ws 服务器：go server 已在 127.0.0.1:8080 常驻时可复用（reuseExistingServer）；
 *   就绪探测 /api/conversations 无 token 返回 401（Playwright 认可的 ready 码）。
 * - vite 必须显式 --host 127.0.0.1：本机 Node 将 localhost 解析到 ::1，否则 127.0.0.1 拒连。
 * - workers:1 / fullyParallel:false：E2E 走 dev 库 ripple，串行避免相互干扰（go test 走 ripple_test）。
 */
export default defineConfig({
  testDir: "./e2e",
  workers: 1,
  fullyParallel: false,
  timeout: 30_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: "http://127.0.0.1:5173",
    channel: "chrome",
    trace: "on-first-retry",
    screenshot: "only-on-failure",
  },
  projects: [
    {
      name: "mobile-390",
      use: {
        viewport: { width: 390, height: 844 },
        deviceScaleFactor: 3,
        isMobile: true,
        hasTouch: true,
      },
    },
    {
      name: "mobile-360",
      use: {
        viewport: { width: 360, height: 800 },
        deviceScaleFactor: 2,
        isMobile: true,
        hasTouch: true,
      },
    },
    {
      name: "desktop-1280",
      use: {
        viewport: { width: 1280, height: 800 },
        hasTouch: false,
      },
    },
  ],
  webServer: [
    {
      command: "go run ./cmd/server",
      cwd: "../server",
      url: "http://127.0.0.1:8080/api/conversations",
      reuseExistingServer: true,
      timeout: 120_000,
    },
    {
      command: "npm run dev -- --host 127.0.0.1 --port 5173",
      cwd: ".",
      url: "http://127.0.0.1:5173",
      reuseExistingServer: true,
      timeout: 60_000,
    },
  ],
});
