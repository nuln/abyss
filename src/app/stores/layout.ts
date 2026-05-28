import { defineStore } from "pinia";
// import { useAuthPreferencesStore } from "./auth-preferences";
// import { useAuthEmailStore } from "./auth-email";

export const useLayoutStore = defineStore("layout", {
  // convert to a function
  state: (): {
    loading: boolean;
    prompts: PopupProps[];
    suppressNextClosed: boolean;
    sidebarVisible: boolean;
  } => ({
    loading: false,
    prompts: [],
    // When set, the next `onModalClosed` call will be ignored because
    // it corresponds to a programmatic close triggered by `closeHovers()`.
    suppressNextClosed: false,
    sidebarVisible: true,
  }),
  getters: {
    currentPrompt(state) {
      return state.prompts.length > 0
        ? state.prompts[state.prompts.length - 1]
        : null;
    },
    currentPromptName(): string | null | undefined {
      return this.currentPrompt?.prompt;
    },
    // user and jwt getter removed, no longer needed
  },
  actions: {
    // no context as first argument, use `this` instead
    toggleSidebar() {
      this.sidebarVisible = !this.sidebarVisible;
    },
    showSidebar() {
      this.sidebarVisible = true;
    },
    hideSidebar() {
      this.sidebarVisible = false;
    },
    setCloseOnPrompt(closeFunction: () => Promise<string>, onPrompt: string) {
      const prompt = this.prompts.find((prompt) => prompt.prompt === onPrompt);
      if (prompt) {
        prompt.close = closeFunction;
      }
    },
    showHover(value: PopupProps | string) {
      if (typeof value !== "object") {
        this.prompts.push({
          prompt: value,
          confirm: null,
          action: undefined,
          saveAction: undefined,
          props: null,
          close: null,
        });
        return;
      }

      this.prompts.push({
        prompt: value.prompt,
        confirm: value?.confirm,
        action: value?.action,
        saveAction: value?.saveAction,
        props: value?.props,
        close: value?.close,
      });
    },
    showError() {
      this.prompts.push({
        prompt: "error",
        confirm: null,
        action: undefined,
        props: null,
        close: null,
      });
    },
    showSuccess() {
      this.prompts.push({
        prompt: "success",
        confirm: null,
        action: undefined,
        props: null,
        close: null,
      });
    },

    // Called to programmatically close the currently active prompt.
    // This triggers the modal's close function, but also sets a suppress
    // flag so that when the modal emits its `closed` event we don't
    // accidentally close the next prompt opened during the animation.
    closeHovers() {
      if (this.prompts.length === 0) return;
      this.suppressNextClosed = true;
      const prompt = this.prompts.pop();
      prompt?.close?.();
    },

    // Called by BaseModal when the modal has emitted the `closed` event.
    // If the close was initiated programmatically via `closeHovers` we
    // suppress handling here; otherwise remove the last prompt from the
    // stack (the modal has already closed its UI).
    onModalClosed() {
      if (this.suppressNextClosed) {
        this.suppressNextClosed = false;
        return;
      }

      // Remove the last prompt (modal closed by user interaction).
      this.prompts.pop();
    },
    // easily reset state using `$reset`
    clearLayout() {
      this.$reset();
    },
  },
});
