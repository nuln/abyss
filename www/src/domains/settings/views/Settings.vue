<template>
  <div class="dashboard">
    <div id="nav">
      <div class="wrapper">
        <ul>
          <router-link to="/settings/profile"
            ><li :class="{ active: route.path === '/settings/profile' }">
              {{ t("settings.profileSettings") }}
            </li></router-link
          >
          <template v-if="user?.perm?.admin">
            <router-link
              v-for="tab in settingsTabs"
              :key="tab.slugName"
              :to="tab.route"
              ><li :class="{ active: route.path === tab.route }">
                {{ t(tab.name) }}
              </li></router-link
            >
          </template>
          <router-link to="/settings/global" v-if="user?.perm?.admin"
            ><li :class="{ active: route.path === '/settings/global' }">
              {{ t("settings.globalSettings") }}
            </li></router-link
          >
          <router-link to="/settings/users" v-if="user?.perm?.admin"
            ><li
              :class="{
                active:
                  route.path === '/settings/users' || route.name === 'User',
              }"
            >
              {{ t("settings.userManagement") }}
            </li></router-link
          >
          <router-link to="/settings/plugins" v-if="user?.perm?.admin"
            ><li :class="{ active: route.path === '/settings/plugins' }">
              {{ t("settings.plugins") }}
            </li></router-link
          >
        </ul>
      </div>
    </div>

    <div v-if="loading">
      <h2 class="message delayed">
        <div class="spinner">
          <div class="bounce1"></div>
          <div class="bounce2"></div>
          <div class="bounce3"></div>
        </div>
        <span>{{ t("files.loading") }}</span>
      </h2>
    </div>

    <router-view></router-view>
  </div>
</template>

<script setup lang="ts">
import { useAuthStore } from "@/domains/auth";
import { useLayoutStore } from "@/app/stores/layout";
import { usePluginStore } from "@/domains/settings/store";
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute } from "vue-router";

const { t } = useI18n();
const route = useRoute();

const authStore = useAuthStore();
const layoutStore = useLayoutStore();
const pluginStore = usePluginStore();

const user = computed(() => authStore.user);
const loading = computed(() => layoutStore.loading);
const settingsTabs = computed(() => pluginStore.settingsTabs);
</script>
