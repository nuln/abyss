<template>
  <div :class="{ 'centered-error-container': error }">
    <errors v-if="error" :errorCode="error.status" inline />
    <div class="row row--full" v-else-if="!layoutStore.loading">
      <div class="column column--full">
        <div class="card">
          <div class="card-title">
            <h2>{{ t("settings.plugins") }}</h2>
          </div>

          <div class="card-content full">
            <table>
              <thead>
                <tr>
                  <th>{{ t("settings.name") }}</th>
                  <th>{{ t("settings.description") }}</th>
                  <th>{{ t("settings.version") }}</th>
                  <th>{{ t("settings.status") }}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="plugin in plugins" :key="plugin.slugName">
                  <td>
                    {{ t(plugin.name) }}
                    <span
                      v-if="plugin.type === 'paid'"
                      class="badge badge--pro"
                      >{{ t("settings.pro") }}</span
                    >
                  </td>
                  <td>{{ t(plugin.description) }}</td>
                  <td>{{ plugin.version }}</td>
                  <td>
                    <label class="switch">
                      <input
                        type="checkbox"
                        :checked="plugin.enabled"
                        :disabled="false"
                        @change="handleToggle(plugin)"
                      />
                      <span class="slider round"></span>
                    </label>
                  </td>
                  <td class="small">
                    <button
                      v-if="plugin.hasConfig"
                      class="button button--flat"
                      @click="configure(plugin)"
                      :disabled="!plugin.enabled"
                    >
                      <i class="material-icons">settings</i>
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    <!-- Configuration Modal -->
    <div
      v-if="editingPlugin"
      class="modal-wrapper"
      @click.self="editingPlugin = null"
    >
        <form @submit.prevent="saveConfig" class="modal card">
          <div class="card-title">
            <h2>{{ t(editingPlugin.name) }} {{ t("settings.configuration") }}</h2>
          </div>
          <div class="card-content">
          <div v-for="(group, groupName) in groupedFields" :key="groupName" class="config-group">
            <h3 v-if="groupName !== 'default'" class="group-title">{{ t(groupName) }}</h3>
            
            <div v-for="(row, rowIdx) in group" :key="rowIdx" class="config-row">
              <div
                v-for="field in row"
                :key="field.name"
                class="config-field"
                :class="{ 'config-field--flex': row.length > 1 }"
              >
                <div class="field-header">
                  <label :for="field.name">{{ t(field.title) }}</label>
                </div>
                <p class="small" v-if="field.description">
                  {{ t(field.description) }}
                </p>

                <div class="field-content">
                  <div v-if="field.type === 'button'" class="button-field">
                    <button
                      class="button button--secondary"
                      @click="handleAction(field)"
                      :disabled="layoutStore.loading"
                    >
                      <i v-if="field.icon" class="material-icons left">{{ field.icon }}</i>
                      {{ t(field.title) }}
                    </button>
                  </div>

                  <div v-else-if="field.icon" class="input-with-icon">
                    <i class="material-icons" :class="field.iconClass">{{ field.icon }}</i>
                    <span class="value-text">{{ (field.value && typeof field.value === 'string') ? t(field.value) : '' }}</span>
                  </div>
                  <input
                    v-else-if="
                      field.type === 'input' ||
                      field.type === 'password' ||
                      field.type === 'number'
                    "
                    :type="
                      field.type === 'password'
                        ? 'password'
                        : field.type === 'number'
                          ? 'number'
                          : 'text'
                    "
                    class="input input--block"
                    v-model="configValues[field.name]"
                    :id="field.name"
                    :name="field.name"
                    :autocomplete="field.type === 'password' ? 'new-password' : 'off'"
                    :required="field.required"
                    :readonly="field.readOnly"
                    :disabled="field.readOnly"
                  />
                  <textarea
                    v-else-if="field.type === 'textarea'"
                    class="input input--block"
                    v-model="configValues[field.name]"
                    :id="field.name"
                    :name="field.name"
                    :readonly="field.readOnly"
                    :disabled="field.readOnly"
                  ></textarea>

                  <label v-else-if="field.type === 'switch'" class="switch">
                    <input type="checkbox" v-model="configValues[field.name]" :id="field.name" :name="field.name" :disabled="field.readOnly" />
                    <span class="slider round"></span>
                  </label>

                  <select
                    v-else-if="field.type === 'select'"
                    class="input input--block"
                    v-model="configValues[field.name]"
                    :id="field.name"
                    :name="field.name"
                    :disabled="field.readOnly"
                  >
                    <option
                      v-for="opt in field.options"
                      :key="opt.value"
                      :value="opt.value"
                    >
                      {{ opt.label }}
                    </option>
                  </select>
                </div>
              </div>
            </div>
          </div>
        </div>
          <div class="card-action">
            <button class="button button--flat" type="button" @click="editingPlugin = null">
              {{ t("buttons.cancel") }}
            </button>
            <button class="button" type="submit">
              {{ t("buttons.save") }}
            </button>
          </div>
        </form>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useLayoutStore } from "@/app/stores/layout";
