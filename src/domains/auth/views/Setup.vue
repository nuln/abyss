<template>
  <div id="setup">
    <form @submit.prevent="submit">
      <img :src="logoURL" alt="Abyss" />
      <h1>{{ $t("setup.title") }}</h1>
      <p>{{ $t("setup.description") }}</p>

      <div class="input-group">
        <label for="setup-username">{{ $t("login.username") }}</label>
        <input
          id="setup-username"
          class="input input--block"
          type="text"
          name="username"
          autocomplete="username"
          v-model="username"
          :placeholder="$t('login.username')"
          required
        />
      </div>

      <div class="input-group">
        <label for="setup-email">{{ $t("setup.email") }}</label>
        <input
          id="setup-email"
          class="input input--block"
          type="email"
          name="email"
          autocomplete="email"
          v-model="email"
          :placeholder="$t('setup.email')"
          required
        />
      </div>

      <div class="input-group">
        <label for="setup-password">{{ $t("setup.password") }}</label>
        <div class="input-with-icon">
          <input
            id="setup-password"
            class="input input--block"
            :type="showPassword ? 'text' : 'password'"
            name="password"
            autocomplete="new-password"
            v-model="password"
            :placeholder="$t('setup.password')"
            required
          />
          <i
            class="material-icons clickable"
            @click="showPassword = !showPassword"
          >{{ showPassword ? "visibility_off" : "visibility" }}</i>
        </div>
      </div>

      <div class="input-group">
        <label for="setup-confirm-password">{{ $t("setup.confirmPassword") }}</label>
        <div class="input-with-icon">
          <input
            id="setup-confirm-password"
            class="input input--block"
            :type="showPassword ? 'text' : 'password'"
            name="password_confirm"
            autocomplete="new-password"
            v-model="confirmPassword"
            :placeholder="$t('setup.confirmPassword')"
            required
          />
        </div>
      </div>

      <div v-if="error" class="error">{{ error }}</div>

      <button
        class="button button--block"
        type="submit"
        :disabled="loading"
      >
        {{ loading ? $t("setup.loading") : $t("setup.createAccount") }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { useAuthStore } from "@/domains/auth/store";
import { logoURL } from "@/shared/utils/constants";
import { fetchURL } from "@/domains/auth/api";
import i18n from "@/shared/i18n";

const authStore = useAuthStore();
const router = useRouter();

const email = ref("");
const username = ref("");
const password = ref("");
const confirmPassword = ref("");
const showPassword = ref(false);
const error = ref("");
const loading = ref(false);

const submit = async () => {
  if (password.value !== confirmPassword.value) {
    error.value = i18n.global.t("setup.passwordMismatch");
    return;
  }

  loading.value = true;
  error.value = "";

  try {
    const res = await fetchURL("/api/signup", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
      },
      body: JSON.stringify({
        email: email.value,
        username: username.value,
        password: password.value,
      }),
    }, false);

    if (res.ok) {
      // Refresh status and redirect to login
      await authStore.checkSetup();
      router.push("/login");
    } else {
      const data = await res.json();
      error.value = data.error || data.message || i18n.global.t("setup.error");
    }
  } catch (_e) {
    error.value = i18n.global.t("setup.error");
  } finally {
    loading.value = false;
  }
};
</script>

<style scoped>
#setup {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background-color: var(--background);
}

form {
  width: 100%;
  max-width: 400px;
  padding: 2rem;
  background-color: var(--surface);
  border-radius: var(--border-radius);
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
  text-align: center;
}

img {
  width: 80px;
  margin-bottom: 2rem;
}

h1 {
  margin-bottom: 0.5rem;
  font-size: 1.5rem;
}

p {
  margin-bottom: 2rem;
  color: var(--text-secondary);
}

.input-group {
  margin-bottom: 1.5rem;
  text-align: left;
}

.input-group label {
  display: block;
  margin-bottom: 0.5rem;
  font-size: 0.875rem;
  font-weight: 500;
  color: var(--text-secondary);
}

.input-with-icon {
  position: relative;
  display: flex;
  align-items: center;
}

.input-with-icon i {
  position: absolute;
  right: 1rem;
  font-size: 1.25rem;
  color: var(--text-secondary);
  user-select: none;
}

.input {
  margin-bottom: 0;
  padding-right: 3rem;
}

.error {
  margin-bottom: 1rem;
  color: var(--error);
  font-size: 0.875rem;
}

.button {
  margin-top: 1rem;
}
</style>
