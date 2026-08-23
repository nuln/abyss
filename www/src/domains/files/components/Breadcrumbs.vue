<template>
  <div class="breadcrumbs-container">
    <div class="breadcrumbs">
      <!-- Home icon -->
      <router-link
        :to="homeLink || '/files/'"
        :aria-label="t('files.home')"
        :title="t('files.home')"
        class="breadcrumb-item home"
      >
        <i class="material-icons">home</i>
      </router-link>

      <!-- Base title link (shown before path segments if provided) -->
      <template v-if="props.baseTitle">
        <span class="chevron">
          <i class="material-icons">keyboard_arrow_right</i>
        </span>
        <router-link 
          :to="props.base + '/'"
          class="breadcrumb-item"
          :class="{ current: visibleItems.length === 0 && !props.title }"
        >
          {{ props.baseTitle }}
        </router-link>
      </template>
      <!-- Static title (for pages without path segments) -->
      <template v-if="props.title && visibleItems.length === 0">
        <span class="chevron">
          <i class="material-icons">keyboard_arrow_right</i>
        </span>
        <span class="breadcrumb-item current">{{ props.title }}</span>
      </template>

      <!-- Collapsed items indicator (...) -->
      <template v-if="collapsedItems.length > 0">
        <span class="chevron">
          <i class="material-icons">keyboard_arrow_right</i>
        </span>
        <div class="collapsed-menu" ref="collapsedMenuRef">
          <button 
            class="collapsed-trigger breadcrumb-item"
            @click.stop="toggleCollapsedMenu"
            :aria-expanded="showCollapsedMenu"
            aria-haspopup="true"
          >
            <span>...</span>
          </button>
          <div 
            class="collapsed-dropdown"
            v-show="showCollapsedMenu"
          >
            <!-- Collapsed folder items -->
            <component
              v-for="(link, index) in collapsedItems"
              :key="'collapsed-' + index"
              :is="element"
              :to="link.url"
              class="dropdown-item"
              @click="showCollapsedMenu = false"
            >
              <i class="material-icons">folder</i>
              <span>{{ link.name }}</span>
            </component>
          </div>
        </div>
      </template>

      <!-- Visible items (always shown) -->
      <span v-for="(link, index) in visibleItems" :key="'visible-' + index" class="breadcrumb-visible-item">
        <span class="chevron">
          <i class="material-icons">keyboard_arrow_right</i>
        </span>
        <component 
          :is="element" 
          :to="link.url"
          class="breadcrumb-item"
          :class="{ current: index === visibleItems.length - 1 }"
        >
          {{ link.name }}
        </component>
      </span>
    </div>

    <!-- Slot for actions on the right side -->
    <div class="breadcrumbs-actions">
      <slot name="actions"></slot>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onUnmounted } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";

const { t } = useI18n();

const route = useRoute();
const showCollapsedMenu = ref(false);
const collapsedMenuRef = ref<HTMLElement | null>(null);

const props = defineProps<{
  base: string;
  title?: string;
  baseTitle?: string;
  noLink?: boolean;
  maxVisible?: number;
  homeLink?: string;
}>();

// Maximum visible items (default 2: current folder and parent)
const maxVisibleCount = computed(() => props.maxVisible ?? 2);

// Build all breadcrumb items
const allItems = computed(() => {
  const relativePath = route.path.replace(props.base, "");
  const parts = relativePath.split("/");

  if (parts[0] === "") {
    parts.shift();
  }

  if (parts[parts.length - 1] === "") {
    parts.pop();
  }

  const breadcrumbs: BreadCrumb[] = [];
  
  // Check if this is an albums path (no trailing slash needed)
  const isAlbumsPath = props.base === "/albums";

  for (let i = 0; i < parts.length; i++) {
    const trailingSlash = isAlbumsPath ? "" : "/";
    if (i === 0) {
      breadcrumbs.push({
        name: decodeURIComponent(parts[i]),
        url: props.base + "/" + parts[i] + trailingSlash,
      });
    } else {
      const prevUrl = isAlbumsPath ? breadcrumbs[i - 1].url : breadcrumbs[i - 1].url;
      breadcrumbs.push({
        name: decodeURIComponent(parts[i]),
        url: prevUrl + (isAlbumsPath ? "/" : "") + parts[i] + trailingSlash,
      });
    }
  }

  return breadcrumbs;
});

// Items that are collapsed (shown in dropdown)
const collapsedItems = computed(() => {
  if (allItems.value.length <= maxVisibleCount.value) {
    return [];
  }
  return allItems.value.slice(0, allItems.value.length - maxVisibleCount.value);
});

