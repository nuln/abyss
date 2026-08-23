<template>
  <div class="floating-buttons">
    <!-- Scroll to top button -->
    <button
      v-show="showScrollTop"
      class="floating-btn scroll-top"
      @click="scrollToTop"
      :aria-label="t('buttons.scrollToTop')"
      :title="t('buttons.scrollToTop')"
    >
      <i class="material-icons">arrow_upward</i>
    </button>

    <!-- Search button -->
    <button
      class="floating-btn search-btn"
      @click="toggleSearch"
      :aria-label="t('buttons.search')"
      :title="t('buttons.search')"
    >
      <i class="material-icons">search</i>
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted, onUnmounted } from "vue";
import { useLayoutStore } from "@/app/stores/layout";
import { useI18n } from "vue-i18n";

const layoutStore = useLayoutStore();
const { t } = useI18n();

const showScrollTop = ref(false);

const handleScroll = () => {
  showScrollTop.value = window.scrollY > 300;
};

const scrollToTop = () => {
  window.scrollTo({ top: 0, behavior: "smooth" });
};

const toggleSearch = () => {
  layoutStore.showHover("search");
};

onMounted(() => {
  window.addEventListener("scroll", handleScroll);
});

onUnmounted(() => {
  window.removeEventListener("scroll", handleScroll);
});
</script>

<style scoped>
.floating-buttons {
  position: fixed;
  bottom: var(--space-6);
  right: var(--space-6);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  z-index: var(--z-fixed);
}

.floating-btn {
  width: 48px;
  height: 48px;
  border-radius: 50%;
  border: none;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  transition: all var(--transition-fast);
  background: var(--surface-card);
  color: var(--text-secondary);
}

.floating-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.2);
  color: var(--text-primary);
}

.floating-btn i {
  font-size: 24px;
}

.scroll-top {
  opacity: 0;
  animation: fadeIn 0.2s ease forwards;
}

.search-btn {
  background: var(--surface-card);
  border: 1px solid var(--border-subtle);
}

.search-btn i {
  color: var(--text-primary);
}

.search-btn:hover {
  background: var(--surface-card-hover, var(--surface-card));
}

.search-btn:hover i {
  color: var(--text-primary);
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(10px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
</style>
