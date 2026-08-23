<template>
  <div>
    <search />
    <breadcrumbs base="/files">
      <template #actions>
        <template v-if="!isMobile">
          <action
            v-if="headerButtons.copy"
            id="copy-button"
            icon="content_copy"
            :label="t('buttons.copyFile')"
            :tooltip="t('tooltips.copy')"
            show="copy"
          />
          <action
            v-if="headerButtons.move"
            id="move-button"
            icon="forward"
            :label="t('buttons.moveFile')"
            :tooltip="t('tooltips.move')"
            show="move"
          />
          <action
            v-if="headerButtons.delete"
            id="delete-button"
            icon="delete"
            :label="t('buttons.delete')"
            :tooltip="t('tooltips.delete')"
            show="delete"
          />

        </template>

        <action
          :icon="viewIcon"
          :label="t('buttons.switchView')"
          :tooltip="t('tooltips.switchView')"
          @action="switchView"
        />
        <action
          v-if="headerButtons.download"
          icon="file_download"
          :label="t('buttons.download')"
          :tooltip="t('tooltips.download')"
          @action="download"
          :counter="fileStore.selectedCount"
        />
        <action
          v-if="headerButtons.upload"
          icon="add_circle_outline"
          id="upload-button"
          :label="t('buttons.new')"
          :tooltip="t('buttons.new')"
          show="createMenu"
        />

        <action
          icon="check_circle"
          :label="t('buttons.selectMultiple')"
          :tooltip="t('tooltips.selectMultiple')"
          @action="toggleMultipleSelection"
        />
        <ActionSlot name="file-header-actions" />
      </template>
    </breadcrumbs>

    <div v-if="isMobile" id="file-selection">
      <span v-if="fileStore.selectedCount > 0">
        {{ t("prompts.filesSelected", fileStore.selectedCount) }}
      </span>
      <action
        v-if="headerButtons.copy"
        icon="content_copy"
        :label="t('buttons.copyFile')"
        :tooltip="t('tooltips.copy')"
        show="copy"
      />
      <action
        v-if="headerButtons.move"
        icon="forward"
        :label="t('buttons.moveFile')"
        :tooltip="t('tooltips.move')"
        show="move"
      />
      <action
        v-if="headerButtons.delete"
        icon="delete"
        :label="t('buttons.delete')"
        :tooltip="t('tooltips.delete')"
        show="delete"
      />

    </div>

    <div v-if="layoutStore.loading">
      <h2 class="message delayed">
        <div class="spinner">
          <div class="bounce1"></div>
          <div class="bounce2"></div>
          <div class="bounce3"></div>
        </div>
        <span>{{ t("files.loading") }}</span>
      </h2>
    </div>
    <template v-else>
      <div
        v-if="
          (fileStore.req?.numDirs ?? 0) + (fileStore.req?.numFiles ?? 0) == 0
        "
        class="empty-folder-container"
        @contextmenu="showBlankContextMenu"
      >
        <h2 class="message">
          <i class="material-icons">sentiment_dissatisfied</i>
          <span>{{ t("files.lonely") }}</span>
        </h2>
        <!-- Blank area context menu for create/upload in empty folder -->
        <context-menu
          :show="isBlankContextMenuVisible"
          :pos="blankContextMenuPos"
          @hide="hideBlankContextMenu"
        >
          <action
            icon="create_new_folder"
            :label="t('prompts.newDir')"
            @action="openNewDir"
          />
          <action
            icon="note_add"
            :label="t('prompts.newFile')"
            @action="openNewFile"
          />
          <action
            icon="upload_file"
            :label="t('prompts.uploadFile')"
            @action="triggerUploadFile"
          />
          <action
            icon="drive_folder_upload"
            :label="t('prompts.uploadFolder')"
            @action="triggerUploadFolder"
          />
        </context-menu>
      </div>
      <div
        v-else
        id="listing"
        ref="listing"
        class="file-icons"
        :class="authStore.user?.viewMode ?? 'mosaic'"
        @contextmenu="showBlankContextMenu"
      >
        <div>
          <div class="item header">
            <div>
              <p
                :class="{ active: nameSorted }"
                class="name"
                role="button"
                tabindex="0"
                @click="sort('name')"
                :title="t('files.sortByName')"
                :aria-label="t('files.sortByName')"
              >
                <span>{{ t("files.name") }}</span>
                <i class="material-icons">{{ nameIcon }}</i>
              </p>

              <p
                :class="{ active: sizeSorted }"
                class="size"
                role="button"
                tabindex="0"
                @click="sort('size')"
                :title="t('files.sortBySize')"
                :aria-label="t('files.sortBySize')"
              >
                <span>{{ t("files.size") }}</span>
                <i class="material-icons">{{ sizeIcon }}</i>
              </p>
              <p
                :class="{ active: modifiedSorted }"
                class="modified"
                role="button"
                tabindex="0"
                @click="sort('modified')"
                :title="t('files.sortByLastModified')"
                :aria-label="t('files.sortByLastModified')"
              >
                <span>{{ t("files.lastModified") }}</span>
                <i class="material-icons">{{ modifiedIcon }}</i>
              </p>
            </div>
          </div>
        </div>

        <h2 v-if="fileStore.req?.numDirs ?? false">
          {{ t("files.folders") }}
        </h2>
        <div
          v-if="fileStore.req?.numDirs ?? false"
          @contextmenu="showContextMenu"
        >
          <item
            v-for="item in dirs"
            :key="base64(item.name)"
            v-bind:index="item.index"
            v-bind:name="item.name"
            v-bind:isDir="item.isDir"
            v-bind:url="item.url"
            v-bind:modified="item.modified"
            v-bind:type="item.type"
            v-bind:size="item.size"
            v-bind:path="item.path"
            v-bind:shared="item.shared"
            v-bind:ownerId="item.ownerId"
          >
          </item>
        </div>

        <h2 v-if="fileStore.req?.numFiles ?? false">
          {{ t("files.files") }}
        </h2>
        <div
          v-if="fileStore.req?.numFiles ?? false"
          @contextmenu="showContextMenu"
        >
          <item
            v-for="item in files"
            :key="base64(item.name)"
            v-bind:index="item.index"
            v-bind:name="item.name"
            v-bind:isDir="item.isDir"
            v-bind:url="item.url"
            v-bind:modified="item.modified"
            v-bind:type="item.type"
            v-bind:size="item.size"
            v-bind:path="item.path"
            v-bind:shared="item.shared"
            v-bind:ownerId="item.ownerId"
          >
          </item>
        </div>
        <context-menu
          :show="isContextMenuVisible"
          :pos="contextMenuPos"
          @hide="hideContextMenu"
        >
          <action
            v-if="headerButtons.rename"
            icon="mode_edit"
            :label="t('buttons.rename')"
            show="rename"
          />
          <action
            v-if="headerButtons.copy"
            id="copy-button"
            icon="content_copy"
            :label="t('buttons.copyFile')"
            show="copy"
          />
          <action
            v-if="headerButtons.move"
            id="move-button"
            icon="forward"
            :label="t('buttons.moveFile')"
            show="move"
          />
          <action
            v-if="headerButtons.delete"
            id="delete-button"
            icon="delete"
            :label="t('buttons.delete')"
            show="delete"
          />
          <action
            v-if="headerButtons.download"
            icon="file_download"
            :label="t('buttons.download')"
            @action="download"
            :counter="fileStore.selectedCount"
          />
          <action icon="info" :label="t('buttons.info')" :tooltip="t('tooltips.info')" show="info" />
          <template v-slot:file-context-actions>
            <ActionSlot name="file-context-actions" />
          </template>
          <template v-slot:file-context-menu>
            <ActionSlot name="file-context-menu" />
          </template>
        </context-menu>

        <!-- Blank area context menu for create/upload -->
        <context-menu
          :show="isBlankContextMenuVisible"
          :pos="blankContextMenuPos"
          @hide="hideBlankContextMenu"
        >
          <action
            icon="create_new_folder"
            :label="t('prompts.newDir')"
            @action="openNewDir"
          />
          <action
            icon="note_add"
            :label="t('prompts.newFile')"
            @action="openNewFile"
          />
          <action
            icon="upload_file"
            :label="t('prompts.uploadFile')"
            @action="triggerUploadFile"
          />
          <action
            icon="drive_folder_upload"
            :label="t('prompts.uploadFolder')"
            @action="triggerUploadFolder"
          />
        </context-menu>


        <div :class="{ active: fileStore.multiple }" id="multiple-selection">
          <p>{{ t("files.multipleSelectionEnabled") }}</p>
          <div
            @click="() => (fileStore.multiple = false)"
            tabindex="0"
            role="button"
            :title="t('buttons.clear')"
            :aria-label="t('buttons.clear')"
            class="action"
          >
            <i class="material-icons">clear</i>
          </div>
        </div>
      </div>
    </template>

    <floating-buttons />

    <input
      style="display: none"
      type="file"
      id="upload-input"
      @change="uploadInput($event)"
      multiple
    />
    <input
      style="display: none"
      type="file"
      id="upload-folder-input"
      @change="uploadInput($event)"
      webkitdirectory
      multiple
    />

    <ComponentSlot name="file-modals" />
  </div>