import { fetchJSON } from "@/domains/settings/api";
import {
  usePluginStore,
  type IPluginInfo,
  type IConfigField,
} from "@/domains/settings/store";
import { loadPlugin } from "@/plugin/loader";
import Errors from "@/app/Errors.vue";
import { computed, onMounted, ref, inject } from "vue";
import { useI18n } from "vue-i18n";
import type { StatusError } from "@/domains/settings/api";

const error = ref<StatusError | null>(null);
const editingPlugin = ref<IPluginInfo | null>(null);
const configFields = ref<IConfigField[]>([]);
const configValues = ref<Record<string, any>>({});

const layoutStore = useLayoutStore();
const pluginStore = usePluginStore();
const { t } = useI18n();

const $showError = inject<IToastError>("$showError")!;
const $showSuccess = inject<IToastSuccess>("$showSuccess")!;

const plugins = computed(() => pluginStore.plugins);

const groupedFields = computed(() => {
  const groups: Record<string, Record<number, IConfigField[]>> = {};

  configFields.value.forEach((field, index) => {
    const groupName = field.group || "default";
    // If field.row is 0 or undefined, use a unique row index based on the field's position
    // to ensure vertical stacking by default.
    const rowNum = field.row || (index + 1) * 1000;

    if (!groups[groupName]) {
      groups[groupName] = {};
    }
    if (!groups[groupName][rowNum]) {
      groups[groupName][rowNum] = [];
    }
    groups[groupName][rowNum].push(field);
  });

  return groups;
});

onMounted(async () => {
  layoutStore.loading = true;
  try {
    await pluginStore.fetchPlugins();
    // Load each plugin's JS bundle so names/descriptions always come from the plugin frontend.
    // Plugin static assets are always served regardless of enabled status.
    await Promise.allSettled(
      plugins.value
        .filter((p) => p.enabled && p.hasUI)
        .map((p) => loadPlugin(p.slugName)),
    );
  } catch (err) {
    if (err instanceof Error) error.value = err as StatusError;
  } finally {
    layoutStore.loading = false;
  }
});

const handleToggle = async (plugin: IPluginInfo) => {
  const newEnabled = !plugin.enabled;
  try {
    await pluginStore.togglePlugin(plugin.slugName, newEnabled);
    const message = t(newEnabled ? "settings.pluginEnabled" : "settings.pluginDisabled", {
      name: t(plugin.name),
    });
    $showSuccess(message);
  } catch (e: any) {
    $showError(e);
  }
};

const configure = async (plugin: IPluginInfo) => {
  try {
    if (plugin.hasUI) {
      try {
        await loadPlugin(plugin.slugName);
      } catch (_e) {
        // console.warn(`[Plugins] Failed to load plugin assets for ${plugin.slugName}`, e);
      }
    }

    const fields = await pluginStore.fetchPluginConfig(plugin.slugName);
    if (!fields || !Array.isArray(fields) || fields.length === 0) {
      $showError(t("settings.noConfigFields"));
      return;
    }
    configFields.value = fields;

    // Initialize values
    const values: Record<string, any> = {};
    fields.forEach((f) => {
      values[f.name] = f.value;
    });
    configValues.value = values;
    editingPlugin.value = plugin;
  } catch (e: any) {
    $showError(e);
  }
};


