import { type App } from "vue";
import { useToast, POSITION } from "vue-toastification";
import type { ToastOptions } from "vue-toastification/dist/types/types";
import CustomToast from "@/shared/ui/CustomToast.vue";
import { i18n, isRtl } from "@/shared/i18n";
import { StatusError } from "@/shared/api/utils";

const toastConfig = {
    position: POSITION.BOTTOM_CENTER,
    timeout: 4000,
    closeOnClick: true,
    pauseOnFocusLoss: true,
    pauseOnHover: true,
    draggable: true,
    draggablePercent: 0.6,
    showCloseButtonOnHover: false,
    hideProgressBar: false,
    closeButton: "button",
    icon: false,
} satisfies ToastOptions;

export function useGlobalToast(app: App) {
    const $toast = useToast();

    // Clear all existing toasts and show a new one
    const show = (
        fn: (content: any, options?: any) => void,
        message: string,
        type: "success" | "error",
        timeout: number = 4000
    ) => {
        $toast.clear();
        fn(
            {
                component: CustomToast,
                props: {
                    message: message,
                    type: type,
                },
            },
            { ...toastConfig, timeout, rtl: isRtl() }
        );
    };

    const showSuccess = (message: string) => {
        const lowMsg = message.toLowerCase();
        let translated = message;

        const successKeyMap: Record<string, string> = {
            "link copied": "success.linkCopied",
            "configuration updated": "settings.configUpdated",
            "plugin status updated": "settings.pluginStatusUpdated",
            "settings saved": "settings.configUpdated",
            "user created": "settings.userCreated",
            "user deleted": "settings.userDeleted",
            "migration completed": "settings.migrationSuccess",
            "gc completed": "settings.gcSuccess",
            "password changed": "settings.configUpdated",
            "otp enabled": "otp.enabledSuccessfully",
            "token generated": "webdav.tokenGenerated",
            "remote added": "rclone.remoteAdded",
            "remote updated": "rclone.remoteUpdated",
            "remote deleted": "rclone.remoteDeleted",
        };

        for (const [raw, key] of Object.entries(successKeyMap)) {
            if (lowMsg.includes(raw.toLowerCase())) {
                translated = i18n.global.t(key);
                break;
            }
        }

        show($toast.success, translated, "success");
    };

    const showError = (error: Error | string) => {
        // Log detailed error to console for debugging
        // console.error("[Abyss Debug Error]:", error);

        let message = "";

        if (error instanceof StatusError) {
            if (error.status === 409) {
                message = i18n.global.t("errors.conflict");
            } else {
                message = error.message;
            }
        } else if (error instanceof Error) {
            message = error.message;
        } else {
            message = error;
        }

        // Try to parse JSON error message if it looks like one
        if (typeof message === "string" && (message.startsWith("{") || message.startsWith("["))) {
            try {
                const parsed = JSON.parse(message);
                if (parsed.error) {
                    message = parsed.error;
                } else if (parsed.message) {
                    message = parsed.message;
                }
            } catch (_e) {
                // Ignore parse error, keep original message
            }
        }

        // Internationalization of common backend error strings
        const errorKeyMap: Record<string, string> = {
            "target storage type is required": "settings.targetTypeRequired",
            "source and target storage types are the same": "settings.sameSourceTarget",
            "migration is already running": "errors.migrationAlreadyRunning",
            "wrong password": "errors.wrongPassword",
            "incorrect password": "errors.wrongPassword",
            "missing totp code": "errors.missingTotpCode",
            "invalid totp token": "errors.invalidTotpToken",
            "totp not enabled": "errors.totpNotEnabled",
            "invalid totp code": "errors.invalidTotpCode",
            "forbidden": "errors.forbidden",
            "invalid action type": "errors.invalidActionType",
            "task not found": "errors.taskNotFound",
            "user uuid cannot be empty": "errors.userUuidRequired",
            "not implemented": "errors.notImplemented",
            "unsupported image format": "errors.unsupportedImageFormat",
            "image too large for thumbnail generation": "errors.imageTooLarge",
            "migration failed": "settings.migrationFailedGeneric",
            "password is empty": "errors.passwordEmpty",
            "password is too easy": "errors.passwordEasy",
            "email is empty": "errors.emailEmpty",
            "empty request": "errors.emptyRequest",
            "the resource already exists": "errors.resourceExists",
            "the resource does not exist": "errors.resourceNotExist",
            "invalid data type": "errors.invalidDataType",
            "permission denied": "errors.permissionDenied",
            "user with id 1 can't be deleted": "errors.rootUserDeletion",
            "the totp encryption key should be a 32-byte string encoded in base64": "errors.invalidEncryptionKey",
            "scope is a relative path": "errors.scopeIsRelative",
            "file is directory": "errors.isDirectory",
            "invalid option": "errors.invalidOption",
            "invalid auth method": "errors.invalidAuthMethod",
            "invalid request params": "errors.invalidRequestParams",
            "source is parent": "errors.sourceIsParent",
        };

        if (typeof message === "string") {
            const lowMsg = message.toLowerCase();

            // Special handling for dynamic errors
            const shortPasswordMatch = lowMsg.match(/password is too short, minimum length is (\d+)/);
            if (shortPasswordMatch) {
                const min = shortPasswordMatch[1];
                message = i18n.global.t("errors.passwordTooShort", { min });
            } else {
                let foundMatch = false;
                for (const [raw, key] of Object.entries(errorKeyMap)) {
                    if (lowMsg.includes(raw.toLowerCase())) {
                        message = i18n.global.t(key);
                        foundMatch = true;
                        break;
                    }
                }
                // If not found in map, attempt to translate by key if valid
                if (!foundMatch && message.includes(".")) {
                    const translated = i18n.global.t(message);
                    if (translated !== message) message = translated;
                }
            }
        }

        show($toast.error, message, "error", 5000); // 5s for errors
    };

    app.provide("$showSuccess", showSuccess);
    app.provide("$showError", showError);

    // Also inject into globalProperties for non-setup Abyss API SDK usage 
    // Example: fetchJSON inside registry.ts
     
    (app.config.globalProperties as any).$showSuccess = showSuccess;
     
    (app.config.globalProperties as any).$showError = showError;
}