</template>

<script setup lang="ts">
import { useAuthStore } from "@/domains/auth";
import { useClipboardStore } from "@/app/stores/clipboard";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";

import { users, files as api } from "@/domains/files/api";
import * as upload from "@/domains/files/utils";
import css from "@/shared/utils/css";
import { throttle } from "lodash-es";
import { Base64 } from "js-base64";

import Breadcrumbs from "@/domains/files/components/Breadcrumbs.vue";
import Action from "@/shared/ui/header/Action.vue";
import Search from "@/domains/files/components/Search.vue";
import Item from "@/domains/files/components/files/ListingItem.vue";
import ContextMenu from "@/shared/ui/ContextMenu.vue";
import ActionSlot from "@/shared/ui/ActionSlot.vue";
import ComponentSlot from "@/shared/ui/ComponentSlot.vue";
import FloatingButtons from "@/domains/files/components/FloatingButtons.vue";
import {
  computed,
  inject,
  nextTick,
  onBeforeUnmount,
  onMounted,
  ref,
  watch,
} from "vue";
import { useRoute, onBeforeRouteUpdate } from "vue-router";
import { useI18n } from "vue-i18n";
import { storeToRefs } from "pinia";
import { removePrefix } from "@/domains/files/api";

const showLimit = ref<number>(50);
const columnWidth = ref<number>(280);
const dragCounter = ref<number>(0);
const width = ref<number>(window.innerWidth);
const itemWeight = ref<number>(0);
const isContextMenuVisible = ref<boolean>(false);
const contextMenuPos = ref<{ x: number; y: number }>({ x: 0, y: 0 });
const isBlankContextMenuVisible = ref<boolean>(false);
const blankContextMenuPos = ref<{ x: number; y: number }>({ x: 0, y: 0 });


