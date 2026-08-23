import { onMounted, onUnmounted } from "vue";

export function useContextMenu() {
  const handleContextMenu = (e: MouseEvent) => {
    try {
      const path = (e.composedPath && e.composedPath()) || (e as any).path || [];
      const hitInteractive = path.some((n: any) => {
        if (!n || !n.classList) return false;
        return (
          n.classList.contains("photo-card") ||
          n.classList.contains("album-card") ||
          n.classList.contains("item") ||
          (n.closest && typeof n.closest === "function" && n.closest(".modal-overlay"))
        );
      });

      // Also check if we are inside the main content area
      const hitMain = path.some((n: any) => n.tagName === "MAIN");

      if (!hitInteractive && hitMain) {
        // Prevent the browser default menu when we will show the app menu
        try {
          e.preventDefault();
        } catch (_err) {
          // Ignore error
        }
        try {
          e.stopPropagation();
        } catch (_err) {
          // Ignore error
        }
        const ev = new CustomEvent("app:empty-contextmenu", {
          detail: { x: e.clientX, y: e.clientY },
          bubbles: true,
        });
        window.dispatchEvent(ev);
      }
    } catch (_err) {
      // ignore
    }
  };

  onMounted(() => {
    document.addEventListener("contextmenu", handleContextMenu, { capture: true });
  });

  onUnmounted(() => {
    document.removeEventListener("contextmenu", handleContextMenu, { capture: true });
  });
}
