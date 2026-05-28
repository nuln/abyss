<template>
  <div :class="{ 'centered-error-container': error }">
    <errors v-if="error" :errorCode="error.status" inline />
    <div class="row" v-else-if="!layoutStore.loading && settings !== null">
      <div class="column">
        <form class="card" @submit.prevent="save">
          <div class="card-title">
            <h2>{{ t("settings.globalSettings") }}</h2>
            <input
              class="button button--flat"
              type="submit"
              :value="t('buttons.update')"
            />
          </div>

          <div class="card-content">
            <p class="checkbox">
              <input type="checkbox" v-model="settings.signup" id="signup" name="signup" />
              <label for="signup">{{ t("settings.allowSignup") }}</label>
            </p>

            <p>
              <label for="minimumPasswordLength">{{
                t("settings.minimumPasswordLength")
              }}</label>
              <vue-number-input
                controls
                v-model.number="settings.minimumPasswordLength"
                id="minimumPasswordLength"
                name="minimumPasswordLength"
                :min="1"
              />
            </p>

            <h3>{{ t("settings.tusUploads") }}</h3>
            <p class="small">{{ t("settings.tusUploadsHelp") }}</p>
            <!-- Note: Detailed TUS settings are now handled via the TUS plugin dynamic component -->
          </div>
        </form>

          <form @submit.prevent class="card">
            <div class="card-title">
              <h2>{{ t("settings.storage") }}</h2>
              <div
                v-if="storageTypeChanged && !migrating"
                class="card-title-actions"
              >
                <button
                  class="button button--flat"
                  type="button"
                  @click="cancelStorageTypeChange"
                  :disabled="migrating"
                >
                  {{ t("buttons.cancel") }}
                </button>
                <button
                  class="button button--flat button--blue"
                  type="button"
                  @click="saveStorageType"
                  :disabled="migrating"
                  style="margin-left: 0.5rem"
                >
                  {{ t("buttons.migrateAndApply") }}
                </button>
              </div>
            </div>
          <div class="card-content">
            <p class="small">{{ t("settings.storageDescription") }}</p>
            <p>
              <label for="storage-type">{{ t("settings.storageType") }}</label>
                <select
                  class="input input--block"
                  v-model="settings.storageType"
                  id="storage-type"
                  name="storageType"
                  @change="onStorageTypeChange"
                  :disabled="migrating"
                >
                  <option
                    v-for="st in settings.availableStorageTypes"
                    :key="st.name"
                    :value="st.name"
                  >
                    {{ t(st.displayName) }}
                  </option>
                </select>
              <span class="input-help" v-if="settings.availableStorageTypes">{{
                settings.availableStorageTypes.find(
                  (s: StorageTypeInfo) => s.name === settings.storageType,
                )?.description
                  ? t(
                      settings.availableStorageTypes.find(
                        (s: StorageTypeInfo) => s.name === settings.storageType,
                      )?.description,
                    )
                  : ""
              }}</span>
            </p>
            <div
              v-if="storageTypeChanged && !migrating"
              class="alert alert--warning"
            >
              <p>{{ t("settings.storageTypeChangeWarning") }}</p>
            </div>

            <!-- Migration progress -->
            <div v-if="migrating" class="migration-progress">
              <div class="progress-header">
                <span class="status-badge status-active">{{
                  t("settings.migrating")
                }}</span>
                <span class="progress-text"
                  >{{ migrationStatus?.migrated ?? 0 }} /
                  {{ migrationStatus?.total ?? "?" }}</span
                >
              </div>
              <div class="progress-bar">
                <div
                  class="progress-bar-fill"
                  :style="{ width: progressPercent + '%' }"
                ></div>
              </div>
              <p v-if="migrationStatus?.failed" class="text-error small">
                {{
                  t("settings.migrationFailed", {
                    count: migrationStatus.failed,
                  })
                }}
              </p>
            </div>

            <!-- Migration result -->
            <div v-if="migrationResult" class="migration-result">
              <p v-if="migrationResult.status === 'done'" class="success">
                ✓ {{ t("settings.migrationSuccess") }} ({{
                  migrationResult.migrated
                }}/{{ migrationResult.total }})
              </p>
              <p
                v-else-if="migrationResult.status === 'done_with_errors'"
                class="text-warning"
              >
                ⚠ {{ t("settings.migrationSuccess") }} ({{
                  migrationResult.migrated
                }}/{{ migrationResult.total }}, {{ migrationResult.failed }}
                failed)
              </p>
              <p
                v-else-if="migrationResult.status === 'failed'"
                class="text-error"
              >
                ✗ {{ migrationResult.error }}
              </p>
            </div>

            <!-- Plugin Storage settings (conditional) -->
            <div class="plugin-storage-settings">
              <template
                v-for="pSetting in abyss.pluginGlobalSettings.filter(
                  (s) => s.category === 'storage',
                )"
                :key="pSetting.slug + pSetting.label"
              >
                <component
                  :is="abyss.components[pSetting.component]"
                  :settings="settings"
                />
              </template>
              </div>
            </div>
          </form>
        <!-- General Plugin Global Settings -->
        <template
          v-for="pSetting in abyss.pluginGlobalSettings.filter(
            (s) => s.category !== 'storage',
          )"
          :key="pSetting.slug + pSetting.label"
        >
          <component
            :is="abyss.components[pSetting.component]"
            :settings="settings"
          />
        </template>
      </div>

      <div class="column">
        <form class="card" @submit.prevent="save">
          <div class="card-title">
            <h2>{{ t("settings.userDefaults") }}</h2>
            <input
              class="button button--flat"
              type="submit"
              :value="t('buttons.update')"
            />
          </div>

          <div class="card-content">
            <p class="small">{{ t("settings.defaultUserDescription") }}</p>
            <user-form
              :isNew="false"
              :isDefault="true"
              v-model:user="settings.defaults"
            />
          </div>
        </form>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { settings as api, StatusError } from "@/domains/settings/api";
