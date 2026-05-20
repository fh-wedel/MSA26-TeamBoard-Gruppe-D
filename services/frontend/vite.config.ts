import { defineConfig, loadEnv } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), "");
  const coreUrl = env.VITE_CORE_URL ?? "http://localhost:3000";
  const broadcasterUrl = env.VITE_BROADCASTER_URL ?? "ws://localhost:3002";

  const coreProxyTarget = process.env.CORE_PROXY_TARGET ?? env.CORE_PROXY_TARGET ?? coreUrl;
  const broadcasterProxySource =
    process.env.BROADCASTER_PROXY_TARGET ?? env.BROADCASTER_PROXY_TARGET ?? broadcasterUrl;
  const wsProxyTarget = broadcasterProxySource
    .replace(/^ws:\/\//, "http://")
    .replace(/^wss:\/\//, "https://");

  return {
    plugins: [react()],
    server: {
      host: "0.0.0.0",
      port: 5173,
      strictPort: true,
      proxy: {
        "/api": {
          target: coreProxyTarget,
          changeOrigin: true,
        },
        "/admin-stream": {
          target: coreProxyTarget,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/admin-stream/, "/api/admin/events/stream"),
        },
        "/ws-events": {
          target: wsProxyTarget,
          changeOrigin: true,
          ws: true,
          rewrite: (path) => path.replace(/^\/ws-events/, "/ws"),
        },
      },
    },
    define: {
      // Expose resolved env to the app at build/dev time.
      __CORE_URL__: JSON.stringify(coreUrl),
      __BROADCASTER_URL__: JSON.stringify(broadcasterUrl),
    },
  };
});
