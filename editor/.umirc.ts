import { defineConfig } from "umi";

export default defineConfig({
  history: {
    type: 'hash'
  },
  publicPath: process.env.NODE_ENV === 'production' ? './' : '/',
  hash: true,
  routes: [
    {
      path: '/',
      redirect: '/editor',
    },
    { path: "/editor/*", component: "editor", name: "editor", key: "editor" },
    { path: "/bots", component: "bots", name: "bots", key: "bots" },
    { path: "/running", component: "running", name: "running", key: "running" },
    { path: "/report", component: "report", name: "report", key: "report" },
    { path: "/prefab", component: "prefab", name: "prefab", key: "prefab" },
    { path: "/config", component: "config", name: "config", key: "config" },
  ],
  plugins: [
  ],
  npmClient: 'pnpm',
});
