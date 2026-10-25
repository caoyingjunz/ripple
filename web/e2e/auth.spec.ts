import { expect, loginViaUi, test } from "./fixtures";

/**
 * §6.2 auth：UI 登录 → 首页三卡片；未登录访问 /chat 重定向 /login。
 */
test.describe("鉴权与首页", () => {
  test("UI 登录 → 首页 hero + 三入口卡片", async ({ page, userA }) => {
    await loginViaUi(page, userA);

    await expect(page.locator(".brand-hero")).toHaveText("Ripple");
    await expect(page.locator(".tri .portal")).toHaveCount(3);
    await expect(page.locator(".tri .portal-title")).toHaveText(["聊天", "好友", "漂流瓶"]);
  });

  test("未登录访问 /chat 重定向 /login", async ({ page }) => {
    await page.goto("/chat");
    await page.waitForURL((u) => u.pathname === "/login");
    await expect(page.locator(".splash-panel")).toBeVisible();
    await expect(page.getByRole("button", { name: "进入" })).toBeVisible();
    // 未渲染任何聊天列表内容
    await expect(page.locator(".list-row")).toHaveCount(0);
  });
});
