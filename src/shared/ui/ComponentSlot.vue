<template>
  <template v-for="(entry, index) in slotEntries" :key="index">
    <component
      :is="entry.component"
      v-bind="typeof entry.props === 'function' ? entry.props() : (entry.props || {})"
      v-on="entry.events || {}"
    />
  </template>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { getSlotComponents } from "@/plugin/slots";

const props = defineProps<{
  name: string;
}>();

interface SlotEntry {
  component: any;
  props?: (() => Record<string, any>) | Record<string, any>;
  events?: Record<string, (...args: any[]) => void>;
}

const slotEntries = computed((): SlotEntry[] => {
  return getSlotComponents(props.name) as SlotEntry[];
});
</script>
