import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // 绝对路径：桌面壳以标准 origin（ripple://local/）承载 web/dist，H5 由 Go 后端
  // 托管在 :8080 根路径——两者均需 /assets/... 绝对引用，二级深链（/chat/:id 等）
  // 刷新才不会解析成 /chat/assets/...（曾用 "./" 相对 base，深链刷新即断）
  base: "/",
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8080", ws: true },
      "/uploads": "http://localhost:8080",
    },
  },
});
