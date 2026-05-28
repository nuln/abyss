<template>
  <div>
    <h3>{{ t("settings.permissions") }}</h3>
    <p class="small">{{ t("settings.permissionsHelp") }}</p>

    <p>
      <input type="checkbox" v-model="admin" />
      {{ t("settings.administrator") }}
    </p>

    <p>
      <input type="checkbox" :disabled="admin" v-model="perm.create" /> <!-- eslint-disable-line vue/no-mutating-props -->
      {{ t("settings.perm.create") }}
    </p>
    <p>
      <input type="checkbox" :disabled="admin" v-model="perm.delete" /> <!-- eslint-disable-line vue/no-mutating-props -->
      {{ t("settings.perm.delete") }}
    </p>
    <p>
      <input type="checkbox" :disabled="admin" v-model="perm.download" /> <!-- eslint-disable-line vue/no-mutating-props -->
      {{ t("settings.perm.download") }}
    </p>
    <p>
      <input type="checkbox" :disabled="admin" v-model="perm.modify" /> <!-- eslint-disable-line vue/no-mutating-props -->
      {{ t("settings.perm.modify") }}
    </p>
    <p v-if="isExecEnabled">
      <input type="checkbox" :disabled="admin" v-model="perm.execute" /> <!-- eslint-disable-line vue/no-mutating-props -->
      {{ t("settings.perm.execute") }}
    </p>
    <p>
      <input type="checkbox" :disabled="admin" v-model="perm.rename" /> <!-- eslint-disable-line vue/no-mutating-props -->
      {{ t("settings.perm.rename") }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

const props = defineProps<{
  perm: Permissions;
  isExecEnabled?: boolean;
}>();

const admin = computed({
  get() {
    return props.perm.admin;
  },
  set(value: boolean) {
    if (value) {
      for (const key in props.perm) {
        (props.perm as any)[key] = true;
      }
    }
    props.perm.admin = value; // eslint-disable-line vue/no-mutating-props
  },
});
</script>
