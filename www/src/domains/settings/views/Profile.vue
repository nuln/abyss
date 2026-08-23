<template>
  <div class="row">
    <div class="column">
      <form class="card" @submit.prevent="updateSettings">
        <div class="card-title">
          <h2>{{ t("settings.profileSettings") }}</h2>
            <button class="button button--flat" type="submit">
            {{ t("buttons.update") }}
          </button>
        </div>

        <div class="card-content">
          <p class="checkbox">
            <input type="checkbox" id="singleClick" name="singleClick" v-model="singleClick" />
            <label for="singleClick">{{ t("settings.singleClick") }}</label>
          </p>
          <p class="checkbox">
            <input type="checkbox" id="dateFormat" name="dateFormat" v-model="dateFormat" />
            <label for="dateFormat">{{ t("settings.setDateFormat") }}</label>
          </p>
          <p class="checkbox">
            <input type="checkbox" id="showHidden" name="showHidden" v-model="showHidden" />
            <label for="showHidden">{{ t("settings.showHidden") }}</label>
          </p>

          <h3>{{ t("settings.language") }}</h3>
          <languages
            class="input input--block"
            id="locale"
            name="locale"
            v-model:locale="locale"
          ></languages>

          <h3>{{ t("settings.themes.title") }}</h3>
          <ThemeSelector
            class="input input--block"
            v-model:theme="theme"
            id="themeSelector"
          ></ThemeSelector>
        </div>

      </form>
      
      <!-- Plugin based settings in left column to match profile width -->
      <PluginSlot name="user-profile-settings" />
    </div>

    <div class="column security-column">
      <form
        class="card card--compact"
        v-if="!authStore.user?.lockPassword"
        @submit="updatePassword"
      >
        <div class="card-title">
          <h2>{{ t("settings.changePassword") }}</h2>
          <button class="button button--flat" type="submit">
            {{ t("buttons.update") }}
          </button>
        </div>

        <div class="card-content">
          <input
            :class="passwordClass"
            type="password"
            :placeholder="t('settings.currentPassword')"
            v-model="currentPassword"
            id="currentPassword"
            name="currentPassword"
            autocomplete="current-password"
          />
          <input
            :class="passwordClass"
            type="password"
            :placeholder="t('settings.newPassword')"
            v-model="password"
            id="password"
            name="password"
            autocomplete="new-password"
          />
          <input
            :class="passwordClass"
            type="password"
            :placeholder="t('settings.newPasswordConfirm')"
            v-model="passwordConf"
            id="passwordConf"
            name="passwordConf"
            autocomplete="new-password"
          />
        </div>

      </form>

      <!-- Plugin based security settings -->
      <PluginSlot name="user-security-settings" />
    </div>
  </div>
</template>

<script setup lang="ts">
import { useAuthStore } from "@/domains/auth";
import { useLayoutStore } from "@/app/stores/layout";
import { users as api } from "@/domains/settings/api";
import ThemeSelector from "@/domains/settings/components/ThemeSelector.vue";
import Languages from "@/domains/settings/components/Languages.vue";
import PluginSlot from "@/plugin/PluginSlot.vue";
import { computed, inject, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { setTheme } from "@/shared/theme";

const layoutStore = useLayoutStore();
const authStore = useAuthStore();
const { t } = useI18n();

const $showSuccess = inject<IToastSuccess>("$showSuccess")!;
const $showError = inject<IToastError>("$showError")!;

const password = ref<string>("");
const currentPassword = ref<string>("");
const passwordConf = ref<string>("");

const singleClick = ref<boolean>(false);
const dateFormat = ref<boolean>(false);
const showHidden = ref<boolean>(false);
const locale = ref<string>("");
const theme = ref<UserTheme>("auto");

const passwordClass = computed(() => {
  const baseClass = "input input--block";

  if (password.value === "" && passwordConf.value === "") {
    return baseClass;
  }

  if (password.value === passwordConf.value) {
    return `${baseClass} input--green`;
  }

  return `${baseClass} input--red`;
});


onMounted(async () => {
  layoutStore.loading = true;
  if (authStore.user === null) {
    layoutStore.loading = false;
    return false;
  }
  locale.value = authStore.user.locale || "auto";

  singleClick.value = authStore.user.singleClick;
  dateFormat.value = authStore.user.dateFormat;
  showHidden.value = authStore.user.showHidden;
  theme.value = authStore.user.theme || "auto";
  layoutStore.loading = false;
  return true;
});

const updatePassword = async (event: Event) => {
  event.preventDefault();

  if (
    password.value !== passwordConf.value ||
    password.value === "" ||
    authStore.user === null
  ) {
    return;
  }

  try {
    const data = {
      ...authStore.user,
      id: authStore.user.id,
      password: password.value,
    };
    await api.update(data, ["password"], currentPassword.value, false);
    // Never merge the plaintext password into the global user state.
    const { password: _pw, ...safeUser } = data;
    authStore.updateUser(safeUser as typeof authStore.user);
    $showSuccess(t("settings.passwordUpdated"));
  } catch (e: any) {
    $showError(e);
  } finally {
    password.value = passwordConf.value = currentPassword.value = "";
  }
};
const updateSettings = async (event: Event) => {
  event.preventDefault();

  try {
    if (authStore.user === null) throw new Error("User is not set!");

    const data = {
      ...authStore.user,
      id: authStore.user.id,
      locale: locale.value,

      singleClick: singleClick.value,
      dateFormat: dateFormat.value,
      showHidden: showHidden.value,
      theme: theme.value,
    };

    await api.update(data, [
      "locale",
      "singleClick",
      "dateFormat",
      "showHidden",
      "theme",
    ]);
    authStore.updateUser(data);
    // Apply theme immediately
    setTheme(theme.value);
    $showSuccess(t("settings.settingsUpdated"));
  } catch (err) {
    if (err instanceof Error) {
      $showError(err);
    }
  }
};
</script>

<style scoped>
.retention-row {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin: 1rem 0;
}

.retention-label {
  font-weight: 500;
  white-space: nowrap;
}

.retention-input {
  width: 80px;
  text-align: right;
  padding-right: 0.5rem;
}

.retention-suffix {
  color: var(--text-secondary);
  white-space: nowrap;
}
</style>
