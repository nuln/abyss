/**
 * Theme utility for applying and auto-detecting themes.
 */

export type ThemeMode = "auto" | "light" | "dark" | "";

const THEME_KEY = "theme";

export function getTheme(): ThemeMode {
    return (localStorage.getItem(THEME_KEY) as ThemeMode) || "auto";
}

export function getMediaPreference(): ThemeMode {
    const hasDarkPreference = window.matchMedia("(prefers-color-scheme: dark)").matches;
    return hasDarkPreference ? "dark" : "light";
}

function isSystemDark(): boolean {
    return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function setTheme(theme: ThemeMode) {
    localStorage.setItem(THEME_KEY, theme || "auto");
    applyTheme(theme || "auto");
    startThemeWatcher(theme || "auto");
}

export function applyTheme(theme: ThemeMode) {
    const root = document.documentElement;

    if (theme === "auto" || theme === "") {
        root.classList.toggle("dark", isSystemDark());
    } else if (theme === "dark") {
        root.classList.add("dark");
    } else {
        root.classList.remove("dark");
    }
}

let mediaQuery: MediaQueryList | null = null;
let mediaQueryHandler: ((e: MediaQueryListEvent) => void) | null = null;

export function startThemeWatcher(theme: ThemeMode) {
    stopThemeWatcher();

    if (theme === "auto" || theme === "") {
        mediaQuery = window.matchMedia("(prefers-color-scheme: dark)");
        mediaQueryHandler = () => {
            applyTheme("auto");
        };
        mediaQuery.addEventListener("change", mediaQueryHandler);
    }
}

export function stopThemeWatcher() {
    if (mediaQuery && mediaQueryHandler) {
        mediaQuery.removeEventListener("change", mediaQueryHandler);
        mediaQuery = null;
        mediaQueryHandler = null;
    }
}

export function getEditorTheme(theme?: string): string {
    const currentTheme = (theme as ThemeMode) || getTheme();
    let isDark = currentTheme === "dark";

    if (currentTheme === "auto" || currentTheme === "") {
        isDark = isSystemDark();
    }

    return isDark ? "ace/theme/tomorrow_night" : "ace/theme/tomorrow";
}
