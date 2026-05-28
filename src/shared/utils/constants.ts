const name: string = window.Abyss.Name || "abyss";
const disableExternal: boolean = window.Abyss.DisableExternal;
const disableUsedPercentage: boolean = window.Abyss.DisableUsedPercentage;
const baseURL: string = window.Abyss.BaseURL;
const staticURL: string = window.Abyss.StaticURL;
const recaptcha: string = window.Abyss.ReCaptcha;
const recaptchaKey: string = window.Abyss.ReCaptchaKey;
const signup: boolean = window.Abyss.Signup;
const version: string = window.Abyss.Version;
const logoURL = `${staticURL}/img/logo.svg`;
const noAuth: boolean = window.Abyss.NoAuth;
const authMethod = window.Abyss.AuthMethod;
const logoutPage: string = window.Abyss.LogoutPage;
const loginPage: boolean = window.Abyss.LoginPage;
const theme: UserTheme = window.Abyss.Theme;
const enableThumbs: boolean = window.Abyss.EnableThumbs;
const resizePreview: boolean = window.Abyss.ResizePreview;
const tusSettings = window.Abyss.TusSettings && window.Abyss.TusSettings.chunkSize > 0 ? window.Abyss.TusSettings : null;
const origin = window.location.origin;
const tusEndpoint = `${baseURL}/api/tus`;
const hideLoginButton = window.Abyss.HideLoginButton;
const demoEnabled: boolean = window.Abyss.Demo;
const demoEmail: string = window.Abyss.DemoEmail;
const demoPassword: string = window.Abyss.DemoPassword;

// Feature flags
// @ts-ignore
const edition = __EDITION__;
(window as any).__EDITION__ = edition;
const isPro = edition === "pro";

export {
  name,
  disableExternal,
  disableUsedPercentage,
  baseURL,
  logoURL,
  recaptcha,
  recaptchaKey,
  signup,
  version,
  noAuth,
  authMethod,
  logoutPage,
  loginPage,
  theme,
  enableThumbs,
  resizePreview,
  tusSettings,
  origin,
  tusEndpoint,
  hideLoginButton,
  edition,
  isPro,
  demoEnabled,
  demoEmail,
  demoPassword,
};
