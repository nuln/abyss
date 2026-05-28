<template>
  <div>
    <ul class="file-list">
      <li
        @click="itemClick"
        @touchstart="touchstart"
        @dblclick="next"
        role="button"
        tabindex="0"
        :aria-label="item.name"
        :aria-selected="selected == item.url"
        :key="item.name"
        v-for="item in items"
        :data-url="item.url"
      >
        {{ item.name }}
      </li>
    </ul>

    <p>
      {{ t("prompts.currentlyNavigating") }} <code>{{ nav }}</code>.
    </p>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, inject } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";
import { useAuthStore } from "@/domains/auth";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";
import url from "@/shared/utils/url";
import { files } from "@/domains/files/api";
import { StatusError } from "@/domains/files/api";

const { t } = useI18n();
const route = useRoute();
const authStore = useAuthStore();
const fileStore = useFileStore();
const layoutStore = useLayoutStore();
const $showError = inject<IToastError>("$showError")!;

const props = withDefaults(defineProps<{
  exclude?: string[];
}>(), {
  exclude: () => [],
});

const emit = defineEmits<{
  (e: "update:selected", val: string): void;
}>();

interface FileListItem {
  name: string;
  url: string;
}

const items = ref<FileListItem[]>([]);
const selected = ref<string | null>(null);
const current = ref(window.location.pathname);
let nextAbortController = new AbortController();

const nav = computed(() => decodeURIComponent(current.value));

const fillOptions = (req: any) => {
  current.value = req.url;
  items.value = [];

  emit("update:selected", current.value);

  if (req.url !== "/files/") {
    items.value.push({
      name: "..",
      url: url.removeLastDir(req.url) + "/",
    });
  }

  if (req === null || req.items === null) return;

  for (const item of req.items) {
    if (!item.isDir) continue;
    if (props.exclude?.includes(item.url)) continue;

    items.value.push({
      name: item.name,
      url: item.url,
    });
  }
};

const next = (event: Event) => {
  const uri = (event.currentTarget as HTMLElement).dataset.url!;
  nextAbortController.abort();
  nextAbortController = new AbortController();
  files
    .fetch(uri, nextAbortController.signal)
    .then(fillOptions)
    .catch((e: any) => {
      if (e instanceof StatusError && e.is_canceled) {
        return;
      }
      $showError(e);
    });
};

const touchstart = (event: TouchEvent) => {
  // Touch handling for mobile double-tap navigation
  const el = event.currentTarget as HTMLElement;
  const touchUrl = el.dataset.url!;
  const now = Date.now();

  if (el.dataset.lastTouch && touchUrl === el.dataset.lastTouchUrl) {
    const elapsed = now - parseInt(el.dataset.lastTouch);
    if (elapsed < 300) {
      next(event);
      return;
    }
  }

  el.dataset.lastTouch = String(now);
  el.dataset.lastTouchUrl = touchUrl;
};

const itemClick = (event: Event) => {
  if (authStore.user?.singleClick) next(event);
  else select(event);
};

const select = (event: Event) => {
  const clickedUrl = (event.currentTarget as HTMLElement).dataset.url!;

  if (selected.value === clickedUrl) {
    selected.value = null;
    emit("update:selected", current.value);
    return;
  }

  selected.value = clickedUrl;
  emit("update:selected", selected.value);
};

const createDir = () => {
  layoutStore.showHover({
    prompt: "newDir",
    action: undefined,
    confirm: undefined,
    props: {
      redirect: false,
      base: current.value === route.path ? null : current.value,
    },
  });
};

onMounted(() => {
  fillOptions(fileStore.req);
});

onUnmounted(() => {
  nextAbortController.abort();
});

defineExpose({ createDir });
</script>
