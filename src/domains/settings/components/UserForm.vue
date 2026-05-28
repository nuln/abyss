<template>
  <div>
    <p v-if="!isDefault && props.user !== null">
      <label for="email">{{ t("settings.email") }}</label>
      <input
        class="input input--block"
        type="email"
        v-model="user.email"
        id="email"
        name="email"
        autocomplete="email"
      />
    </p>

    <p v-if="!isDefault">
      <label for="username">{{ t("settings.username") }}</label>
      <input
        class="input input--block"
        type="text"
        v-model="user.username"
        id="username"
        name="username"
        autocomplete="username"
        maxlength="20"
        :placeholder="t('settings.usernamePlaceholder')"
      />
    </p>

    <p v-if="!isDefault">
      <label for="password">{{ t("settings.password") }}</label>
      <input
        class="input input--block"
        type="password"
        :placeholder="passwordPlaceholder"
        v-model="user.password"
        id="password"
        name="password"
        :autocomplete="isNew ? 'new-password' : 'current-password'"
      />
    </p>

    <p v-if="!isNew && !isDefault">
      <label for="scope">{{ t("settings.scope") }}</label>
      <input
        disabled
        :placeholder="scopePlaceholder"
        class="input input--block"
        type="text"
        v-model="user.scope"
        id="scope"
      />
    </p>
    <p class="checkbox small" v-if="displayHomeDirectoryCheckbox">
      <input type="checkbox" v-model="createUserDirData" id="createUserDir" />
      <label for="createUserDir">{{ t("settings.createUserHomeDirectory") }}</label>
    </p>

    <p v-if="!user.perm?.admin">
      <label for="storageQuota">{{ t("settings.storageQuota") }}</label>
      <input
        class="input input--block"
        type="text"
        v-model="formattedStorageQuota"
        id="storageQuota"
        placeholder="10 GB"
      />
      <span class="small">{{ t("settings.storageQuotaHelp") }}</span>
    </p>



    <p>
      <label for="locale">{{ t("settings.language") }}</label>
      <languages
        class="input input--block"
        id="locale"
        v-model:locale="user.locale"
      ></languages>
    </p>
    <p>
      <label for="theme">{{ t("settings.themes.title") }}</label>
      <themes
        class="input input--block"
        id="theme"
        v-model:theme="user.theme"
      ></themes>
    </p>

    <p class="checkbox" v-if="!isDefault && user?.perm">
      <input
        type="checkbox"
        :disabled="user?.perm?.admin"
        v-model="user.lockPassword"
        id="lockPassword"
      />
      <label for="lockPassword">{{ t("settings.lockPassword") }}</label>
    </p>

    <permissions v-if="user.perm" v-model:perm="user.perm" />
  </div>
</template>

<script setup lang="ts">
import Languages from "./Languages.vue";
import Themes from "./Themes.vue";
import Permissions from "./Permissions.vue";
import { computed, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();

const createUserDirData = ref<boolean | null>(null);
const originalUserScope = ref<string | null>(null);

const props = defineProps<{
  user: IUserForm;
  isNew: boolean;
  isDefault: boolean;
  createUserDir?: boolean;
}>();

const user = computed(() => props.user);

onMounted(() => {
  if (props.user.scope) {
    originalUserScope.value = props.user.scope;
    createUserDirData.value = props.createUserDir;
  }
});

const passwordPlaceholder = computed(() =>
  props.isNew ? "" : t("settings.avoidChanges")
);
const scopePlaceholder = computed(() =>
  createUserDirData.value ? t("settings.userScopeGenerationPlaceholder") : ""
);
const displayHomeDirectoryCheckbox = computed(
  () => props.isNew && createUserDirData.value
);

const formattedStorageQuota = computed({
  get() {
    const val = user.value.storageQuota;
    if (val === undefined || val === null || val === "") return "";
    
    // If it's already a string with a unit (like "10 GB"), return it as is
    if (typeof val === "string" && /[a-zA-Z]/.test(val)) {
        return val;
    }
    
    const num = Number(val);
    return !isNaN(num) ? formatBytes(num) : "";
  },
  set(value: string) {
    if (debounceTimeout.value) {
      clearTimeout(debounceTimeout.value);
    }
    debounceTimeout.value = window.setTimeout(() => {
        const bytes = parseBytes(value);
        // Backend expects different types:
        // - Global Defaults (settings.json): int64 (JSON Number)
        // - Create User (request struct): string (JSON String)
        if (props.isDefault) {
            user.value.storageQuota = bytes;
        } else {
            user.value.storageQuota = bytes.toString();
        }
    }, 500);
  },
});

interface SettingsUnit {
  KB: number;
  MB: number;
  GB: number;
  TB: number;
  PB: number;
}

// Parse the user-friendly input (e.g., "20M" or "1T") to bytes
const parseBytes = (input: string) => {
  const regex = /^(\d+(\.\d+)?)(B|K|KB|M|MB|G|GB|T|TB|P|PB)?$/i;
  const matches = input.match(regex);
  if (matches) {
    const size = parseFloat(matches[1]);
    let unit = matches[3] ? matches[3].toUpperCase() : "";
    
    // Normalize unit (e.g., "G" -> "GB")
    if (unit && !unit.endsWith("B")) {
        unit += "B";
    } else if (!unit) {
        // Default to bytes if no unit provided? Or treat raw number as bytes
        return size; 
    }

    const units: SettingsUnit = {
      KB: 1024,
      MB: 1024 ** 2,
      GB: 1024 ** 3,
      TB: 1024 ** 4,
      PB: 1024 ** 5,
    };
    return Math.floor(size * (units[unit as keyof SettingsUnit] || 1));
  }
  return 0;
};

// Format the chunk size in bytes to user-friendly format
const formatBytes = (bytes: number) => {
  if (isNaN(bytes) || bytes === null) return "";
  if (bytes === 0) return "";
  const units = ["B", "KB", "MB", "GB", "TB", "PB"];
  let size = bytes;
  let unitIndex = 0;
  while (size >= 1024 && unitIndex < units.length - 1) {
    size /= 1024;
    unitIndex++;
  }
  return `${parseFloat(size.toFixed(2))} ${units[unitIndex]}`;
};

const debounceTimeout = ref<number | null>(null);

watch(
  () => props.user,
  () => {
    if (!props.user?.perm?.admin) return;
    props.user.lockPassword = false; // eslint-disable-line vue/no-mutating-props
    // Auto-set storage quota to 0 (unlimited) for admin users
    props.user.storageQuota = "0"; // eslint-disable-line vue/no-mutating-props
  }
);

watch(
  () => props.user.perm?.admin,
  (isAdmin) => {
    if (isAdmin && props.user) {
      props.user.storageQuota = "0"; // eslint-disable-line vue/no-mutating-props
    }
  }
);

</script>