// Items that are always visible
const visibleItems = computed(() => {
  if (allItems.value.length <= maxVisibleCount.value) {
    return allItems.value;
  }
  return allItems.value.slice(allItems.value.length - maxVisibleCount.value);
});

const element = computed(() => {
  if (props.noLink) {
    return "span";
  }
  return "router-link";
});

const toggleCollapsedMenu = () => {
  showCollapsedMenu.value = !showCollapsedMenu.value;
};

// Close dropdown when clicking outside
const handleClickOutside = (event: MouseEvent) => {
  if (collapsedMenuRef.value && !collapsedMenuRef.value.contains(event.target as Node)) {
    showCollapsedMenu.value = false;
  }
};

onMounted(() => {
  document.addEventListener("click", handleClickOutside);
});

onUnmounted(() => {
  document.removeEventListener("click", handleClickOutside);
});
</script>

<style scoped>
.breadcrumbs-container {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 3em;
  background: var(--surface-page);
  border-bottom: 1px solid var(--border-subtle);
  position: sticky;
  z-index: 100;
  top: 0;
  padding: 0 var(--space-4);
}

.breadcrumbs {
  display: flex;
  align-items: center;
  color: var(--text-secondary);
  font-size: var(--text-sm);
  flex: 1;
  min-width: 0;
  overflow: visible;
  height: auto;
  background: transparent;
  border-bottom: none;
  position: static;
}

.breadcrumb-visible-item {
  display: inline-flex;
  align-items: center;
}

.breadcrumb-item {
  display: inline-flex;
  align-items: center;
  color: inherit;
  transition: all var(--transition-fast);
  border-radius: var(--radius-sm);
  padding: var(--space-1) var(--space-2);
  white-space: nowrap;
  text-decoration: none;
}

.breadcrumb-item:hover {
  background-color: var(--color-gray-100);
  color: var(--text-primary);
}

.breadcrumb-item.current {
  color: var(--text-primary);
  font-weight: 500;
}

.breadcrumb-item.home {
  padding: var(--space-1);
  margin-left: var(--space-2);
}

.breadcrumb-item.home i {
  font-size: 1.25rem;
}

.chevron {
  display: flex;
  align-items: center;
  color: var(--text-tertiary);
}

.chevron i {
  font-size: 1.125rem;
}

/* Collapsed menu styles */
.collapsed-menu {
  position: relative;
  display: inline-flex;
  align-items: center;
}

.collapsed-trigger {
  background: none;
  border: none;
  cursor: pointer;
  font-family: inherit;
  font-size: inherit;
}

.collapsed-trigger:hover {
  background-color: var(--color-gray-100);
}

.collapsed-dropdown {
  position: absolute;
  top: 100%;
  left: 0;
  margin-top: var(--space-1);
  background: var(--surface-card);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  min-width: 200px;
  max-width: 300px;
  max-height: 400px;
  overflow-y: auto;
  z-index: 9999;
}

.dropdown-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  color: var(--text-primary);
  text-decoration: none;
  transition: background-color var(--transition-fast);
}

.dropdown-item:hover {
  background-color: var(--color-gray-50);
}

.dropdown-item i {
  color: var(--text-tertiary);
  font-size: 1.125rem;
}

.dropdown-item span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Actions area */
.breadcrumbs-actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-shrink: 0;
}

.breadcrumbs-actions :deep(.action) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: none;
  border: none;
  cursor: pointer;
  padding: var(--space-1);
  border-radius: var(--radius-sm);
  color: var(--text-secondary);
  transition: all var(--transition-fast);
}

.breadcrumbs-actions :deep(.action:hover) {
  background-color: var(--color-gray-100);
  color: var(--text-primary);
}

.breadcrumbs-actions :deep(.action i) {
  font-size: 1.25rem;
}

.breadcrumbs-actions :deep(.action span) {
  display: none;
}

.breadcrumbs-actions :deep(.action .counter) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 1.25rem;
  height: 1.25rem;
  padding: 0 0.25rem;
  background: var(--color-accent);
  color: white;
  font-size: 0.75rem;
  font-weight: 600;
  border-radius: 999px;
  margin-left: var(--space-1);
}

/* RTL Support */
html[dir="rtl"] .chevron i {
  transform: scaleX(-1);
}

html[dir="rtl"] .collapsed-dropdown {
  left: auto;
  right: 0;
}
</style>