const handleAction = async (field: IConfigField) => {
  if (!field.action || !editingPlugin.value) return;

  layoutStore.loading = true;
  try {
    await fetchJSON<any>(field.action, {
      method: "POST",
    });

    // Refresh fields
    const fields = await pluginStore.fetchPluginConfig(editingPlugin.value.slugName);
    configFields.value = fields;
    fields.forEach((f) => {
      configValues.value[f.name] = f.value;
    });
    // Also refresh plugin list status
    await pluginStore.fetchPlugins();
  } catch (e: any) {
    $showError(e);
  } finally {
    layoutStore.loading = false;
  }
};

const performSaveConfig = async () => {
  if (!editingPlugin.value) return;
  try {
    // Filter out empty values for sensitive fields to prevent accidental clearing
    const payload: Record<string, any> = {};
    for (const key in configValues.value) {
      payload[key] = configValues.value[key];
    }

    await pluginStore.updatePluginConfig(
      editingPlugin.value.slugName,
      payload,
    );
    $showSuccess(t("settings.configUpdated"));
    editingPlugin.value = null;
  } catch (e: any) {
    $showError(e);
  }
};

const saveConfig = async () => {
  if (!editingPlugin.value) return;
  await performSaveConfig();
};
</script>

<style scoped>
.modal-wrapper {
  position: fixed;
  top: 0;
  left: 0;
  width: 100%;
  height: 100%;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal {
  width: 100%;
  max-width: 500px;
  max-height: 90vh;
  overflow-y: auto;
}

.config-field {
  margin-bottom: 0.5rem;
}

.config-field label {
  display: block;
  font-weight: bold;
  margin-bottom: 0.25rem;
}

.switch {
  position: relative;
  display: inline-block;
  width: 40px;
  height: 20px;
}

.switch input {
  opacity: 0;
  width: 0;
  height: 0;
}

.slider {
  position: absolute;
  cursor: pointer;
  top: 0;
  left: 0;
  right: 0;
  bottom: 0;
  background-color: var(--surfaceSecondary);
  transition: 0.4s;
}

input:disabled + .slider {
  cursor: not-allowed;
  opacity: 0.5;
}

.slider:before {
  position: absolute;
  content: "";
  height: 14px;
  width: 14px;
  left: 3px;
  bottom: 3px;
  background-color: white;
  transition: 0.4s;
}

input:checked + .slider {
  background-color: var(--blue);
}

input:checked + .slider:before {
  transform: translateX(20px);
}

.slider.round {
  border-radius: 20px;
}

.slider.round:before {
  border-radius: 50%;
}

.badge {
  display: inline-block;
  padding: 0.1rem 0.4rem;
  border-radius: 4px;
  font-size: 0.75rem;
  font-weight: bold;
  margin-left: 0.5rem;
}

.badge--pro {
  background-color: var(--surfaceSecondary);
  color: var(--text-secondary);
  border: 1px solid var(--divider);
  text-transform: uppercase;
  font-size: 10px;
  padding: 0.1rem 0.3rem;
}

.config-group {
  margin-bottom: 2rem;
  padding: 0 0 1rem 0;
  border-bottom: 1px solid var(--divider);
}

.config-group:last-child {
  border-bottom: none;
}

.group-title {
  margin-top: 0;
  margin-bottom: 1.25rem;
  font-size: 0.85rem;
  font-weight: 700;
  color: var(--text-secondary);
  text-transform: uppercase;
  letter-spacing: 0.05em;
  padding-left: 0;
}

.config-row {
  display: flex;
  gap: 1.5rem;
  margin-bottom: 0.5rem;
  flex-wrap: nowrap;
}

.config-field--flex {
  flex: 1;
  min-width: 0;
}

.field-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 0.5rem;
}

.field-header label {
  margin-bottom: 0 !important;
}

.input-with-icon {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.5rem 0;
  font-weight: 500;
}

.input-with-icon i {
  font-size: 20px;
}

.value-text {
  font-size: 0.95rem;
}

.text-success {
  color: #4caf50;
}

.text-error {
  color: #f44336;
}

.text-warning {
  color: #ff9800;
}

.small-icon i {
  font-size: 18px !important;
}

.left {
  margin-right: 0.5rem;
}

:root.dark .badge--pro {
  background-color: var(--color-gray-700);
  color: var(--color-gray-400);
  border: 1px solid var(--color-gray-600);
}
</style>