const $showError = inject<IToastError>("$showError")!;
// const $showSuccess = inject<IToastSuccess>("$showSuccess")!;

const clipboardStore = useClipboardStore();
const authStore = useAuthStore();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();

const { req } = storeToRefs(fileStore);

const route = useRoute();
onBeforeRouteUpdate(() => {
  hideContextMenu();
  hideBlankContextMenu();
});

const { t } = useI18n();

const listing = ref<HTMLElement | null>(null);

const nameSorted = computed(() =>
  fileStore.req ? fileStore.req.sorting.by === "name" : false
);

const sizeSorted = computed(() =>
  fileStore.req ? fileStore.req.sorting.by === "size" : false
);

const modifiedSorted = computed(() =>
  fileStore.req ? fileStore.req.sorting.by === "modified" : false
);

const ascOrdered = computed(() =>
  fileStore.req ? fileStore.req.sorting.asc : false
);

const dirs = computed(() => items.value.dirs.slice(0, showLimit.value));

const items = computed(() => {
  const dirs: any[] = [];
  const files: any[] = [];

  fileStore.req?.items?.forEach((item) => {
    if (item.isDir) {
      dirs.push(item);
    } else {
      files.push(item);
    }
  });

  return { dirs, files };
});

const files = computed((): Resource[] => {
  let _showLimit = showLimit.value - items.value.dirs.length;

  if (_showLimit < 0) _showLimit = 0;

  return items.value.files.slice(0, _showLimit);
});

const selectedItems = computed(() => {
  if (!fileStore.req?.items) return [];
  return fileStore.selected.map((i) => fileStore.req?.items?.[i]).filter(Boolean);
});

