<template>
  <div
    class="context-menu"
    ref="contextMenu"
    v-show="show"
    :style="menuStyle"
  >
    <slot />
    <slot name="file-context-actions" />
    <slot name="file-context-menu" />
  </div>
</template>

<script setup lang="ts">
import { ref, watch, computed, onUnmounted, nextTick } from "vue";

const emit = defineEmits(["hide"]);
const props = defineProps<{ show: boolean; pos: { x: number; y: number } }>();
const contextMenu = ref<HTMLElement | null>(null);
const menuWidth = ref(0);
const menuHeight = ref(0);

const menuStyle = computed(() => {
  const padding = 8;
  const viewportWidth = window.innerWidth;
  const viewportHeight = window.innerHeight;
  
  let x = props.pos.x;
  let y = props.pos.y;
  
  // 如果菜单超出右边界，在左边显示
  if (x + menuWidth.value + padding > viewportWidth) {
    x = Math.max(padding, x - menuWidth.value);
  }
  
  // 如果菜单超出下边界，在上面显示
  if (y + menuHeight.value + padding > viewportHeight) {
    y = Math.max(padding, y - menuHeight.value);
  }
  
  return {
    top: `${y}px`,
    left: `${x}px`,
  };
});

const hideContextMenu = () => {
  emit("hide");
};

const updateMenuSize = async () => {
  await nextTick();
  if (contextMenu.value) {
    menuWidth.value = contextMenu.value.offsetWidth;
    menuHeight.value = contextMenu.value.offsetHeight;
  }
};

watch(
  () => props.show,
  (val) => {
    if (val) {
      updateMenuSize();
      document.addEventListener("click", hideContextMenu);
    } else {
      document.removeEventListener("click", hideContextMenu);
    }
  }
);

onUnmounted(() => {
  document.removeEventListener("click", hideContextMenu);
});
</script>
