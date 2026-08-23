import * as Vue from "vue";
const { createApp } = Vue;
import dayjs from "dayjs";
import localizedFormat from "dayjs/plugin/localizedFormat";
import relativeTime from "dayjs/plugin/relativeTime";
import duration from "dayjs/plugin/duration";
import QrcodeVue from "qrcode.vue";
import App from "@/app/App.vue";
import router from "@/app/router";
import { registerProviders } from "@/app/providers";
import { createRegistry } from "@/plugin/registry";
import { initGlobal } from "@/plugin/bridge";
import { FilesLayout } from "@/domains/files";
import "@/styles/global.css";

// Dayjs plugins
[localizedFormat, relativeTime, duration].forEach((p) => dayjs.extend(p));

const app = createApp(App);
const { i18n } = registerProviders(app, router);
const registry = createRegistry(app, router, i18n);

initGlobal({ app, router, i18n, Layout: FilesLayout, QrcodeVue, registry });

router.isReady().then(() => app.mount("#app"));