const nameIcon = computed(() => {
  if (nameSorted.value && !ascOrdered.value) {
    return "arrow_upward";
  }

  return "arrow_downward";
});

const sizeIcon = computed(() => {
  if (sizeSorted.value && ascOrdered.value) {
    return "arrow_downward";
  }

  return "arrow_upward";
});

const modifiedIcon = computed(() => {
  if (modifiedSorted.value && ascOrdered.value) {
    return "arrow_downward";
  }

  return "arrow_upward";
});

const viewIcon = computed(() => {
  const icons = {
    list: "view_module",
    mosaic: "view_list",
    "mosaic gallery": "view_list",
  };
  return authStore.user === null
    ? icons["mosaic"]
    : icons[authStore.user.viewMode || "mosaic"];
});

const headerButtons = computed(() => {
  const isOwner = (item: any) =>
    item && (item.ownerId === undefined || item.ownerId === authStore.user?.id);
  const allOwned = selectedItems.value.every((item) => isOwner(item));

  return {
    upload: authStore.user?.perm.create,
    download: authStore.user?.perm.download,
    delete: fileStore.selectedCount > 0 && authStore.user?.perm.delete && allOwned,
    rename:
      fileStore.selectedCount === 1 && authStore.user?.perm.rename && allOwned,
    move: fileStore.selectedCount > 0 && authStore.user?.perm.rename && allOwned,
    copy: fileStore.selectedCount > 0 && authStore.user?.perm.create,
  };
});

const isMobile = computed(() => {
  return width.value <= 736;
});

watch(req, () => {
  // Reset the show value
  showLimit.value = 50;

  nextTick(() => {
    // Ensures that the listing is displayed
    // How much every listing item affects the window height
    setItemWeight();

    // Scroll to the item opened previously
    if (!revealPreviousItem()) {
      // Fill and fit the window with listing items
      fillWindow(true);
    }
  });
});

onMounted(() => {
  // Check the columns size for the first time.
  columnsResize();

  // How much every listing item affects the window height
  setItemWeight();

  // Scroll to the item opened previously
  if (!revealPreviousItem()) {
    // Fill and fit the window with listing items
    fillWindow(true);
  }

  // Add the needed event listeners to the window and document.
  window.addEventListener("keydown", keyEvent);
  window.addEventListener("scroll", scrollEvent);
  window.addEventListener("resize", windowsResize);

  // Global empty-area contextmenu handler -> show blank context menu
  const globalEmptyContextMenuHandler = (ev: any) => {
    const { x, y } = ev.detail || {};
    blankContextMenuPos.value = { x: x || 0, y: y || 0 };
    isBlankContextMenuVisible.value = true;
    isContextMenuVisible.value = false;
  };
  window.addEventListener('app:empty-contextmenu', globalEmptyContextMenuHandler);
  
  // Store handler reference for cleanup
  emptyMenuHandler = globalEmptyContextMenuHandler;

  if (!authStore.user?.perm.create) return;
  document.addEventListener("dragover", preventDefault);
  document.addEventListener("dragenter", dragEnter);
  document.addEventListener("dragleave", dragLeave);
  document.addEventListener("drop", drop);
});

// Component-local reference for cleanup
let emptyMenuHandler: ((ev: any) => void) | null = null;

onBeforeUnmount(() => {
  // Remove event listeners before destroying this page.
  window.removeEventListener("keydown", keyEvent);
  window.removeEventListener("scroll", scrollEvent);
  window.removeEventListener("resize", windowsResize);
  
  // Remove global empty menu handler
  if (emptyMenuHandler) {
    window.removeEventListener('app:empty-contextmenu', emptyMenuHandler);
    emptyMenuHandler = null;
  }

  if (authStore.user && !authStore.user?.perm.create) return;
  document.removeEventListener("dragover", preventDefault);
  document.removeEventListener("dragenter", dragEnter);
  document.removeEventListener("dragleave", dragLeave);
  document.removeEventListener("drop", drop);
});

const base64 = (name: string) => Base64.encodeURI(name);

