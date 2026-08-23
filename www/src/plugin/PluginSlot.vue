<template>
  <div class="plugin-slot">
    <component
      v-for="(entry, index) in components"
      :key="index"
      :is="entry.component"
      v-bind="getProps(entry)"
      v-on="entry.events || {}"
    />
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { getSlotComponents, type SlotEntry } from "./slots";

const props = defineProps<{
  name: string;
  slotProps?: Record<string, any>;
}>();

const components = computed(() => getSlotComponents(props.name));

const getProps = (entry: SlotEntry) => {
  const baseProps = typeof entry.props === "function" ? entry.props() : entry.props || {};
  return { ...baseProps, ...props.slotProps };
};
</script>
