/**
 * H5 界面截图脚本（QQ 式改版验证）：以 iPhone 视口访问 :8080
 * 先用 bob 给 alice 发一条消息制造未读角标，再截消息/联系人页
 */
import { chromium } from "playwright";
import { mkdirSync } from "node:fs";

const BASE = "http://localhost:8080";
const OUT = new URL("./tmp-shots/", import.meta.url).pathname;
mkdirSync(OUT, { recursive: true });

async function apiLogin(username, password) {
  const r = await fetch(`${BASE}/api/auth/login`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
  });
  const data = await r.json();
  if (!r.ok) throw new Error(`login ${username}: ${JSON.stringify(data)}`);
  return data.token;
}

// bob → alice 发消息，制造未读角标
const bobToken = await apiLogin("bob", "demo123");
const sr = await fetch(`${BASE}/api/users/search?q=alice`, {
  headers: { Authorization: `Bearer ${bobToken}` },
});
const { users } = await sr.json();
const alice = users.find((u) => u.user.username === "alice").user;
const cr = await fetch(`${BASE}/api/conversations`, {
  method: "POST",
  headers: { Authorization: `Bearer ${bobToken}`, "Content-Type": "application/json" },
  body: JSON.stringify({ type: "single", user_id: alice.id }),
});
const conv = await cr.json();
if (!cr.ok) throw new Error(`createSingle: ${JSON.stringify(conv)}`);
const mr = await fetch(`${BASE}/api/conversations/${conv.id}/messages`, {
  method: "POST",
  headers: { Authorization: `Bearer ${bobToken}`, "Content-Type": "application/json" },
  body: JSON.stringify({ type: "text", body: "QQ 式改版预览：头像右上角应该有未读角标了" }),
});
if (!mr.ok) throw new Error(`sendMessage: ${mr.status}`);
console.log("seeded unread from bob");

const browser = await chromium.launch({ channel: "chrome" });
const ctx = await browser.newContext({
  viewport: { width: 390, height: 844 },
  deviceScaleFactor: 2,
  isMobile: true,
  hasTouch: true,
  locale: "zh-CN",
});
const page = await ctx.newPage();

await page.goto(`${BASE}/login`, { waitUntil: "networkidle" });
await page.getByLabel("用户名").fill("alice");
await page.getByLabel("密码").fill("demo123");
await page.getByRole("button", { name: "进入" }).click();
await page.waitForURL(`${BASE}/`, { timeout: 15000 });

// 消息页（QQ 式：顶栏头像+加号、头像未读角标、底部导航）
await page.goto(`${BASE}/chat`, { waitUntil: "networkidle" });
await page.waitForTimeout(1000);
await page.screenshot({ path: `${OUT}6-chat-list-qq.png` });

// 联系人页（QQ 式：联系人标题、新朋友/黑名单入口、在线状态）
await page.goto(`${BASE}/friends`, { waitUntil: "networkidle" });
await page.waitForTimeout(800);
await page.screenshot({ path: `${OUT}7-friends-qq.png` });

// 其余页面统一 QQ 顶栏验证
const aliceToken = await apiLogin("alice", "demo123");
const ah = { Authorization: `Bearer ${aliceToken}` };
const lr = await fetch(`${BASE}/api/conversations`, { headers: ah });
const convList = (await lr.json()).conversations;
const convId = convList[0]?.id;

await page.goto(`${BASE}/`, { waitUntil: "networkidle" });
await page.waitForTimeout(600);
await page.screenshot({ path: `${OUT}9-home-qq.png` });

await page.goto(`${BASE}/profile`, { waitUntil: "networkidle" });
await page.waitForTimeout(500);
await page.screenshot({ path: `${OUT}10-profile-qq.png` });

await page.goto(`${BASE}/group/create`, { waitUntil: "networkidle" });
await page.waitForTimeout(500);
await page.screenshot({ path: `${OUT}11-group-create-qq.png` });

if (convId) {
  await page.goto(`${BASE}/chat/${convId}`, { waitUntil: "networkidle" });
  await page.waitForTimeout(1200);
  await page.screenshot({ path: `${OUT}12-chat-room-qq.png` });
  await page.goto(`${BASE}/conversation/${convId}/info`, { waitUntil: "networkidle" });
  await page.waitForTimeout(600);
  await page.screenshot({ path: `${OUT}13-conv-info-qq.png` });
}

await page.goto(`${BASE}/bottle`, { waitUntil: "networkidle" });
await page.waitForTimeout(800);
await page.screenshot({ path: `${OUT}14-bottle-sea-qq.png` });

try {
  const tr = await fetch(`${BASE}/api/bottles`, {
    method: "POST",
    headers: { ...ah, "Content-Type": "application/json" },
    body: JSON.stringify({ content: "统一 QQ 风格验证瓶" }),
  });
  if (tr.ok) {
    const mine = (await (await fetch(`${BASE}/api/bottles/mine`, { headers: ah })).json()).bottles;
    const tid = mine[0]?.id;
    if (tid) {
      await page.goto(`${BASE}/bottle/thread/${tid}`, { waitUntil: "networkidle" });
      await page.waitForTimeout(800);
      await page.screenshot({ path: `${OUT}15-bottle-thread-qq.png` });
    }
  }
} catch (e) {
  console.log("bottle thread shot skipped:", e.message);
}

await browser.close();
console.log("DONE");
