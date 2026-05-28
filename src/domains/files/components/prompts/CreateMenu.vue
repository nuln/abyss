<template>
  <div class="card floating">
      <div class="card-title">
        <h2>{{ "" }}</h2>
      </div>

      <div class="card-action full">
        <div
          @click="openNewDir"
          class="action"
          id="focus-prompt"
          tabindex="1"
        >
          <i class="material-icons">create_new_folder</i>
          <div class="title">{{ t("prompts.newDir") }}</div>
        </div>

        <div
          @click="openNewFile"
          class="action"
          tabindex="2"
        >
          <i class="material-icons">note_add</i>
          <div class="title">{{ t("prompts.newFile") }}</div>
        </div>

        <div
          @click="triggerUploadFile"
          class="action"
          tabindex="3"
        >
          <i class="material-icons">upload_file</i>
          <div class="title">{{ t("prompts.uploadFile") }}</div>
        </div>

        <div
          @click="triggerUploadFolder"
          class="action"
          tabindex="4"
        >
          <i class="material-icons">drive_folder_upload</i>
          <div class="title">{{ t("prompts.uploadFolder") }}</div>
        </div>
      </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { useLayoutStore } from "@/app/stores/layout";
import { useFileStore } from "@/domains/files/store";
import * as upload from "@/domains/files/utils";

const { t } = useI18n();
const layoutStore = useLayoutStore();
const fileStore = useFileStore();
const route = useRoute();
const router = useRouter();

const openNewDir = () => {
  layoutStore.closeHovers();
  layoutStore.showHover("newDir");
};

const openNewFile = () => {
  layoutStore.closeHovers();
  layoutStore.showHover("newFile");
};

const uploadInput = (event: Event) => {
  const files = (event.currentTarget as HTMLInputElement)?.files;
  if (files === null) return;

  const folder_upload = !!files[0].webkitRelativePath;

  const uploadFiles: UploadList = [];
  for (let i = 0; i < files.length; i++) {
    const file = files[i];
    const fullPath = folder_upload ? file.webkitRelativePath : undefined;
    uploadFiles.push({
      file,
      name: file.name,
      size: file.size,
      isDir: false,
      fullPath,
    });
  }

  // Get the current path, or default to /files/ if not on files page
  let path = route.path;
  if (!path.startsWith("/files/")) {
    path = "/files/";
    // Navigate to files page if not already there
    router.push(path);
  }
  path = path.endsWith("/") ? path : path + "/";

  const conflict = upload.checkConflict(uploadFiles, fileStore.req?.items || []);

  if (conflict) {
    layoutStore.showHover({
      prompt: "replace",
      action: (event: Event) => {
        event.preventDefault();
        layoutStore.closeHovers();
        upload.handleFiles(uploadFiles, path, false);
      },
      confirm: (event: Event) => {
        event.preventDefault();
        layoutStore.closeHovers();
        upload.handleFiles(uploadFiles, path, true);
      },
    });

    return;
  }

  upload.handleFiles(uploadFiles, path);
};

const openUpload = (isFolder: boolean) => {
  layoutStore.closeHovers();
  const input = document.createElement("input");
  input.type = "file";
  input.multiple = true;
  if (isFolder) {
    input.webkitdirectory = true;
  }
  input.onchange = uploadInput;
  input.click();
};

const triggerUploadFile = () => {
  openUpload(false);
};

const triggerUploadFolder = () => {
  openUpload(true);
};
</script>

<style scoped>
.card-action.full {
  display: grid !important;
  grid-template-columns: repeat(2, 1fr) !important;
  gap: 0 !important;
  padding-top: 0 !important;
}

.card-action.full .action {
  display: flex !important;
  flex-direction: column !important;
  align-items: center !important;
  justify-content: center !important;
  padding: var(--space-5) !important;
  text-align: center !important;
  cursor: pointer;
  transition: all var(--transition-fast);
  border: 1px solid var(--border-default);
  box-sizing: border-box !important;
  flex: none !important; /* Prevent global flex: 1 interference */
}

.card-action.full .action:hover {
  background-color: var(--color-accent-soft);
  border-color: var(--color-accent);
}

.card-action.full .action i {
  display: block !important;
  font-size: 4em !important;
  margin-bottom: var(--space-2) !important;
  color: var(--text-primary);
}

.card-action.full .action .title {
  font-size: var(--text-lg) !important;
  font-weight: 500;
  color: var(--text-primary);
}
</style>
