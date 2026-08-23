<template>
  <div class="folder-tree">
    <div v-if="loading && depth === 0" class="folder-loading">
      <div class="spinner small"></div>
    </div>
    <div v-else-if="error && depth === 0" class="folder-error">
      {{ error }}
    </div>
    <template v-else>
      <div
        v-for="folder in folders"
        :key="folder.name"
        class="folder-item-wrapper"
      >
        <div
          class="folder-item"
          :style="{ paddingLeft: depth * 12 + 'px' }"
          @click="navigateToFolder(folder)"
        >
          <button
            v-if="folder.hasSubfolders !== false"
            class="folder-arrow"
            :class="{ expanded: folder.expanded }"
            @click.stop="toggleExpand(folder)"
            :aria-label="folder.expanded ? t('buttons.collapse') : t('buttons.expand')"
          >
            <i class="material-icons">chevron_right</i>
          </button>
          <span v-else class="folder-arrow-placeholder"></span>
          <i class="material-icons folder-icon">folder</i>
          <span class="folder-name">{{ folder.name }}</span>
        </div>
        <transition name="folder-children-expand">
          <div v-if="folder.expanded" class="folder-children">
            <FolderTree
              v-if="folder.subfolders"
              :folders="folder.subfolders"
              :depth="depth + 1"
              :basePath="folder.path"
              @navigate="$emit('navigate', $event)"
            />
            <div v-else-if="folder.loadingChildren" class="folder-loading" :style="{ paddingLeft: (depth + 1) * 12 + 'px' }">
              <div class="spinner small"></div>
            </div>
          </div>
        </transition>
      </div>
    </template>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, watch } from "vue";
import { files as api } from "@/domains/files/api";
import { useRouter } from "vue-router";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

interface FolderItem {
  name: string;
  path: string;
  expanded: boolean;
  subfolders?: FolderItem[];
  loadingChildren?: boolean;
  hasSubfolders?: boolean;
}

const props = withDefaults(
  defineProps<{
    folders?: any[];
    depth?: number;
    basePath?: string;
  }>(),
  {
    depth: 0,
    basePath: "/",
  }
);

const emit = defineEmits<{
  (e: "navigate", path: string): void;
}>();

const router = useRouter();
const folders = ref<FolderItem[]>(props.folders || []);
const loading = ref(false);
const error = ref<string | null>(null);

const fetchFolders = async (path: string): Promise<FolderItem[]> => {
  const data = await api.fetch(`/files${path}`);
  if (data.isDir && data.items) {
    return data.items
      .filter((item: any) => item.isDir)
      .map((item: any) => ({
        name: item.name,
        path: path === "/" ? `/${encodeURIComponent(item.name)}/` : `${path}${encodeURIComponent(item.name)}/`,
        expanded: false,
        hasSubfolders: true, // Assume has subfolders, will check on expand
      }));
  }
  return [];
};

const toggleExpand = async (folder: FolderItem) => {
  if (folder.expanded) {
    folder.expanded = false;
    return;
  }

  folder.expanded = true;
  // Always refresh subfolders when expanding
  folder.loadingChildren = true;
  try {
    folder.subfolders = await fetchFolders(folder.path);
    if (folder.subfolders.length === 0) {
      folder.hasSubfolders = false;
    }
  } catch (_e) {
    folder.subfolders = [];
    folder.hasSubfolders = false;
  } finally {
    folder.loadingChildren = false;
  }
};

const navigateToFolder = (folder: FolderItem) => {
  if (!folder.expanded) {
    toggleExpand(folder);
  }
  router.push(`/files${folder.path.split("/").map(encodeURIComponent).join("/")}`);
  emit("navigate", folder.path);
};

onMounted(async () => {
  // Only fetch if we're the root level and no folders were passed
  if (props.depth === 0 && !props.folders) {
    loading.value = true;
    try {
      folders.value = await fetchFolders(props.basePath);
    } catch (_e) {
      error.value = t("errors.loadFailed");
    } finally {
      loading.value = false;
    }
  }
});

// Watch for prop changes
watch(
  () => props.folders,
  (newFolders) => {
    if (newFolders) {
      folders.value = newFolders;
    }
  }
);
</script>

<style scoped>
.folder-tree {
  width: 100%;
}

.folder-item-wrapper {
  width: 100%;
}

.folder-item {
  display: flex;
  align-items: center;
  padding: 2px var(--space-2);
  cursor: pointer;
  transition: background-color var(--transition-fast);
  border-radius: var(--radius-sm);
  margin: 0;
  user-select: none;
}

.folder-item:hover {
  background-color: var(--color-gray-100);
}

.folder-arrow {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  padding: 0;
  border: none;
  background: transparent;
  cursor: pointer;
  transition: transform var(--transition-fast);
  flex-shrink: 0;
}

.folder-arrow i {
  font-size: 18px;
  color: var(--text-tertiary);
  transition: transform var(--transition-fast);
}

.folder-arrow.expanded i {
  transform: rotate(90deg);
}

.folder-arrow:hover i {
  color: var(--text-primary);
}

.folder-arrow-placeholder {
  width: 20px;
  height: 20px;
  flex-shrink: 0;
}

.folder-icon {
  font-size: 20px;
  color: var(--text-tertiary);
  margin-right: var(--space-1);
  flex-shrink: 0;
}

.folder-name {
  font-size: 0.9rem;
  color: var(--text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-weight: 500;
}

.folder-children {
  width: 100%;
}

.folder-loading {
  display: flex;
  align-items: center;
  padding: var(--space-1) var(--space-2);
}

.folder-error {
  padding: var(--space-2);
  color: var(--color-red);
  font-size: var(--text-sm);
}

.spinner.small {
  width: 16px;
  height: 16px;
}

.spinner.small .bounce1,
.spinner.small .bounce2,
.spinner.small .bounce3 {
  width: 6px;
  height: 6px;
}

/* Folder children expand/collapse animation */
.folder-children-expand-enter-active,
.folder-children-expand-leave-active {
  transition: all 0.2s ease-out;
  overflow: hidden;
}

.folder-children-expand-enter-from,
.folder-children-expand-leave-to {
  opacity: 0;
  max-height: 0;
  transform: translateY(-4px);
}

.folder-children-expand-enter-to,
.folder-children-expand-leave-from {
  opacity: 1;
  max-height: 500px;
  transform: translateY(0);
}
</style>
