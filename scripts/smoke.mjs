#!/usr/bin/env node
/**
 * Ripple 实机集成冒烟（scripts/smoke.mjs）
 * 依赖：Node ≥21（全局 fetch / WebSocket），无需 npm install。
 * 用法：
 *   1. 先启动服务端：cd server && RIPPLE_MYSQL_DSN='root:<密码>@tcp(peng:3306)/<库>?charset=utf8mb4&multiStatements=true' RIPPLE_ADDR=':8080' go run ./cmd/server
 *   2. node scripts/smoke.mjs            # 默认 http://127.0.0.1:8080
 *      SMOKE_BASE=http://host:port node scripts/smoke.mjs
 * 注意：会在目标库写入随机前缀的测试用户（u_smk_*），只做加法不做清理（建议指向测试库 ripple_test）。
 */
const BASE = process.env.SMOKE_BASE || "http://127.0.0.1:8080";
const PREFIX = "u_smk_" + Math.random().toString(36).slice(2, 8);
let failed = 0;
const ok = (name) => console.log(`  ✔ ${name}`);
const bad = (name, detail) => { failed++; console.log(`  ✘ ${name}  ${detail ?? ""}`); };
const assert = (cond, name, detail) => (cond ? ok(name) : bad(name, detail));

async function req(method, path, body, token) {
  const headers = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (token) headers["Authorization"] = `Bearer ${token}`;
  const res = await fetch(BASE + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  return { status: res.status, data };
}

async function register(name) {
  const r = await req("POST", "/api/auth/register", { username: name, password: "pass123456" });
  if (r.status !== 201) throw new Error(`register ${name} failed: ${r.status} ${JSON.stringify(r.data)}`);
  return { token: r.data.token, user: r.data.user };
}

/** WS 收帧：返回 {wait(match, timeoutMs), close()} */
function wsCollect(token) {
  const url = BASE.replace(/^http/, "ws") + `/api/ws?token=${encodeURIComponent(token)}`;
  const ws = new WebSocket(url);
  const frames = [];
  const waiters = [];
  ws.addEventListener("message", (ev) => {
    let f; try { f = JSON.parse(ev.data); } catch { return; }
    frames.push(f);
    for (let i = waiters.length - 1; i >= 0; i--) {
      if (waiters[i].match(f)) { waiters.splice(i, 1)[0].resolve(f); }
    }
  });
  const opened = new Promise((res, rej) => {
    ws.addEventListener("open", () => res());
    ws.addEventListener("error", (e) => rej(new Error("ws error")));
  });
  return {
    opened,
    wait(match, timeoutMs = 5000) {
      const hit = frames.find(match);
      if (hit) return Promise.resolve(hit);
      return new Promise((resolve) => {
        const t = setTimeout(() => { const idx = waiters.findIndex((w) => w.match === match); if (idx >= 0) waiters.splice(idx, 1); resolve(null); }, timeoutMs);
        const w = { match, resolve: (f) => { clearTimeout(t); resolve(f); } };
        waiters.push(w);
      });
    },
    send(obj) { ws.send(JSON.stringify(obj)); },
    close() { ws.close(); },
  };
}

const run = async () => {
  console.log(`[smoke] target=${BASE} prefix=${PREFIX}`);

  // ── 注册与资料 ──
  const [u1, u2, u3] = await Promise.all([register(PREFIX + "a"), register(PREFIX + "b"), register(PREFIX + "c")]);
  assert(u1.token && u1.user.id, "3 用户注册");
  const me = await req("GET", "/api/me", undefined, u1.token);
  assert(me.status === 200 && me.data.username === PREFIX + "a", "GET /me");
  const up = await req("PUT", "/api/me", { signature: "海是相通的" }, u1.token);
  assert(up.status === 200 && up.data.signature === "海是相通的", "PUT /me 资料");

  // ── 搜索与好友 ──
  const s1 = await req("GET", `/api/users/search?q=${PREFIX}b`, undefined, u1.token);
  assert(s1.status === 200 && s1.data.users.some((x) => x.user.username === PREFIX + "b" && x.relation === "none"), "搜索用户 relation=none");
  const fr = await req("POST", "/api/friends/requests", { user_id: u2.user.id }, u1.token);
  assert(fr.status === 201 && fr.data.status === "pending", "u1 申请 u2 → pending");
  const fr2 = await req("POST", "/api/friends/requests", { user_id: u1.user.id }, u3.token);
  const fl2 = await req("GET", "/api/friends", undefined, u2.token);
  const pendingIn = fl2.data.pending_in.find((x) => x.user.id === u1.user.id);
  assert(!!pendingIn, "u2 收到 pending_in");
  const acc = await req("POST", `/api/friends/requests/${pendingIn.id}/accept`, {}, u2.token);
  assert(acc.status === 200, "u2 同意好友");
  const fl1 = await req("GET", "/api/friends", undefined, u1.token);
  assert(fl1.data.friends.some((x) => x.id === u2.user.id), "u1 好友列表含 u2");

  // ── 单聊 ──
  const sc = await req("POST", "/api/conversations", { type: "single", user_id: u2.user.id }, u1.token);
  assert((sc.status === 200 || sc.status === 201) && sc.data.type === "single" && sc.data.peer?.id === u2.user.id, "创建 single 会话（幂等顶层 Summary，新建 201/已存在 200）");
  const sc2 = await req("POST", "/api/conversations", { type: "single", user_id: u1.user.id }, u2.token);
  assert(sc2.data.id === sc.data.id, "single 幂等（对端创建同 id）");
  for (let i = 1; i <= 3; i++) {
    const m = await req("POST", `/api/conversations/${sc.data.id}/messages`, { type: "text", body: `m${i}` }, u1.token);
    assert(m.status === 201 && m.data.seq === i, `消息 seq=${i}`);
  }
  const inc = await req("GET", `/api/conversations/${sc.data.id}/messages?after_seq=1`, undefined, u2.token);
  assert(inc.data.messages.length === 2 && inc.data.messages[0].seq === 2, "after_seq=1 增量拉取恰好 2 条");
  const rd = await req("POST", `/api/conversations/${sc.data.id}/read`, { seq: 3 }, u2.token);
  assert(rd.status === 200 && rd.data.last_read_seq === 3, "已读上报");
  const list2 = await req("GET", "/api/conversations", undefined, u2.token);
  const c2 = list2.data.conversations.find((c) => c.id === sc.data.id);
  assert(c2 && c2.unread === 0, "已读后未读清零");

  // ── 群聊 ──
  const g = await req("POST", "/api/conversations", { type: "group", name: "冒烟群", member_ids: [u2.user.id, u3.user.id] }, u1.token);
  assert(g.status === 201 && g.data.type === "group" && g.data.member_count === 3, "建群 3 人");
  const gm = await req("POST", `/api/conversations/${g.data.id}/messages`, { type: "text", body: "群第一条" }, u2.token);
  assert(gm.status === 201, "u2 群发消息");
  const gm3 = await req("GET", `/api/conversations/${g.data.id}/messages`, undefined, u3.token);
  assert(gm3.data.messages.some((m) => m.sender_id === u2.user.id && m.body === "群第一条"), "u3 可见群消息");
  const detail = await req("GET", `/api/conversations/${g.data.id}`, undefined, u1.token);
  assert(detail.status === 200 && detail.data.members.length === 3, "群明细 members=3");
  const rn = await req("PUT", `/api/conversations/${g.data.id}`, { name: "冒烟群改" }, u1.token);
  assert(rn.status === 200 && rn.data.name === "冒烟群改", "群改名（owner）");
  const rn3 = await req("PUT", `/api/conversations/${g.data.id}`, { name: "x" }, u3.token);
  assert(rn3.status === 403, "非 owner 改名 403");

  // ── 撤回 ──
  const rv = await req("POST", `/api/messages/${gm.data.id}/revoke`, {}, u2.token);
  assert(rv.status === 200 && rv.data.message.revoked_at != null && rv.data.message.body === "", "撤回（body 清空+revoked_at）");
  const rv3 = await req("POST", `/api/messages/${gm.data.id}/revoke`, {}, u3.token);
  assert(rv3.status === 403, "非本人撤回 403");

  // ── 黑名单 ──
  const blk = await req("POST", `/api/friends/${u3.user.id}/block`, {}, u1.token);
  assert(blk.status === 200, "u1 拉黑 u3");
  const sendBlocked = await req("POST", `/api/conversations/${sc.data.id}/messages`, { type: "text", body: "x" }, u2.token);
  assert(sendBlocked.status !== 403, "u1↔u2 不受 u1↔u3 黑名单影响");
  const s3 = await req("GET", `/api/users/search?q=${PREFIX}c`, undefined, u1.token);
  assert(s3.data.users.every((x) => x.relation === "blocked"), "搜索 relation=blocked");
  const unblk = await req("DELETE", `/api/friends/${u3.user.id}/block`, undefined, u1.token);
  assert(unblk.status === 200, "解除拉黑");

  // ── 漂流瓶 ──
  const tb = await req("POST", "/api/bottles", { content: "来自冒烟的瓶子" }, u3.token);
  assert(tb.status === 201, "u3 扔瓶");
  const pk = await req("POST", "/api/bottles/pick", {}, u1.token);
  assert(pk.status === 201 && pk.data.conversation_id && pk.data.messages.length >= 1, "u1 捡瓶（首条消息即瓶内容）");
  const raw = JSON.stringify(pk.data);
  assert(!raw.includes("sender_id"), "匿名投影：响应无 sender_id");
  assert(pk.data.my_alias === "捡瓶人", "alias=捡瓶人");
  const bm = await req("POST", `/api/bottles/threads/${pk.data.id}/messages`, { body: "匿名回一句" }, u1.token);
  assert(bm.status === 201 && bm.data.mine === true && bm.data.alias === "捡瓶人", "匿名回复（mine 视角）");
  const rvBottle = await req("POST", `/api/bottles/threads/${pk.data.id}/reveal`, {}, u1.token);
  assert(rvBottle.status === 200 && rvBottle.data.conversation_id && rvBottle.data.peer.id === u3.user.id, "reveal 产生私聊（peer=u3）");

  // ── WS 实时链路 ──
  const w1 = wsCollect(u1.token);
  const w2 = wsCollect(u2.token);
  await w1.opened; await w2.opened;
  ok("双用户 WS 连接");
  const CMID = "smoke-" + Date.now();
  w1.send({ type: "message.send", client_msg_id: CMID, conversation_id: sc.data.id, msg_type: "text", body: "ws 冒烟" });
  const ack = await w1.wait((f) => f.type === "message.ack" && f.client_msg_id === CMID);
  assert(!!ack && ack.message.body === "ws 冒烟", "发送端 message.ack（client_msg_id 关联）");
  const new2 = await w2.wait((f) => f.type === "message.new" && f.message.id === ack.message.id);
  assert(!!new2, "对端收到 message.new（同 message.id；发送端按契约仅收 ack 不重复收 new）");
  w2.close();
  const offline = await w1.wait((f) => f.type === "presence.change" && f.user_id === u2.user.id && f.online === false, 8000);
  assert(!!offline, "u2 断开后 presence.change 下线广播");
  w1.close();

  console.log(failed === 0 ? "\n[smoke] ALL PASS ✅" : `\n[smoke] FAILED: ${failed} 项 ❌`);
  process.exit(failed === 0 ? 0 : 1);
};

run().catch((e) => { console.error("[smoke] fatal:", e.message); process.exit(1); });
