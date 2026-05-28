<template>
  <ListingItem v-bind="props" />
</template>

<script setup lang="ts">
import ListingItem from "@/domains/files/components/files/ListingItem.vue";

const props = defineProps<{
  name: string;
  isDir: boolean;
  url: string;
  type: string;
  size: number;
  modified: string;
  index: number;
  readOnly?: boolean;
  path?: string;
  shared?: boolean;
  ownerId?: number;
  thumbnailUrl?: string;
}>();
</script>
