<template>
  <button @click="action" :aria-label="typeof label === 'function' ? label() : label" :title="(typeof tooltip === 'function' ? tooltip() : tooltip) || (typeof label === 'function' ? label() : label)" :data-tooltip="(typeof tooltip === 'function' ? tooltip() : tooltip) || (typeof label === 'function' ? label() : label)" class="action">
    <i class="material-icons" :title="(typeof tooltip === 'function' ? tooltip() : tooltip) || (typeof label === 'function' ? label() : label)">{{ icon }}</i>
    <span>{{ typeof label === 'function' ? label() : label }}</span>
    <span v-if="counter && counter > 0" class="counter">{{ counter }}</span>
  </button>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/app/stores/layout";

const props = defineProps<{
  icon?: string;
  label?: string | (() => string);
  tooltip?: string | (() => string);
  counter?: number;
  show?: string;
}>();

const emit = defineEmits<{
  (e: "action"): any;
}>();

const layoutStore = useLayoutStore();

const action = () => {
  if (props.show) {
    layoutStore.showHover(props.show);
  }

  emit("action");
};
</script>
