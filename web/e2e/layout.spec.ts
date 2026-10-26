import fs from "node:fs/promises";
import path from "node:path";
import type { Page, TestInfo } from "@playwright/test";
import { expect, injectToken, test } from "./fixtures";

/**
 * §6.2 layout：逐页无横向溢出（scrollWidth ≤ innerWidth+1）、--app-vh 已注入、
 * 证据截图（document.fonts.ready 后）落 web/test-results/evidence/（不入库）；
 * PWA：manifest 与 4 个 icon 均 200，index.html 的 %BASE_URL% 已被 vite 替换。
 */

async function settle(page: Page): Promise<void> {
  await Promise.race([
    page.evaluate(() => document.fonts.ready.then(() => true)),
    page.waitForTimeout(3000),
  ]);
  await page.waitForTimeout(200); // 布局/字体回填后再量
}

interface Measure {
  scrollWidth: number;
  bodyScrollWidth: number;
  innerWidth: number;
  appVh: string;
}

async function measure(page: Page): Promise<Measure> {
  return page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    bodyScrollWidth: document.body.scrollWidth,
    innerWidth: window.innerWidth,
    appVh: getComputedStyle(document.documentElement).getPropertyValue("--app-vh").trim(),
  }));
}

async function shoot(page: Page, dir: string, name: string): Promise<void> {
  await fs.mkdir(dir, { recursive: true });
  await page.screenshot({ path: path.join(dir, `${name}.png`), fullPage: true });
}

/**
 * 证据目录固定锚定 playwright.config.ts 所在目录（web/），即 web/test-results/evidence/（已 gitignore）。
 * 不用 config.rootDir 推导：某些 CLI 调用方式下 rootDir 会指向 e2e/，导致截图落到未忽略的 e2e/test-results/。
 */
function evidenceDirOf(testInfo: TestInfo): string {
  const configDir = testInfo.config.configFile
    ? path.dirname(testInfo.config.configFile)
    : testInfo.config.rootDir;
  return path.join(configDir, "test-results", "evidence");
}

test.describe("布局与视口", () => {
  test("公开页：/login /register 无横向溢出 + --app-vh 注入 + 截图", async ({
    page,
  }, testInfo) => {
    const evidenceDir = evidenceDirOf(testInfo);
    const project = testInfo.project.name;

    for (const p of [
      { slug: "login", url: "/login", anchor: ".splash-panel" },
      { slug: "register", url: "/register", anchor: ".splash-panel" },
    ]) {
      await page.goto(p.url);
      await expect(page.locator(p.anchor)).toBeVisible();
      await settle(page);
      const m = await measure(page);
      expect.soft(m.scrollWidth, `${project} ${p.slug} documentElement 横向溢出`).toBeLessThanOrEqual(m.innerWidth + 1);
      expect.soft(m.bodyScrollWidth, `${project} ${p.slug} body 横向溢出`).toBeLessThanOrEqual(m.innerWidth + 1);
      expect.soft(m.appVh, `${project} ${p.slug} --app-vh 未注入`).toMatch(/^\d+(\.\d+)?px$/);
      await shoot(page, evidenceDir, `${project}-${p.slug}`);
    }
  });

  test("登录后页面：/ /chat /chat/:id /friends /profile /bottle 无横向溢出 + 截图", async ({
    page,
    context,
    userA,
    directConvId,
  }, testInfo) => {
    await injectToken(context, userA.token);
    const evidenceDir = evidenceDirOf(testInfo);
    const project = testInfo.project.name;

    const pages = [
      { slug: "home", url: "/", anchor: ".brand-hero" },
      { slug: "chat-list", url: "/chat", anchor: ".list" },
      { slug: "chat-room", url: `/chat/${directConvId}`, anchor: ".messages" },
      { slug: "friends", url: "/friends", anchor: ".searchbar" },
      { slug: "profile", url: "/profile", anchor: ".avatar-edit" },
      { slug: "bottle", url: "/bottle", anchor: ".sea-stage" },
    ];

    for (const p of pages) {
      await page.goto(p.url);
      await expect(page.locator(p.anchor)).toBeVisible();
      await settle(page);
      const m = await measure(page);
      expect.soft(m.scrollWidth, `${project} ${p.slug} documentElement 横向溢出`).toBeLessThanOrEqual(m.innerWidth + 1);
      expect.soft(m.bodyScrollWidth, `${project} ${p.slug} body 横向溢出`).toBeLessThanOrEqual(m.innerWidth + 1);
      expect.soft(m.appVh, `${project} ${p.slug} --app-vh 未注入`).toMatch(/^\d+(\.\d+)?px$/);

      if (p.slug === "chat-room") {
        // 聊天室高度消费 --app-vh（键盘适配的落点）：与当前可见高度一致（±20px）
        const roomH = await page
          .locator(".page.chat-room")
          .evaluate((el) => el.getBoundingClientRect().height);
        expect.soft(Math.abs(roomH - parseFloat(m.appVh)), `${project} chat-room 高度 ≠ --app-vh`).toBeLessThanOrEqual(20);
      }
      await shoot(page, evidenceDir, `${project}-${p.slug}`);
    }
  });

  test("PWA 资源：manifest + 4 图标 200，index.html 引用已替换 BASE_URL", async ({
    page,
    request,
  }) => {
    const assets = [
      "/manifest.webmanifest",
      "/icons/icon-192.png",
      "/icons/icon-512.png",
      "/icons/icon-512-maskable.png",
      "/icons/apple-touch-icon.png",
    ];
    for (const a of assets) {
      const res = await request.get(a);
      expect.soft(res.status(), `${a} 非 200`).toBe(200);
    }

    const man = (await (await request.get("/manifest.webmanifest")).json()) as {
      name?: string;
      display?: string;
      start_url?: string;
      icons?: unknown[];
    };
    expect.soft(man.name ?? "").toContain("Ripple");
    expect.soft(man.display).toBe("standalone");
    expect.soft(man.icons?.length ?? 0).toBeGreaterThanOrEqual(3);

    await page.goto("/login");
    const href = await page.locator('link[rel="manifest"]').getAttribute("href");
    expect(href ?? "", "manifest href 未被 vite 替换").not.toContain("%BASE_URL%");
    expect(href ?? "").toContain("manifest.webmanifest");
    const apple = await page.locator('link[rel="apple-touch-icon"]').getAttribute("href");
    expect(apple ?? "", "apple-touch-icon href 未被 vite 替换").not.toContain("%BASE_URL%");
  });
});
