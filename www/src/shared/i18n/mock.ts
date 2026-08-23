export const detectLocale = () => "en";
export const setLocale = () => { /* noop */ };
export const i18n = {
    global: {
        t: (key: string) => key,
        d: (key: string) => key,
        n: (key: string) => key,
    }
};
export default i18n;
