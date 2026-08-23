<template>
  <template v-for="(action, index) in actions" :key="index">
    <!-- If it's a string, treat as icon or label -->
    <Action
      v-if="typeof action === 'object' && action.icon"
      v-bind="action"
      @action="handleAction(action)"
    />
    <!-- If it's a component -->
    <component v-else :is="action" />
  </template>
</template>

<script setup lang="ts">
import { computed } from "vue";
import Action from "@/shared/ui/header/Action.vue";
import { getActions } from "@/plugin/slots";

const props = defineProps<{
  name: string;
}>();

const actions = computed(() => {
  return getActions(props.name);
});

const handleAction = (action: any) => {
  if (typeof action.handler === 'function') {
    action.handler();
  }
};
</script>

<style scoped>
/* Removed styles to let components flow naturally in parent containers */
</style>
