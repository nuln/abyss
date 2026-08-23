<template>
  <div id="login" :class="{ recaptcha: recaptcha }">
    <form id="login-form" @submit="submit">
      <img :src="logoURL" alt="Abyss" />
      <h1>{{ name }}</h1>
      <p v-if="reason != null" class="logout-message">
        {{ t(`login.logout_reasons.${reason}`) }}
      </p>
      <div v-if="error !== ''" class="wrong">{{ error }}</div>

      <input
        id="login-email"
        autofocus
        class="input input--block"
        type="email"
        name="email"
        :autocomplete="createMode ? 'email' : 'username'"
        autocapitalize="off"
        v-model="email"
        :placeholder="t('login.email')"
        required
      />
      <input
        id="login-username"
        class="input input--block"
        v-if="createMode"
        type="text"
        name="username"
        autocomplete="username"
        v-model="username"
        maxlength="20"
        :placeholder="t('login.username')"
        required
      />
      <input
        id="login-password"
        class="input input--block"
        type="password"
        name="password"
        :autocomplete="createMode ? 'new-password' : 'current-password'"
        v-model="password"
        :placeholder="t('login.password')"
        required
      />
      <input
        id="login-confirm-password"
        class="input input--block"
        v-if="createMode"
        type="password"
        name="password_confirm"
        autocomplete="new-password"
        v-model="passwordConfirm"
        :placeholder="t('login.passwordConfirm')"
        required
      />

      <div v-if="recaptcha" id="recaptcha"></div>
      <input
        id="login-submit"
        class="button button--block"
        type="submit"
        :value="createMode ? t('login.signup') : t('login.submit')"
      />

      <component
        v-for="(comp, index) in loginButtons"
        :key="index"
        :is="comp"
      ></component>

      <button
        v-if="signup"
        type="button"
        class="button button--block button--outline"
        @click="toggleMode"
      >
        {{ createMode ? t("login.loginInstead") : t("login.createAnAccount") }}
      </button>
    </form>
  </div>
</template>

<script setup lang="ts">
import { StatusError } from "@/domains/auth/api";
import { useLayoutStore } from "@/app/stores/layout";
import * as auth from "@/domains/auth/utils";
import {
  name,
  logoURL,
  recaptcha,
  recaptchaKey,
  signup,
  demoEnabled,
  demoEmail,
  demoPassword,
} from "@/shared/utils/constants";
import { loadPlugin } from "@/plugin/loader";
import { inject, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";

// Define refs
const createMode = ref<boolean>(false);
const error = ref<string>("");
const email = ref<string>("");
const username = ref<string>("");
const password = ref<string>("");
const passwordConfirm = ref<string>("");

const route = useRoute();
const router = useRouter();
const { t } = useI18n({});
const layoutStore = useLayoutStore();
// Define functions
const toggleMode = () => (createMode.value = !createMode.value);

const $showError = inject<IToastError>("$showError")!;

const reason = route.query["logout-reason"] ?? null;

const loginButtons = window.__ABYSS__?.loginButtons || [];

const submit = async (event: Event) => {
  event.preventDefault();
  event.stopPropagation();

  const redirect = (route.query.redirect || "/files/") as string;

  let captcha = "";
  if (recaptcha) {
    captcha = window.grecaptcha.getResponse();

    if (captcha === "") {
      error.value = t("login.wrongCredentials");
      return;
    }
  }

  if (createMode.value) {
    if (password.value !== passwordConfirm.value) {
      error.value = t("login.passwordsDontMatch");
      return;
    }
  }

  try {
    if (createMode.value) {
      await auth.signup(email.value, username.value, password.value);
    }
    const res = await auth.login(email.value, password.value, captcha);
    if (res.otp) {
      // Ensure the OTP plugin is loaded before showing the prompt
      try {
        await loadPlugin("otp");
      } catch (_e) {
        // console.error("Failed to load otp plugin:", e);
      }
      
      layoutStore.showHover({
        prompt: "OtpPrompt",
        props: {
          token: res.token,
        },
        confirm: async () => {
          router.push({ path: redirect });
        },
      });
    } else {
      // auth.login() already stored both tokens and scheduled the silent
      // refresh; calling parseToken here would replace that with a hard
      // logout timer at access-token expiry.
      router.push({ path: redirect });
    }
  } catch (e: unknown) {
    // console.error(e);
    if (e instanceof StatusError) {
      if (e.status === 409) {
        error.value = t("login.usernameTaken");
      } else if (e.status === 403) {
        error.value = t("login.wrongCredentials");
      } else if (e.status === 400) {
        const match = e.message.match(/minimum length is (\d+)/);
        if (match) {
          error.value = t("login.passwordTooShort", { min: match[1] });
        } else {
          error.value = e.message;
        }
      } else {
        $showError(e);
      }
    }
  }
};

// Run hooks
onMounted(() => {
  if (demoEnabled && email.value === "" && password.value === "") {
    email.value = demoEmail;
    password.value = demoPassword;
  }

  if (!recaptcha) return;

  window.grecaptcha.ready(function () {
    window.grecaptcha.render("recaptcha", {
      sitekey: recaptchaKey,
    });
  });
});
</script>

<style scoped>
.button--outline {
  background: transparent;
  border: 1px solid var(--blue);
  color: var(--blue);
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 0.5em;
  margin-top: 0.5em;
}

.button--outline:hover {
  background: var(--blue);
  color: white;
}

.button--outline .material-icons {
  font-size: 1.2em;
}
</style>
