<template>
  <div class="card floating">
    <div class="card-title">
      <h2>{{ title }}</h2>
    </div>

    <div class="card-content">
      <p v-if="message">{{ message }}</p>
      <input
        id="focus-prompt"
        class="input input--block"
        :type="inputType"
        @keyup.enter="submit"
        v-model.trim="inputValue"
        :placeholder="placeholder"
        ref="inputRef"
      />
    </div>

    <div class="card-action">
      <button
        class="button button--flat button--grey"
        @click="layoutStore.closeHovers()"
        :aria-label="t('buttons.cancel')"
        :title="t('buttons.cancel')"
      >
        {{ t("buttons.cancel") }}
      </button>
      <button
        @click="submit"
        class="button button--flat"
        :aria-label="t('buttons.ok')"
        :title="t('buttons.ok')"
      >
        {{ t("buttons.ok") }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, onMounted, onBeforeUnmount, nextTick } from "vue";
import { useLayoutStore } from "@/app/stores/layout";
import { useI18n } from "vue-i18n";

const { t } = useI18n();
const layoutStore = useLayoutStore();

const props = withDefaults(defineProps<{
  title?: string;
  message?: string;
  placeholder?: string;
  defaultValue?: string;
  type?: string;
  confirm?: ((value: string) => void) | null;
}>(), {
  title: "",
  message: "",
  placeholder: "",
  defaultValue: "",
  type: "text",
  confirm: null,
});

const inputValue = ref("");
const submitted = ref(false);
const inputRef = ref<HTMLInputElement | null>(null);

const inputType = computed(() => props.type || "text");

onMounted(() => {
  inputValue.value = props.defaultValue;
  nextTick(() => {
    inputRef.value?.focus();
  });
});

onBeforeUnmount(() => {
  if (!submitted.value && props.confirm) {
    props.confirm("");
  }
});

const submit = () => {
  submitted.value = true;
  if (props.confirm) {
    props.confirm(inputValue.value);
  }
  layoutStore.closeHovers();
};
</script>
