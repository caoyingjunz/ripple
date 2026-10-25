import {
  test as base,
  expect,
  type APIRequestContext,
  type BrowserContext,
  type BrowserContextOptions,
  type Page,
} from "@playwright/test";

/**
 * E2E 公共夹具（S6）。
 * 数据不依赖 seed 用户：每个用例现注册 e2e_a_/e2e_b_ 随机用户（幂等），
 * 直连 8080 建单聊；登录态可用 addInitScript 注入 ripple_token 跳过 UI 登录。
 */

export const API_BASE = "http://127.0.0.1:8080";
export const PASSWORD = "e2e_pass_123"; // 后端规则：密码 ≥ 6 位
const TOKEN_KEY = "ripple_token"; // 见 web/src/api/client.ts

export interface E2EUser {
  username: string;
  displayName: string;
  token: string;
  id: string;
}

let seq = 0;
function rand(): string {
  seq += 1;
  return `${Date.now().toString(36)}${seq.toString(36)}${Math.random().toString(36).slice(2, 6)}`;
}

// ---- API 直连 ----

/** 注册用户（用户名规则 ^[a-z0-9_]{3,32}$）。 */
export async function registerUser(
  request: APIRequestContext,
  prefix: "e2e_a_" | "e2e_b_",
): Promise<E2EUser> {
  const username = `${prefix}${rand()}`.slice(0, 32);
  const res = await request.post(`${API_BASE}/api/auth/register`, {
    data: { username, password: PASSWORD, display_name: `E2E ${username}` },
  });
  expect(res.status(), `register ${username} -> ${res.status()}`).toBe(201);
  const body = (await res.json()) as { token: string; user: { id: string; username: string } };
  return { username, displayName: body.user.username, token: body.token, id: body.user.id };
}

/** token 直连登录（seed 用户可用：alice…erin / demo123）。 */
export async function loginApi(
  request: APIRequestContext,
  username: string,
  password: string,
): Promise<E2EUser> {
  const res = await request.post(`${API_BASE}/api/auth/login`, {
    data: { username, password },
  });
  expect(res.status(), `login ${username} -> ${res.status()}`).toBe(200);
  const body = (await res.json()) as { token: string; user: { id: string; username: string } };
  return { username, displayName: body.user.username, token: body.token, id: body.user.id };
}

/** A 与 B 建单聊（无需好友关系），返回 conversation_id。 */
export async function createSingleConv(
  request: APIRequestContext,
  as: E2EUser,
  peerId: string,
): Promise<string> {
  const res = await request.post(`${API_BASE}/api/conversations`, {
    headers: { Authorization: `Bearer ${as.token}` },
    data: { type: "single", user_id: peerId },
  });
  expect(res.status(), `create conversation -> ${res.status()}`).toBe(201);
  const body = (await res.json()) as { id: string };
  return body.id;
}

/** 经 REST 发一条文字消息（用于“对端发消息”场景）。 */
export async function sendTextViaApi(
  request: APIRequestContext,
  as: E2EUser,
  convId: string,
  body: string,
): Promise<void> {
  const res = await request.post(`${API_BASE}/api/conversations/${convId}/messages`, {
    headers: { Authorization: `Bearer ${as.token}` },
    data: { type: "text", body },
  });
  expect(res.status(), `send message -> ${res.status()}`).toBe(201);
}

/** 灌历史消息（分批并发，避免 60 次串行等待）。 */
export async function seedHistory(
  request: APIRequestContext,
  as: E2EUser,
  convId: string,
  count: number,
  prefix: string,
): Promise<void> {
  const batch = 10;
  for (let i = 0; i < count; i += batch) {
    await Promise.all(
      Array.from({ length: Math.min(batch, count - i) }, (_, k) =>
        sendTextViaApi(request, as, convId, `${prefix}${i + k + 1}`),
      ),
    );
  }
}

// ---- 浏览器侧辅助 ----

/** 预置登录态（localStorage 键名 ripple_token）。 */
export async function injectToken(context: BrowserContext, token: string): Promise<void> {
  await context.addInitScript(
    ({ key, value }) => {
      try {
        localStorage.setItem(key, value);
      } catch {
        /* ignore */
      }
    },
    { key: TOKEN_KEY, value: token },
  );
}

/** UI 登录：填表 → 进入 → 等首页。 */
export async function loginViaUi(page: Page, user: Pick<E2EUser, "username">): Promise<void> {
  await page.goto("/login");
  await page.getByLabel("用户名").fill(user.username);
  await page.getByLabel("密码").fill(PASSWORD);
  await page.getByRole("button", { name: "进入" }).click();
  await page.waitForURL((u) => u.pathname === "/");
}

/** 1×1 有效 PNG（内存构造，不落盘）。 */
export function pngBuffer(): Buffer {
  return Buffer.from(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==",
    "base64",
  );
}

/** 打开聊天室并等待消息区渲染完成。 */
export async function openRoom(page: Page, convId: string): Promise<void> {
  await page.goto(`/chat/${convId}`);
  await expect(page.locator(".messages")).toBeVisible();
}

// ---- 测试夹具 ----

export interface ExtraContextHandle {
  context: BrowserContext;
  page: Page;
}

type Fixtures = {
  userA: E2EUser;
  userB: E2EUser;
  directConvId: string;
  /** 按当前 project 的 use 配置（viewport/isMobile/hasTouch/dsf）新建附加上下文，用于双端场景。 */
  newProjectContext: () => Promise<ExtraContextHandle>;
};

export const test = base.extend<Fixtures>({
  userA: async ({ request }, use) => {
    await use(await registerUser(request, "e2e_a_"));
  },
  userB: async ({ request }, use) => {
    await use(await registerUser(request, "e2e_b_"));
  },
  directConvId: async ({ request, userA, userB }, use) => {
    await use(await createSingleConv(request, userA, userB.id));
  },
  newProjectContext: async ({ browser }, use, testInfo) => {
    const useOpts = testInfo.project.use as BrowserContextOptions;
    const open: BrowserContext[] = [];
    await use(async () => {
      const context = await browser.newContext({
        viewport: useOpts.viewport,
        deviceScaleFactor: useOpts.deviceScaleFactor,
        isMobile: useOpts.isMobile,
        hasTouch: useOpts.hasTouch,
        baseURL: useOpts.baseURL,
      });
      open.push(context);
      return { context, page: await context.newPage() };
    });
    await Promise.all(open.map((c) => c.close().catch(() => undefined)));
  },
});

export { expect };
