<template>
  <div class="toast-content">
    <div class="toast-icon">
      <i class="material-icons">{{ icon }}</i>
    </div>
    <div class="toast-message">
      {{ message }}
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";

const props = defineProps<{
  message: string;
  type?: "success" | "error" | "warning" | "info";
}>();

const icon = computed(() => {
  switch (props.type) {
    case "success":
      return "check_circle";
    case "error":
      return "error_outline";
    case "warning":
      return "warning_amber";
    default:
      return "info_outline";
  }
});
</script>

<style scoped>
.toast-content {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  width: 100%;
}

.toast-icon {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding-top: 1px;
}

.toast-icon i {
  font-size: 1.25rem;
}

.toast-message {
  flex: 1;
  font-size: var(--text-sm);
  font-weight: 500;
  line-height: var(--leading-normal);
  padding-top: 1px;
}
</style>
