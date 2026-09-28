import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  // 桌面壳以 file:// 加载 web/dist/index.html，必须相对路径
  base: "./",
  server: {
    port: 5173,
    proxy: {
      "/api": { target: "http://localhost:8080", ws: true },
      "/uploads": "http://localhost:8080",
    },
  },
});