const keyEvent = (event: KeyboardEvent) => {
  // No prompts are shown
  if (layoutStore.currentPrompt !== null) {
    return;
  }

  if (event.key === "Escape") {
    // Reset files selection.
    fileStore.selected = [];
  }

  if (event.key === "Delete") {
    if (!authStore.user?.perm.delete || fileStore.selectedCount == 0) return;

    // Show delete prompt.
    layoutStore.showHover("delete");
  }

  if (event.key === "F2") {
    if (!authStore.user?.perm.rename || fileStore.selectedCount !== 1) return;

    // Show rename prompt.
    layoutStore.showHover("rename");
  }

  // Ctrl is pressed
  if (!event.ctrlKey && !event.metaKey) {
    return;
  }

  switch (event.key) {
    case "f":
    case "F":
      if (event.shiftKey) {
        event.preventDefault();
        layoutStore.showHover("search");
      }
      break;
    case "c":
    case "x":
      copyCut(event);
      break;
    case "v":
      paste(event);
      break;
    case "a":
      event.preventDefault();
      for (const file of items.value.files) {
        if (fileStore.selected.indexOf(file.index) === -1) {
          fileStore.selected.push(file.index);
        }
      }
      for (const dir of items.value.dirs) {
        if (fileStore.selected.indexOf(dir.index) === -1) {
          fileStore.selected.push(dir.index);
        }
      }
      break;
    case "s":
      event.preventDefault();
      document.getElementById("download-button")?.click();
      break;
  }
};

const preventDefault = (event: Event) => {
  // Wrapper around prevent default.
  event.preventDefault();
};

const copyCut = (event: Event | KeyboardEvent): void => {
  if ((event.target as HTMLElement).tagName?.toLowerCase() === "input") return;

  if (fileStore.req === null) return;

  const itemsList = fileStore.req?.items || [];
  const itemsToCopy = [];

  for (const i of fileStore.selected) {
    if (itemsList[i]) {
      itemsToCopy.push({
        from: itemsList[i].url,
        name: itemsList[i].name,
      });
    }
  }

  if (itemsToCopy.length === 0) {
    return;
  }

  clipboardStore.$patch({
    key: (event as KeyboardEvent).key,
    items: itemsToCopy,
    path: route.path,
  });
};

const paste = (event: Event) => {
  if ((event.target as HTMLElement).tagName?.toLowerCase() === "input") return;

  // TODO router location should it be
  const items: any[] = [];

  for (const item of clipboardStore.items) {
    const from = item.from.endsWith("/") ? item.from.slice(0, -1) : item.from;
    const to = route.path + encodeURIComponent(item.name);
    items.push({ from, to, name: item.name });
  }

  if (items.length === 0) {
    return;
  }

  const preselect = removePrefix(route.path) + items[0].name;

  let action = (overwrite: boolean, rename: boolean) => {
    api
      .copy(items, overwrite, rename)
      .then(() => {
        fileStore.preselect = preselect;
        fileStore.reload = true;
      })
      .catch($showError);
  };

  if (clipboardStore.key === "x") {
    action = (overwrite, rename) => {
      api
        .move(items, overwrite, rename)
        .then(() => {
          clipboardStore.resetClipboard();
          fileStore.preselect = preselect;
          fileStore.reload = true;
        })
        .catch($showError);
    };
  }

  if (clipboardStore.path == route.path) {
    action(false, true);

    return;
  }

  const conflict = upload.checkConflict(items, fileStore.req?.items || []);

  let overwrite = false;
  let rename = false;

  if (conflict) {
    layoutStore.showHover({
      prompt: "replace-rename",
      confirm: (event: Event, option: string) => {
        overwrite = option == "overwrite";
        rename = option == "rename";

        event.preventDefault();
        layoutStore.closeHovers();
        action(overwrite, rename);
      },
    });

    return;
  }

  action(overwrite, rename);
};

const columnsResize = () => {
  // Update the columns size based on the window width.
  const items_ = css(["#listing.mosaic .item", ".mosaic#listing .item"]);
  if (items_ === null) return;

  let columns = Math.floor(
    (document.querySelector("main")?.offsetWidth ?? 0) / columnWidth.value
  );
  if (columns === 0) columns = 1;
  items_.style.width = `calc(${100 / columns}% - 1em)`;
};

