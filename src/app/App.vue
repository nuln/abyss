<template>
  <div>
    <router-view />
    <Prompts />
    <TaskNotifications />
  </div>
</template>

<script setup lang="ts">
import { useContextMenu } from "@/shared/composables/useContextMenu";
import Prompts from "@/domains/files/components/prompts/Prompts.vue";
import TaskNotifications from "@/domains/tasks/components/TaskNotifications.vue";
import { useAppBoot } from "@/app/boot";

useContextMenu();
useAppBoot();
</script>
