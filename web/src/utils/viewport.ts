/**
 * 移动端视口基建（移动 H5 计划 §5.2）
 *
 * - installAppViewportHeight()：main.tsx 渲染前调用一次。
 *   把「当前可见视口高度」写入 CSS 变量 --app-vh（.chat-room 消费），
 *   并在键盘弹出时给 <html> 打上 data-kb 标记（供移动样式收掉多余安全区空白）。
 * - isCoarsePointer()：触屏判定，ChatRoomPage 的点按撤回用它做门控。
 *   两个导出的名字均已冻结，不得改名。
 */

/** 可见高度比 window.innerHeight 少这么多像素 → 判定为键盘弹出 */
const KB_THRESHOLD_PX = 120;

/** 捏合缩放阈值：scale 超过它时跳过更新，避免用缩放后的高度压缩布局 */
const ZOOM_EPSILON = 1.01;

let installed = false;
let rafId = 0;

function apply(): void {
  const vv = window.visualViewport;
  // 捏合缩放保护：放大查看时 visualViewport.height 会变小，不能拿它改布局高度
  if (vv && vv.scale > ZOOM_EPSILON) return;
  const h = vv?.height ?? window.innerHeight;
  const root = document.documentElement;
  root.style.setProperty("--app-vh", `${Math.round(h)}px`);
  root.toggleAttribute("data-kb", h < window.innerHeight - KB_THRESHOLD_PX);
}

/** rAF 合并高频事件（visualViewport.resize/scroll 在键盘动画期可能每帧触发） */
function schedule(): void {
  if (rafId) return;
  rafId = requestAnimationFrame(() => {
    rafId = 0;
    apply();
  });
}

export function installAppViewportHeight(): void {
  if (installed) return;
  installed = true;

  apply(); // 首帧渲染前先写一次，避免首屏用错高度

  const vv = window.visualViewport;
  vv?.addEventListener("resize", schedule);
  vv?.addEventListener("scroll", schedule); // iOS 键盘动画期只发 scroll，不发 resize
  window.addEventListener("resize", schedule);
  window.addEventListener("orientationchange", schedule);
}

export function isCoarsePointer(): boolean {
  return window.matchMedia("(hover: none)").matches;
}