import UserForm from "@/domains/settings/components/UserForm.vue";
import { useLayoutStore } from "@/app/stores/layout";
import Errors from "@/app/Errors.vue";
import { computed, inject, onBeforeUnmount, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n();
const $showError = inject<any>("$showError")!;
const $showSuccess = inject<any>("$showSuccess")!;
const layoutStore = useLayoutStore();
const abyss = window.__ABYSS__;

const error = ref<StatusError | null>(null);
const originalSettings = ref<ISettings | null>(null);
const settings = ref<any>(null);

const storageTypeChanged = ref(false);
const migrating = ref(false);
const migrationStatus = ref<MigrationStatus | null>(null);
const migrationResult = ref<MigrationStatus | null>(null);
let migrationPollTimer: number | null = null;

const progressPercent = computed(() => {
  if (!migrationStatus.value || !migrationStatus.value.total) return 0;
  return Math.round(
    (migrationStatus.value.migrated / migrationStatus.value.total) * 100,
  );
});

const onStorageTypeChange = () => {
  if (settings.value.storageType !== originalSettings.value?.storageType) {
    storageTypeChanged.value = true;
  } else {
    storageTypeChanged.value = false;
  }
};

const cancelStorageTypeChange = () => {
  if (originalSettings.value) {
    settings.value.storageType = originalSettings.value.storageType;
    storageTypeChanged.value = false;
  }
};

const pollMigrationStatus = async () => {
  try {
    const status = await api.getMigrationStatus();
    migrationStatus.value = status;

    if (!status.running) {
      // Migration finished
      stopPolling();
      migrating.value = false;
      migrationResult.value = status;
      storageTypeChanged.value = false;

      if (status.status === "done") {
        // Settings were updated by the server, refresh them
        if (originalSettings.value) {
          originalSettings.value.storageType = status.targetType;
          settings.value.storageType = status.targetType;
        }
        $showSuccess(
          t("settings.migrationSuccess") +
            ` (${status.migrated}/${status.total})`,
        );
      } else if (status.status === "done_with_errors") {
        $showSuccess(
          t("settings.migrationSuccess") +
            ` (${status.migrated}/${status.total}, ${status.failed} failed)`,
        );
      } else if (status.status === "failed") {
        $showError(status.error || t("settings.migrationFailedGeneric"));
        // Revert the selection since migration failed
        if (originalSettings.value) {
          settings.value.storageType = originalSettings.value.storageType;
        }
      }
    }
  } catch (_e) {
    // console.error("Failed to poll migration status:", e);
  }
};

const stopPolling = () => {
  if (migrationPollTimer) {
    clearInterval(migrationPollTimer);
    migrationPollTimer = null;
  }
};

const saveStorageType = async () => {
  layoutStore.showHover({
    prompt: "simpleConfirm",
    props: {
      title: t("prompts.confirmation"),
      message: t("settings.storageTypeChangeConfirmMigration"),
    },
    confirm: async (confirmed: boolean) => {
      if (!confirmed) return;

      migrating.value = true;
      migrationResult.value = null;
      try {
        const res = await api.migrateStorage(settings.value.storageType);
        migrationStatus.value = res;

        // Start polling for status updates
        migrationPollTimer = window.setInterval(pollMigrationStatus, 2000);
      } catch (e: any) {
        migrating.value = false;
        $showError(e);
      }
    },
  });
};

const save = async () => {
  if (settings.value === null) return false;

  try {
    await api.update(settings.value);
    originalSettings.value = JSON.parse(JSON.stringify(settings.value));
    $showSuccess(t("settings.settingsUpdated"));
  } catch (e: any) {
    $showError(e);
  }

  return true;
};

onMounted(async () => {
  try {
    layoutStore.loading = true;
    const original: any = await api.get();

    originalSettings.value = original;
    if (!original.tus) {
      original.tus = { chunkSize: 1024 * 1024 * 5, retryCount: 3 };
    }
    settings.value = JSON.parse(JSON.stringify(original));

    // Check if a migration is currently running
    try {
      const status = await api.getMigrationStatus();
      if (status.running) {
        migrating.value = true;
        migrationStatus.value = status;
        migrationPollTimer = window.setInterval(pollMigrationStatus, 2000);
      }
    } catch (_e) {
      // Ignore - endpoint may not exist in older versions
    }
  } catch (err) {
    if (err instanceof Error) {
      error.value = err as StatusError;
    }
  } finally {
    layoutStore.loading = false;
  }
});

onBeforeUnmount(() => {
  stopPolling();
});
</script>

<style scoped>
.input-help {
  display: block;
  font-size: 0.8rem;
  color: var(--textSecondary);
  margin-top: 0.25rem;
}

.migration-progress {
  margin: 1rem 0;
  padding: 1rem;
  border-radius: 0.5rem;
  background: var(--surfaceSecondary);
}

.progress-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.progress-text {
  font-size: 0.9rem;
  color: var(--textSecondary);
}

.progress-bar {
  width: 100%;
  height: 8px;
  background: var(--divider);
  border-radius: 4px;
  overflow: hidden;
}

.progress-bar-fill {
  height: 100%;
  background: var(--blue);
  border-radius: 4px;
  transition: width 0.3s ease;
}

.migration-result {
  margin: 1rem 0;
  padding: 0.75rem;
  border-radius: 0.25rem;
  background: var(--surfaceSecondary);
}

.migration-result p {
  margin: 0;
}
.migration-result .success {
  color: var(--green);
}
.migration-result .text-warning {
  color: var(--yellow);
}
.migration-result .text-error {
  color: var(--red);
}

.status-badge {
  font-size: 0.8rem;
  padding: 0.2rem 0.5rem;
  border-radius: 1rem;
  background: var(--surfaceSecondary);
  color: var(--textSecondary);
}

.status-active {
  background: var(--blue);
  color: white;
}

.text-error {
  color: var(--red);
}
</style>
