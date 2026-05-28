<template>
  <ModalsContainer />
</template>

<script setup lang="ts">
import { h, watch } from "vue";
import { ModalsContainer, useModal } from "vue-final-modal";
import { storeToRefs } from "pinia";
import { useLayoutStore } from "@/app/stores/layout";

import BaseModal from "./BaseModal.vue";
import Help from "./Help.vue";
import Info from "./Info.vue";
import Delete from "./Delete.vue";
import DeleteUser from "./DeleteUser.vue";
import Download from "./Download.vue";
import Rename from "./Rename.vue";
import Move from "./Move.vue";
import Copy from "./Copy.vue";
import NewFile from "./NewFile.vue";
import NewDir from "./NewDir.vue";
import CreateMenu from "./CreateMenu.vue";
import Replace from "./Replace.vue";
import ReplaceRename from "./ReplaceRename.vue";
import Upload from "./Upload.vue";
import DiscardEditorChanges from "./DiscardEditorChanges.vue";
import SimpleInput from "./SimpleInput.vue";
import SimpleConfirm from "./SimpleConfirm.vue";
import SimpleAlert from "./SimpleAlert.vue";

const layoutStore = useLayoutStore();

const { currentPromptName } = storeToRefs(layoutStore);

const components = new Map<string, any>([
  ["info", Info],
  ["help", Help],
  ["delete", Delete],
  ["rename", Rename],
  ["move", Move],
  ["copy", Copy],
  ["newFile", NewFile],
  ["newDir", NewDir],
  ["createMenu", CreateMenu],
  ["download", Download],
  ["replace", Replace],
  ["replace-rename", ReplaceRename],
  ["upload", Upload],
  ["deleteUser", DeleteUser],
  ["discardEditorChanges", DiscardEditorChanges],
  ["simpleInput", SimpleInput],
  ["simpleConfirm", SimpleConfirm],
  ["simpleAlert", SimpleAlert],
]);

// eslint-disable-next-line @typescript-eslint/no-unused-vars
let activeModalClose: (() => void) | null = null;
let activeModalName: string | null = null;

watch(currentPromptName, (newValue) => {
  // If we already have a modal open for this name, don't re-open
  if (newValue === activeModalName) return;

  // If new value is null, it means prompts were cleared
  if (!newValue) {
    activeModalName = null;
    activeModalClose = null;
    return;
  }

  let modal = components.get(newValue!);
  if (!modal) {
    // Try lookup in dynamic components
    modal = window.__ABYSS__?.components?.[newValue!];
  }
  if (!modal) return;

  // Capture properties at creation time
  const props = {
    ...layoutStore.currentPrompt?.props,
    confirm: layoutStore.currentPrompt?.confirm,
  };

  const { open, close } = useModal({
    component: BaseModal,
    slots: {
      default: {
        component: h(modal, props),
      },
    },
  });

  activeModalName = newValue;
  activeModalClose = close;
  layoutStore.setCloseOnPrompt(close, newValue!);
  open();
});

window.addEventListener("keydown", (event) => {
  if (!layoutStore.currentPrompt) return;

  if (event.key === "Escape") {
    event.stopImmediatePropagation();
    layoutStore.closeHovers();
  }
});
</script>
