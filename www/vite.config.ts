import path from "node:path";
import { defineConfig, type UserConfig } from "vite";
import vue from "@vitejs/plugin-vue";
import VueI18nPlugin from "@intlify/unplugin-vue-i18n/vite";
import { compression } from "vite-plugin-compression2";

// Shared configuration logic
const alias = {
  "@": path.resolve(__dirname, "src"),
};

const commonPlugins = [
  vue(),
];

// https://vitejs.dev/config/
export default defineConfig(({ command }) => {
  // 1. Plugin Bundle Mode (Library mode)
  if (process.env.BUILD_TARGET === "plugin") {
    return {
      plugins: commonPlugins,
      resolve: { 
        alias: { ...alias, "@/i18n": path.resolve(__dirname, "src/i18n/mock.ts") } 
      },
      build: {
        lib: {
          entry: process.env.PLUGIN_ENTRY || "",
          name: "plugin",
          formats: ["iife"],
          fileName: () => "index.js",
        },
        rollupOptions: {
          external: ["vue", "vue-router", "pinia", "vue-i18n", "dayjs", "qrcode.vue"],
          output: {
            globals: {
              vue: "window.__ABYSS__.Vue",
              "vue-router": "window.__ABYSS__.Router",
              pinia: "window.__ABYSS__.Pinia",
              "vue-i18n": "window.__ABYSS__.I18n",
              dayjs: "window.__ABYSS__.dayjs",
              "qrcode.vue": "window.__ABYSS__.QrcodeVue",
            },
            extend: true,
          },
        },
        outDir: process.env.PLUGIN_OUT_DIR || "dist",
        emptyOutDir: true,
        cssCodeSplit: false,
      },
    } as any;
  }

  // 2. Main SPA Mode
  const isBuild = command === "build";
  return {
    define: {
      __EDITION__: JSON.stringify("pro"),
    },
    plugins: [
      ...commonPlugins,
      VueI18nPlugin({
        include: [path.resolve(__dirname, "./src/shared/i18n/*.json")],
      }),
      isBuild && compression({ include: /\.js$/i, deleteOriginalAssets: false }),
    ],
    resolve: { alias },
    server: {
      proxy: {
        "/api/command": { target: "ws://127.0.0.1:8080", ws: true },
        "/api": "http://127.0.0.1:8080",
      },
    },
    build: {
      chunkSizeWarningLimit: 1500,
      rollupOptions: {
        output: {
          manualChunks: (id: string) => {
            if (id.includes("node_modules")) {
              const deps = ["video.js", "ace-builds", "epubjs", "lodash-es"];
              for (const dep of deps) if (id.includes(dep)) return dep.split(".")[0];
              if (["vue", "pinia", "vue-router"].some(k => id.includes(k))) return "vendor";
              if (id.includes("dayjs/")) return "dayjs";
            }
          },
        },
      },
      experimental: {
        renderBuiltUrl(filename: string, { hostType }: { hostType: 'js' | 'css' | 'html' }) {
          if (hostType === "js") return { runtime: `window.__prependStaticUrl("${filename}")` };
          if (hostType === "html") return `[{[ .StaticURL ]}]/${filename}`;
          return { relative: true };
        },
      },
    },
  } as UserConfig;
});
