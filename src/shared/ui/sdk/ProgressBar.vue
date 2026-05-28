<template>
  <ProgressBar v-bind="props" />
</template>

<script setup lang="ts">
import ProgressBar from "@/shared/ui/ProgressBar.vue";

const props = defineProps({
  val: { default: 0 },
  max: { default: 100 },
  size: { default: 3 },
  "bg-color": { type: String, default: "#eee" },
  "bar-color": { type: String, default: "#2196f3" },
  "bar-transition": { type: String, default: "all 0.5s ease" },
  "bar-border-radius": { type: Number, default: 0 },
  spacing: { type: Number, default: 4 },
  text: { type: String, default: "" },
  "text-align": { type: String, default: "center" },
  "text-position": { type: String, default: "bottom" },
  "font-size": { type: Number, default: 13 },
  "text-fg-color": { type: String, default: "#222" },
});
</script>
