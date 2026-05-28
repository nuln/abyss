  <template>
  <template v-if="pluginStore.loaded">
    <template v-if="activeSetting">
      <component :is="abyss.components[activeSetting.component]" />
    </template>
    <div v-else class="not-found-wrapper">
      <h2 class="message">
        <i class="material-icons">extension_off</i>
        <span>{{ t('errors.notFound') }}</span>
      </h2>
    </div>
  </template>
  <div v-else class="loading-wrapper">
    <div class="spinner">
      <div class="bounce1"></div>
      <div class="bounce2"></div>
      <div class="bounce3"></div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { usePluginStore } from "@/domains/settings/store";
import { computed } from "vue";
import { useRoute } from "vue-router";
import { useI18n } from "vue-i18n";

const { t } = useI18n();
const route = useRoute();
const pluginStore = usePluginStore();
const abyss = window.__ABYSS__;

// Extracted from route path, e.g. /settings/trash -> pluginSlug = trash
const activeSetting = computed(() => {
  const pluginSlug = route.params.plugin as string;
  return abyss.pluginSettings.find(s => s.slug === pluginSlug);
});
</script>

<style scoped>
.not-found-wrapper,
.loading-wrapper {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100%;
}

.spinner {
  margin: 100px auto 0;
  width: 70px;
  text-align: center;
}

.spinner > div {
  width: 18px;
  height: 18px;
  background-color: var(--primary);

  border-radius: 100%;
  display: inline-block;
  -webkit-animation: sk-bouncedelay 1.4s infinite ease-in-out both;
  animation: sk-bouncedelay 1.4s infinite ease-in-out both;
}

.spinner .bounce1 {
  -webkit-animation-delay: -0.32s;
  animation-delay: -0.32s;
}

.spinner .bounce2 {
  -webkit-animation-delay: -0.16s;
  animation-delay: -0.16s;
}

@-webkit-keyframes sk-bouncedelay {
  0%, 80%, 100% { -webkit-transform: scale(0) }
  40% { -webkit-transform: scale(1.0) }
}

@keyframes sk-bouncedelay {
  0%, 80%, 100% { 
    -webkit-transform: scale(0);
    transform: scale(0);
  } 40% { 
    -webkit-transform: scale(1.0);
    transform: scale(1.0);
  }
}

.message {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 4rem 2rem;
  color: var(--textSecondary);
  text-align: center;
}

.message i {
  font-size: 4rem;
  margin-bottom: 1rem;
}
</style>
