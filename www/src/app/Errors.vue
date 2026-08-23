<template>
  <div class="error-page" :class="{ 'error-page--inline': props.inline }">
    <header-bar v-if="showHeader && !props.inline" showMenu showLogo />

    <div class="error-content">
      <div class="error-card">
        <div class="icon-wrapper" :class="'error-' + props.errorCode">
          <i class="material-icons">{{ info.icon }}</i>
        </div>
        <h1 class="error-code">{{ props.errorCode }}</h1>
        <p class="error-message">{{ t(info.message) }}</p>
        <router-link v-if="!props.inline" to="/" class="home-button">
          <i class="material-icons">home</i>
          <span>{{ t("files.home") }}</span>
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import HeaderBar from "@/shared/ui/header/HeaderBar.vue";
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const { t } = useI18n({});

const errors: {
  [key: number]: {
    icon: string;
    message: string;
  };
} = {
  0: {
    icon: "cloud_off",
    message: "errors.connection",
  },
  403: {
    icon: "lock_outline",
    message: "errors.forbidden",
  },
  404: {
    icon: "search_off",
    message: "errors.notFound",
  },
  500: {
    icon: "error_outline",
    message: "errors.internal",
  },
};

const props = withDefaults(
  defineProps<{
    errorCode?: number;
    showHeader?: boolean;
    inline?: boolean;
  }>(),
  {
    errorCode: 500,
    showHeader: false,
    inline: false,
  },
);

const info = computed(() => {
  return errors[props.errorCode] ? errors[props.errorCode] : errors[500];
});
</script>

<style scoped>
.error-page {
  display: flex;
  flex-direction: column;
}

.error-page:not(.error-page--inline) {
  min-height: 100vh;
  background-color: var(--surface-page);
}

.error-page--inline {
  padding: var(--space-8) 0;
  flex: 1;
}

.error-content {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-6);
}

.error-card {
  max-width: 480px;
  width: 100%;
  text-align: center;
  padding: var(--space-10) var(--space-6);
  background: var(--surface-card);
  border-radius: var(--radius-2xl);
  box-shadow: var(--shadow-2xl);
  border: 1px solid var(--border-subtle);
  backdrop-filter: blur(8px);
  animation: fadeIn 0.6s ease-out;
}

.icon-wrapper {
  width: 100px;
  height: 100px;
  margin: 0 auto var(--space-6);
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: transform var(--transition-normal);
  background: var(--color-gray-100);
}

.error-card:hover .icon-wrapper {
  transform: scale(1.05) rotate(5deg);
}

.icon-wrapper i {
  font-size: 52px;
}

.icon-wrapper.error-404 {
  background: hsla(210, 100%, 96%, 1);
}
.icon-wrapper.error-404 i {
  color: var(--color-accent);
}

.icon-wrapper.error-403 {
  background: hsla(38, 100%, 96%, 1);
}
.icon-wrapper.error-403 i {
  color: var(--color-warning);
}

.icon-wrapper.error-500 {
  background: hsla(0, 100%, 96%, 1);
}
.icon-wrapper.error-500 i {
  color: var(--color-error);
}

.error-code {
  font-size: 80px;
  font-weight: 900;
  margin: 0 0 var(--space-1);
  line-height: 1;
  letter-spacing: -0.05em;
  background: linear-gradient(
    135deg,
    var(--text-primary) 0%,
    var(--text-tertiary) 100%
  );
  background-clip: text;
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
}

.error-message {
  font-size: var(--text-base);
  color: var(--text-secondary);
  margin-bottom: var(--space-8);
  font-weight: 500;
}

.home-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-8);
  background: var(--color-primary);
  color: var(--text-inverse);
  border-radius: var(--radius-full);
  font-weight: 600;
  transition: all var(--transition-fast);
  box-shadow: var(--shadow-lg);
  text-decoration: none;
}

.home-button:hover {
  background: var(--color-primary-dark);
  transform: translateY(-2px);
  box-shadow: var(--shadow-xl);
  color: var(--text-inverse);
}

.home-button i {
  font-size: 20px;
}

@keyframes fadeIn {
  from {
    opacity: 0;
    transform: translateY(30px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}

@media (max-width: 640px) {
  .error-code {
    font-size: 64px;
  }
  .error-card {
    padding: var(--space-8) var(--space-4);
  }
  .icon-wrapper {
    width: 80px;
    height: 80px;
  }
  .icon-wrapper i {
    font-size: 40px;
  }
}
</style>
