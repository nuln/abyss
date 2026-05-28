<template>
  <header>
    <img v-if="showLogo" :src="logoURL" />
    <Action
      v-if="showMenu"
      class="menu-button"
      icon="menu"
      :label="t('buttons.toggleSidebar')"
      @action="layoutStore.showHover('sidebar')"
    />

    <slot />

    <div
      id="dropdown"
      :class="{ active: layoutStore.currentPromptName === 'more' }"
    >
      <slot name="actions" />
    </div>

    <Action
      v-if="ifActionsSlot"
      id="more"
      icon="more_vert"
      :label="t('buttons.more')"
      @action="layoutStore.showHover('more')"
    />

    <div
      class="overlay"
      v-show="layoutStore.currentPromptName == 'more'"
      @click="layoutStore.closeHovers"
    />
    <ActionSlot name="header" />
  </header>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/app/stores/layout";

import { logoURL } from "@/shared/utils/constants";

import Action from "@/shared/ui/header/Action.vue";
import ActionSlot from "@/shared/ui/ActionSlot.vue";
import { computed, useSlots } from "vue";
import { useI18n } from "vue-i18n";

defineProps<{
  showLogo?: boolean;
  showMenu?: boolean;
}>();

const layoutStore = useLayoutStore();
const slots = useSlots();

const { t } = useI18n();

const ifActionsSlot = computed(() => (slots.actions ? true : false));
</script>

<style></style>
