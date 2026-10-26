import type { WebSocketRoute } from "@playwright/test";
import { expect, injectToken, openRoom, sendTextViaApi, test } from "./fixtures";

/**
 * §6.2 realtime：
 * 1) 双上下文 WS 实时：A 发 B 见（不刷新）。
 * 2) 断线补拉：context.routeWebSocket 先 drop 再 pass（退避 1s→2s 档位，避开 8s），
 *    恢复后 A 不刷新即见 B 期间经 REST 发的消息（after_seq 增量补齐）。
 *    spike 已验证：断线窗口内消息不泄漏（真断），恢复后 ~3.3s 内补齐。
 * 3) [test.fixme] 空会话断线补拉：既有产品缺陷，当前必失败，详见该用例注释。
 */
test.describe("实时通道", () => {
  test("双端 WS 实时：A 发文字，B 不刷新即见", async ({
    page,
    context,
    userA,
    userB,
    directConvId,
    newProjectContext,
  }) => {
    await injectToken(context, userA.token);
    const b = await newProjectContext();
    await injectToken(b.context, userB.token);

    await openRoom(b.page, directConvId);
    await openRoom(page, directConvId);
    await expect(page.locator(".conn-dot.on")).toBeVisible({ timeout: 15_000 });
    await expect(b.page.locator(".conn-dot.on")).toBeVisible({ timeout: 15_000 });

    const body = `rt-${Date.now().toString(36)}`;
    await page.getByPlaceholder("说点什么…").fill(body);
    await page.getByRole("button", { name: "发送" }).click();
    await expect(page.locator(".bubble.mine", { hasText: body })).toBeVisible();

    // B 未刷新、未轮询：靠 message.new 帧实时上屏
    await expect(b.page.locator(".messages")).toContainText(body, { timeout: 10_000 });
  });

  test("断线补拉：重连后 A 不刷新即见 B 经 REST 发的消息", async ({
    page,
    context,
    request,
    userA,
    userB,
    directConvId,
  }) => {
    await injectToken(context, userA.token);

    let dropMode = false;
    let live: WebSocketRoute | null = null;
    await context.routeWebSocket(/\/api\/ws/, (ws) => {
      if (dropMode) {
        void ws.close(); // 断线窗口：拒连（page 侧立即关闭，模拟不可达）
        return;
      }
      ws.connectToServer(); // 默认自动双向转发
      live = ws;
    });

    // 锚点消息：store 的 catchUpAll 需要 after_seq 锚点（空会话会被跳过）
    await sendTextViaApi(request, userA, directConvId, "rt-anchor");

    await openRoom(page, directConvId);
    await expect(page.locator(".messages")).toContainText("rt-anchor");
    await expect(page.locator(".conn-dot.on")).toBeVisible({ timeout: 15_000 });
    await page.evaluate(() => {
      (window as unknown as { __e2eNoReload?: number }).__e2eNoReload = 1;
    });

    // 断线（关闭当前两侧连接；drop 窗口内新连接一律被拒）
    dropMode = true;
    await live?.close();
    await expect(page.locator(".conn-dot.on")).toBeHidden({ timeout: 10_000 });

    // 断线窗口内 B 经 REST 发消息：只落库，A 不可能实时收到
    const body = `offline-${Date.now().toString(36)}`;
    await sendTextViaApi(request, userB, directConvId, body);

    // 真断线校验：窗口内消息不得出现（若泄漏说明“断线”是假的）
    await page.waitForTimeout(1500);
    await expect(page.locator(".messages")).not.toContainText(body);

    // 恢复：drop 窗口 2–3s，避开 8s 退避档；下一次重连 ~1–2s 后成功
    await page.waitForTimeout(1000);
    dropMode = false;

    await expect(page.locator(".messages")).toContainText(body, { timeout: 25_000 });
    await expect(page.locator(".conn-dot.on")).toBeVisible();
    // 全程未整页刷新
    expect(
      await page.evaluate(
        () => (window as unknown as { __e2eNoReload?: number }).__e2eNoReload === 1,
      ),
    ).toBe(true);
  });

  /**
   * 已知缺陷（既有产品缺陷，非本次 E2E 改动引入）：空会话断线重连后不补拉。
   *
   * 现象：A 打开一个尚无任何消息的单聊（无 after_seq 锚点）→ 断网（WS drop）期间 B 经 REST 发首条消息
   *       → A 网络恢复、WS 重连成功（.conn-dot.on 恢复）→ 该消息始终不出现；刷新或重新进入该会话后可见
   *       （证明消息已落库，属前端补拉遗漏）。本地复现 2/2。
   * 根因：web/src/stores/conversations.ts 的 catchUpAll() 以已加载消息的最大 seq 作为 after_seq，
   *       对“已加载消息中没有真实条目”的会话 `if (!seqs.length) continue` 直接跳过；
   *       空会话因此永远不参与重连补拉。该文件在本分支未被本次改动触碰 → 既有缺陷。
   * 最小复现：即本用例步骤（routeWebSocket 先 drop 再 pass；会话不预置任何锚点消息）。
   * 建议修复方向（仅建议，未改任何代码）：
   *   - catchUpAll 对“当前已打开、但无已加载消息”的会话以 after_seq=0 调 fetchMessages；或
   *   - WS 重连成功（bridge.ts scheduleRefresh 路径）后对当前打开会话强制刷新一次。
   * 修复后删除 test.fixme 即可转为常规回归用例。
   */
  test.fixme("断线补拉（空会话）：重连后应补齐 B 在断线期间发的首条消息", async ({
    page,
    context,
    request,
    userA,
    userB,
    directConvId,
  }) => {
    await injectToken(context, userA.token);

    let dropMode = false;
    let live: WebSocketRoute | null = null;
    await context.routeWebSocket(/\/api\/ws/, (ws) => {
      if (dropMode) {
        void ws.close();
        return;
      }
      ws.connectToServer();
      live = ws;
    });

    // 空会话：不预置锚点消息（缺陷触发条件）
    await openRoom(page, directConvId);
    await expect(page.locator(".bubble")).toHaveCount(0);
    await expect(page.locator(".conn-dot.on")).toBeVisible({ timeout: 15_000 });

    dropMode = true;
    await live?.close();
    await expect(page.locator(".conn-dot.on")).toBeHidden({ timeout: 10_000 });

    const body = `offline-empty-${Date.now().toString(36)}`;
    await sendTextViaApi(request, userB, directConvId, body);

    await page.waitForTimeout(1500);
    await expect(page.locator(".messages")).not.toContainText(body);

    dropMode = false;
    // 期望行为（修复后应通过）：重连补拉把断线期间的首条消息补上；当前缺陷下此断言不成立。
    await expect(page.locator(".messages")).toContainText(body, { timeout: 25_000 });
  });
});
