import type { Page } from "@playwright/test";
import {
  expect,
  injectToken,
  loginViaUi,
  openRoom,
  pngBuffer,
  seedHistory,
  test,
} from "./fixtures";

/**
 * §6.2 chat：核心聊天链路（三个 project 都跑，desktop-1280 验证桌面零回归）。
 * 发送走 WS（连接态），撤回仅 2 分钟窗口内可达。
 */

function uniq(prefix: string): string {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 6)}`;
}

async function sendChatText(page: Page, body: string): Promise<void> {
  await page.getByPlaceholder("说点什么…").fill(body);
  await page.getByRole("button", { name: "发送" }).click();
}

test.describe("核心聊天链路", () => {
  test("UI 登录 → 列表见对端 → 进聊天室 → 发文字（乐观气泡 → ack 收口）", async ({
    page,
    userA,
    userB,
    directConvId,
  }) => {
    await loginViaUi(page, userA);

    // 会话列表：能看到与 B 的单聊（显示对端 display_name）
    await page.goto("/chat");
    const row = page.locator(".list-row", { hasText: userB.displayName });
    await expect(row).toBeVisible();
    await row.click();

    await page.waitForURL((u) => u.pathname === `/chat/${directConvId}`);
    await expect(page.locator(".messages")).toBeVisible();

    const body = uniq("chat-text");
    await sendChatText(page, body);

    // 乐观气泡立即出现（pending 态由「发送中…」+ 无 .bubble-time 标记）
    const bubble = page.locator(".bubble.mine", { hasText: body });
    await expect(bubble).toBeVisible();

    // WS ack 收口：pending 气泡被真实消息替换，出现时间戳
    await expect(page.locator(".bubble.mine.pending")).toHaveCount(0);
    await expect(bubble.locator(".bubble-time")).toBeVisible();
  });

  test("图片消息：filechooser → /uploads 经代理渲染真实图片", async ({
    page,
    context,
    userA,
    directConvId,
  }) => {
    await injectToken(context, userA.token);
    await openRoom(page, directConvId);

    const chooserPromise = page.waitForEvent("filechooser");
    await page.locator(".file-btn").click();
    const chooser = await chooserPromise;
    await chooser.setFiles({ name: "e2e.png", mimeType: "image/png", buffer: pngBuffer() });

    const img = page.locator(".bubble.mine .chat-img").last();
    await expect(img).toBeVisible();
    // naturalWidth > 0 证明图片真实解码成功（/uploads 经 vite proxy 可达）
    await expect
      .poll(() => img.evaluate((el: HTMLImageElement) => el.naturalWidth), { timeout: 15_000 })
      .toBeGreaterThan(0);
    // 图片消息同样完成 ack（pending 收口）
    await expect(page.locator(".bubble.mine.pending")).toHaveCount(0);
  });

  test("历史翻页：滚到顶出「加载更早消息…」→ 补齐更早消息且位置不回跳", async ({
    page,
    context,
    request,
    userA,
    directConvId,
  }) => {
    // 本用例灌 60 条历史，单条 POST 在 peng 远端库上约 0.4–1.2s（多轮往返 + 会话行锁串行），
    // 30s 全局预算不够 → 单独放宽到 150s（其余用例维持 config 的 30s）
    test.setTimeout(150_000);
    await seedHistory(request, userA, directConvId, 60, "hist-");
    await injectToken(context, userA.token);
    await openRoom(page, directConvId);

    // 首屏 limit=50
    await expect(page.locator(".bubble")).toHaveCount(50);

    // MutationObserver 捕获瞬时的「加载更早消息…」提示（网络快时 DOM 存在极短）
    await page.evaluate(() => {
      (window as unknown as { __sawOlder?: boolean }).__sawOlder = false;
      const root = document.querySelector(".messages");
      if (!root) return;
      new MutationObserver(() => {
        if (root.textContent?.includes("加载更早消息")) {
          (window as unknown as { __sawOlder?: boolean }).__sawOlder = true;
        }
      }).observe(root, { childList: true, subtree: true, characterData: true });
    });

    const messages = page.locator(".messages");
    await messages.evaluate((el) => {
      el.scrollTop = 0; // 触发 onScroll 翻页分支
    });

    await expect(page.locator(".bubble")).toHaveCount(60, { timeout: 15_000 });
    await expect
      .poll(() => page.evaluate(() => (window as unknown as { __sawOlder?: boolean }).__sawOlder))
      .toBe(true);

    // 位置不跳顶：等翻页收口（restore 会再触发一次 scroll 事件，可能带出第二次空 loadOlder），
    // 收口后轮询 scrollTop —— 正确终值 = 新增的 10 条消息高度（远大于 0，远小于“跳到底”）
    await expect(page.locator(".sysmsg", { hasText: "加载更早消息…" })).toHaveCount(0);
    await expect
      .poll(() => messages.evaluate((el) => el.scrollTop), { timeout: 15_000 })
      .toBeGreaterThan(0);
    const state = await messages.evaluate((el) => ({
      top: el.scrollTop,
      height: el.scrollHeight,
      client: el.clientHeight,
    }));
    // 未跳到最底（贴底会失去“阅读更早消息”的位置感）
    expect(state.top).toBeLessThan(state.height - state.client);

    // 排序正确：首条来自最早一批（hist-1..10，并发灌数据故批内 seq 乱序），末条来自最后一批
    const firstText = (await page.locator(".bubble").first().textContent()) ?? "";
    const firstNo = Number(/hist-(\d+)/.exec(firstText)?.[1]);
    expect(firstNo, `首条气泡 = ${firstText}`).toBeGreaterThanOrEqual(1);
    expect(firstNo, `首条气泡 = ${firstText}`).toBeLessThanOrEqual(10);
    const lastText = (await page.locator(".bubble").last().textContent()) ?? "";
    const lastNo = Number(/hist-(\d+)/.exec(lastText)?.[1]);
    expect(lastNo, `末条气泡 = ${lastText}`).toBeGreaterThanOrEqual(51);
  });

  test("撤回：触屏点按气泡出「撤回」（桌面点按不出动作，零回归）", async ({
    page,
    context,
    userA,
    directConvId,
  }) => {
    await injectToken(context, userA.token);
    await openRoom(page, directConvId);

    const body = uniq("revoke");
    await sendChatText(page, body);
    const bubble = page.locator(".bubble.mine", { hasText: body });
    await expect(bubble).toBeVisible();
    await expect(page.locator(".bubble.mine.pending")).toHaveCount(0);

    const coarse = await page.evaluate(() => matchMedia("(hover: none)").matches);
    if (coarse) {
      // 触屏路径：点按自己气泡 → .show-actions → 撤回按钮可见可点
      await bubble.tap();
      await expect(page.locator(".bubble.mine.show-actions")).toHaveCount(1);
      const btn = bubble.locator(".revoke-btn");
      await expect(btn).toBeVisible();
      // opacity 有 .15s 过渡，需轮询到终值
      await expect(btn).toHaveCSS("opacity", "1");

      await btn.click();
      // 自己与对端均显示「已撤回」
      await expect(page.locator(".sysmsg", { hasText: "已撤回" })).toHaveCount(1);
      await expect(bubble).toHaveCount(0);
    } else {
      // 桌面路径：点按不叠加 show-actions（行为零变化），hover 旧路径仍可用
      await bubble.click();
      await expect(page.locator(".bubble.mine.show-actions")).toHaveCount(0);
      await bubble.hover();
      await expect(bubble.locator(".revoke-btn")).toHaveCSS("opacity", "1");
    }
  });
});
