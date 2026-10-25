import type { ReactNode } from "react";
import { Link } from "react-router-dom";

/**
 * QQ 式页面顶栏：左侧圆形返回箭头 + 居中标题 + 右侧动作位。
 * 统一替换各页面的「← 文字链接」顶栏（topbar 网格：1fr auto 1fr）。
 */
export function PageHeader({
  title,
  backTo,
  backLabel = "返回",
  right,
}: {
  title: ReactNode;
  backTo: string;
  backLabel?: string;
  right?: ReactNode;
}) {
  return (
    <header className="topbar">
      <Link to={backTo} className="topbar-back" aria-label={backLabel} title={backLabel}>
        <svg viewBox="0 0 24 24" width="20" height="20" aria-hidden>
          <path
            d="M14.5 5.5 8 12l6.5 6.5"
            fill="none"
            stroke="currentColor"
            strokeWidth="1.9"
            strokeLinecap="round"
            strokeLinejoin="round"
          />
        </svg>
      </Link>
      <h2>{title}</h2>
      {right ?? <span />}
    </header>
  );
}
