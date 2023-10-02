import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  server: {
    cors: true,
    proxy: {
      "/api": {
        target: "http://localhost:10786",
        changeOrigin: true,
        //rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
  build: {
    rollupOptions: {
      output: {
        entryFileNames: "fs/bundle.js", // The name of the output JavaScript file
        assetFileNames: "fs/style.css", // The name of the output CSS file
        manualChunks: undefined, // Disable chunk splitting
      },
    },
    cssCodeSplit: false, // Disable splitting CSS files,
    assetsDir: "fs", //  asset directory
  },
});
