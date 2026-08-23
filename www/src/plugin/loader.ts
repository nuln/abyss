import { baseURL } from "@/shared/utils/constants";

const loadedPlugins = new Set<string>();
const pendingPlugins = new Map<string, Promise<void>>();

function isAllowedDevServer(rawURL: string): boolean {
    try {
        const parsed = new URL(rawURL);
        return ["localhost", "127.0.0.1", "[::1]"].includes(parsed.hostname);
    } catch {
        return false;
    }
}

/**
 * loads a plugin script dynamically.
 * The script should call window.__ABYSS__.registerPlugin upon loading.
 */
export async function loadPlugin(slug: string): Promise<void> {
    if (loadedPlugins.has(slug)) return;
    if (pendingPlugins.has(slug)) return pendingPlugins.get(slug)!;


    // Load plugin CSS 
    // We use a more resilient approach: don't let CSS load errors block script loading
    const link = document.createElement("link");
    link.rel = "stylesheet";
    link.type = "text/css";
    link.href = `${baseURL}/static/plugins/${slug}/abyss-frontend.css`;
    link.onerror = () => {
        // Silently ignore CSS load errors as some plugins might not have styles
        link.remove();
    };
    document.head.appendChild(link);

    const script = document.createElement("script");

    // HMR / Dev Mode support: if slug is in VITE_PLUGIN_DEV_SERVERS, load from local vite
    // Format: VITE_PLUGIN_DEV_SERVERS="album:http://localhost:5174,passkey:http://localhost:5175"
    // Compiled out of production builds.
    let devUrl: string | undefined;
    if (import.meta.env.DEV) {
        const devServers = localStorage.getItem("VITE_PLUGIN_DEV_SERVERS") || "";
        const devServerMap: Record<string, string> = Object.fromEntries(
            devServers.split(",").filter(Boolean).map(s => s.split(":http").map((v, i) => i === 1 ? "http" + v : v))
        );
        if (devServerMap[slug] && isAllowedDevServer(devServerMap[slug])) {
            devUrl = devServerMap[slug];
        }
    }

    if (devUrl) {
        // console.warn(`[DEBUG] Loading plugin ${slug} from dev server: ${devServerMap[slug]}`);
        script.src = `${devUrl}/index.ts`; // Vite dev server handles .ts directly
        script.type = "module";
    } else {
        // Plugin assets are served at /static/plugins/{slug}/...
        script.src = `${baseURL}/static/plugins/${slug}/index.js`;
        script.async = true;
    }

    const loadPromise = new Promise<void>((resolve, reject) => {
        script.onload = () => {
            loadedPlugins.add(slug);
            pendingPlugins.delete(slug);
            resolve();
        };
        script.onerror = () => {
            pendingPlugins.delete(slug);
            reject(new Error(`Failed to load plugin: ${slug}`));
        };
    });

    pendingPlugins.set(slug, loadPromise);
    document.head.appendChild(script);

    return loadPromise;
}

// Note: window.__ABYSS__.registerPlugin is initialized in plugin/bridge.ts
