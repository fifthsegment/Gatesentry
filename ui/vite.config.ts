import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { copyFileSync, mkdirSync } from "fs";
import { resolve } from "path";

// Small plugin to copy static assets (like the favicon) into dist/fs/
// so they're served by Go's /fs/ file handler.
function copyToFs() {
  return {
    name: "copy-to-fs",
    writeBundle() {
      const src = resolve(__dirname, "src/assets/gatesentry.svg");
      const dest = resolve(__dirname, "dist/fs/gatesentry.svg");
      mkdirSync(resolve(__dirname, "dist/fs"), { recursive: true });
      copyFileSync(src, dest);
    },
  };
}

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte(), copyToFs()],
  base: "./", // Relative asset paths — Go injects <base href> for reverse proxy base path
  server: {
    proxy: {
      "/api": {
        target: "http://localhost:8080",
        changeOrigin: true,
        rewrite: (path) => `/gatesentry${path}`,
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        // Keep the main entry as bundle.js for backwards compatibility
        entryFileNames: "fs/bundle.js",
        // Chunks get content-hashed names for cache-busting
        chunkFileNames: "fs/[name]-[hash].js",
        assetFileNames: "fs/[name]-[hash][extname]",
        manualChunks: {
          // Heavy vendor libs in their own chunks — loaded once, cached forever
          carbon: ["carbon-components-svelte", "carbon-icons-svelte"],
          charts: ["@carbon/charts"],
          vendor: ["lodash", "svelte-i18n", "svelte-routing", "timeago.js"],
        },
      },
    },
    cssCodeSplit: false, // Keep CSS in one file for simplicity
    assetsDir: "fs",
  },
});