const scrollEvent = throttle(() => {
  const totalItems =
    (fileStore.req?.numDirs ?? 0) + (fileStore.req?.numFiles ?? 0);

  // All items are displayed
  if (showLimit.value >= totalItems) return;

  const currentPos = window.innerHeight + window.scrollY;

  // Trigger at the 75% of the window height
  const triggerPos = document.body.offsetHeight - window.innerHeight * 0.25;

  if (currentPos > triggerPos) {
    // Quantity of items needed to fill 2x of the window height
    const showQuantity = Math.ceil((window.innerHeight * 2) / itemWeight.value);

    // Increase the number of displayed items
    showLimit.value += showQuantity;
  }
}, 100);

const dragEnter = () => {
  dragCounter.value++;

  // When the user starts dragging an item, put every
  // file on the listing with 50% opacity.
  const items = document.getElementsByClassName("item");

  Array.from(items).forEach((file: Element) => {
    (file as HTMLElement).style.opacity = "0.5";
  });
};

const dragLeave = () => {
  dragCounter.value--;

  if (dragCounter.value == 0) {
    resetOpacity();
  }
};

const drop = async (event: DragEvent) => {
  event.preventDefault();
  dragCounter.value = 0;
  resetOpacity();

  const dt = event.dataTransfer;
  let el: HTMLElement | null = event.target as HTMLElement;

  if (fileStore.req === null || dt === null || dt.files.length <= 0) return;

  for (let i = 0; i < 5; i++) {
    if (el !== null && !el.classList.contains("item")) {
      el = el.parentElement;
    }
  }

  const files: UploadList = (await upload.scanFiles(dt)) as UploadList;
  let items = fileStore.req?.items || [];
  let path = route.path.endsWith("/") ? route.path : route.path + "/";

  if (
    el !== null &&
    el.classList.contains("item") &&
    el.dataset.dir === "true"
  ) {
    // Get url from ListingItem instance
    // TODO: Don't know what is happening here
    path = el.__vue__.url;

    try {
      items = (await api.fetch(path)).items || [];
    } catch (error: any) {
      $showError(error);
    }
  }

  const conflict = upload.checkConflict(files, items);

  const preselect = removePrefix(path) + (files[0].fullPath || files[0].name);

  if (conflict) {
    layoutStore.showHover({
      prompt: "replace",
      action: (event: Event) => {
        event.preventDefault();
        layoutStore.closeHovers();
        upload.handleFiles(files, path, false);
        fileStore.preselect = preselect;
      },
      confirm: (event: Event) => {
        event.preventDefault();
        layoutStore.closeHovers();
        upload.handleFiles(files, path, true);
        fileStore.preselect = preselect;
      },
    });

    return;
  }

  upload.handleFiles(files, path);
  fileStore.preselect = preselect;
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

  const path = route.path.endsWith("/") ? route.path : route.path + "/";
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

const resetOpacity = () => {
  const items = document.getElementsByClassName("item");

  Array.from(items).forEach((file: Element) => {
    (file as HTMLElement).style.opacity = "1";
  });
};

const sort = async (by: string) => {
  let asc = false;

  if (by === "name") {
    if (nameIcon.value === "arrow_upward") {
      asc = true;
    }
  } else if (by === "size") {
    if (sizeIcon.value === "arrow_upward") {
      asc = true;
    }
  } else if (by === "modified") {
    if (modifiedIcon.value === "arrow_upward") {
      asc = true;
    }
  }

  try {
    if (authStore.user?.id) {
      await users.update({ id: authStore.user?.id, sorting: { by, asc } }, [
        "sorting",
      ]);
    }
  } catch (e: any) {
    $showError(e);
  }

  fileStore.reload = true;
};



const toggleMultipleSelection = () => {
  fileStore.toggleMultiple();
  layoutStore.closeHovers();
};

const windowsResize = throttle(() => {
  columnsResize();
  width.value = window.innerWidth;

  // Listing element is not displayed
  if (listing.value == null) return;

  // How much every listing item affects the window height
  setItemWeight();

  // Fill but not fit the window
  fillWindow();
}, 100);

const download = () => {
  if (fileStore.req === null) return;

  if (fileStore.selectedCount === 1) {
    const selectedItem = fileStore.req?.items?.[fileStore.selected[0]];
    if (selectedItem && !selectedItem.isDir) {
      api.download(null, selectedItem.url);
      return;
    }
  }

  layoutStore.showHover({
    prompt: "download",
    confirm: (format: any) => {
      layoutStore.closeHovers();

      const files = [];

      if (fileStore.selectedCount > 0 && fileStore.req !== null) {
        for (const i of fileStore.selected) {
          if (fileStore.req?.items?.[i]) {
            files.push(fileStore.req.items[i].url);
          }
        }
      } else {
        files.push(route.path);
      }

      api.download(format, ...files);
    },
  });
};

const switchView = async () => {
  layoutStore.closeHovers();

  const modes = {
    list: "mosaic",
    mosaic: "list",
    "mosaic gallery": "list",
  };

  const data = {
    id: authStore.user?.id,
    viewMode: (modes[authStore.user?.viewMode ?? "mosaic"] ||
      "list") as ViewModeType,
  };

  users.update(data, ["viewMode"]).catch($showError);

  authStore.updateUser(data);

  setItemWeight();
  fillWindow();
};

/*
const uploadFunc = () => {
  if (
    typeof window.DataTransferItem !== "undefined" &&
    typeof DataTransferItem.prototype.webkitGetAsEntry !== "undefined"
  ) {
    layoutStore.showHover("upload");
  } else {
    document.getElementById("upload-input")?.click();
  }
};
*/

const setItemWeight = () => {
  // Listing element is not displayed
  if (listing.value === null || fileStore.req === null) return;

  let itemQuantity = fileStore.req.numDirs + fileStore.req.numFiles;
  if (itemQuantity > showLimit.value) itemQuantity = showLimit.value;

  // How much every listing item affects the window height
  itemWeight.value = listing.value.offsetHeight / itemQuantity;
};

const fillWindow = (fit = false) => {
  if (fileStore.req === null) return;

  const totalItems = fileStore.req.numDirs + fileStore.req.numFiles;

  // More items are displayed than the total
  if (showLimit.value >= totalItems && !fit) return;

  const windowHeight = window.innerHeight;

  // Quantity of items needed to fill 2x of the window height
  const showQuantity = Math.ceil(
    (windowHeight + windowHeight * 2) / itemWeight.value
  );

  // Less items to display than current
  if (showLimit.value > showQuantity && !fit) return;

  // Set the number of displayed items
  showLimit.value = showQuantity > totalItems ? totalItems : showQuantity;
};

const revealPreviousItem = () => {
  if (!fileStore.req || !fileStore.oldReq) return;

  const index = fileStore.selected[0];
  if (index === undefined) return;

  showLimit.value =
    index + Math.ceil((window.innerHeight * 2) / itemWeight.value);

  nextTick(() => {
    const items = document.querySelectorAll("#listing .item");
    items[index].scrollIntoView({ block: "center" });
  });

  return true;
};

const showContextMenu = (event: MouseEvent) => {
  event.preventDefault();
  isContextMenuVisible.value = true;
  isBlankContextMenuVisible.value = false;
  contextMenuPos.value = {
    x: event.clientX,
    y: event.clientY,
  };
};

const hideContextMenu = () => {
  isContextMenuVisible.value = false;
};

const showBlankContextMenu = (event: MouseEvent) => {
  // 只在点击空白处时显示，不在文件/文件夹上
  const target = event.target as HTMLElement;
  // 检查是否点击在文件项上（排除 header 和 listing 容器本身）
  const isOnItem = target.closest('.item:not(.header)');
  // const isOnHeader = target.closest('.item.header');
  // const isOnH2 = target.closest('h2');
  
  // 如果点击在文件项上，不显示空白菜单
  if (isOnItem) {
    return;
  }
  
  event.preventDefault();
  isBlankContextMenuVisible.value = true;
  isContextMenuVisible.value = false;
  blankContextMenuPos.value = {
    x: event.clientX,
    y: event.clientY,
  };
};

const hideBlankContextMenu = () => {
  isBlankContextMenuVisible.value = false;
};

const triggerUploadFile = () => {
  hideBlankContextMenu();
  const input = document.getElementById("upload-input") as HTMLInputElement;
  if (input) {
    input.click();
  }
};

const triggerUploadFolder = () => {
  hideBlankContextMenu();
  const input = document.getElementById("upload-folder-input") as HTMLInputElement;
  if (input) {
    input.click();
  }
};

const openNewDir = () => {
  hideBlankContextMenu();
  layoutStore.showHover("newDir");
};

const openNewFile = () => {
  hideBlankContextMenu();
  layoutStore.showHover("newFile");
};

</script>
