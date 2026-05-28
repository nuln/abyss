import { defineStore } from "pinia";
import { files as api, tus } from "@/domains/files/api";
import buttons from "@/shared/utils/buttons";
import { computed, inject, markRaw, ref } from "vue";

export const useFileStore = defineStore("file", {
    state: (): {
        req: Resource | null;
        oldReq: Resource | null;
        reload: boolean;
        selected: number[];
        multiple: boolean;
        isFiles: boolean;
        preselect: string | null;
    } => ({
        req: null,
        oldReq: null,
        reload: false,
        selected: [],
        multiple: false,
        isFiles: false,
        preselect: null,
    }),
    getters: {
        selectedCount: (state) => state.selected.length,
        isListing: (state) => state.isFiles && state?.req?.isDir,
    },
    actions: {
        toggleMultiple() {
            this.multiple = !this.multiple;
        },
        updateRequest(value: Resource | null) {
            const selectedItems = this.selected.map((i) => this.req?.items?.[i]);
            this.oldReq = this.req;
            this.req = value;

            this.selected = [];
            const items = this.req?.items || [];
            if (items.length > 0) {
                this.selected = items
                    .filter((item) =>
                        selectedItems.some((rItem) => rItem?.url === item.url)
                    )
                    .map((item) => item.index);
            }
        },
        removeSelected(value: any) {
            const i = this.selected.indexOf(value);
            if (i === -1) return;
            this.selected.splice(i, 1);
        },
        clearFile() {
            this.$reset();
        },
    },
});

const UPLOADS_LIMIT = 5;

const beforeUnload = (event: Event) => {
    event.preventDefault();
};

export const useUploadStore = defineStore("upload", () => {
    const $showError = inject<IToastError>("$showError")!;

    let progressInterval: number | null = null;

    const allUploads = ref<Upload[]>([]);
    const activeUploads = ref<Set<Upload>>(new Set());
    const lastUpload = ref<number>(-1);
    const totalBytes = ref<number>(0);
    const sentBytes = ref<number>(0);

    const upload = (
        path: string,
        name: string,
        file: File | null,
        overwrite: boolean,
        type: ResourceType
    ) => {
        if (!hasActiveUploads() && !hasPendingUploads()) {
            window.addEventListener("beforeunload", beforeUnload);
            buttons.loading("upload");
        }

        const upload: Upload = {
            path,
            name,
            file,
            overwrite,
            type,
            totalBytes: file?.size || 1,
            sentBytes: 0,
            rawProgress: markRaw({
                sentBytes: 0,
            }),
        };

        totalBytes.value += upload.totalBytes;
        allUploads.value.push(upload);

        processUploads();
    };

    const abort = () => {
        lastUpload.value = Infinity;
        tus.abortAllUploads();
    };

    const pendingUploadCount = computed(
        () =>
            allUploads.value.length -
            (lastUpload.value + 1) +
            activeUploads.value.size
    );

    const hasActiveUploads = () => activeUploads.value.size > 0;

    const hasPendingUploads = () =>
        allUploads.value.length > lastUpload.value + 1;

    const isActiveUploadsOnLimit = () => activeUploads.value.size < UPLOADS_LIMIT;

    const processUploads = async () => {
        if (!hasActiveUploads() && !hasPendingUploads()) {
            const fileStore = useFileStore();
            window.removeEventListener("beforeunload", beforeUnload);
            buttons.success("upload");
            reset();
            fileStore.reload = true;
        }

        if (isActiveUploadsOnLimit() && hasPendingUploads()) {
            if (!hasActiveUploads()) {
                progressInterval = window.setInterval(syncState, 1000);
            }

            const upload = nextUpload();

            if (upload.type === "dir") {
                await api.post(upload.path).catch($showError);
            } else {
                const onUpload = (event: ProgressEvent) => {
                    upload.rawProgress.sentBytes = event.loaded;
                };

                await api
                    .post(upload.path, upload.file!, upload.overwrite, onUpload)
                    .catch((err) => err.message !== "Upload aborted" && $showError(err));
            }

            finishUpload(upload);
        }
    };

    const nextUpload = (): Upload => {
        lastUpload.value++;

        const upload = allUploads.value[lastUpload.value];
        activeUploads.value.add(upload);

        return upload;
    };

    const finishUpload = (upload: Upload) => {
        sentBytes.value += upload.totalBytes - upload.sentBytes;
        upload.sentBytes = upload.totalBytes;
        upload.file = null;

        activeUploads.value.delete(upload);
        if (typeof window !== "undefined" && window.__ABYSS__) {
            window.__ABYSS__.emit("file:uploaded", {
                path: upload.path,
                name: upload.name,
                type: upload.type,
            });
        }
        processUploads();
    };

    const syncState = () => {
        for (const upload of activeUploads.value) {
            sentBytes.value += upload.rawProgress.sentBytes - upload.sentBytes;
            upload.sentBytes = upload.rawProgress.sentBytes;
        }
    };

    const reset = () => {
        if (progressInterval !== null) {
            clearInterval(progressInterval);
            progressInterval = null;
        }

        allUploads.value = [];
        activeUploads.value = new Set();
        lastUpload.value = -1;
        totalBytes.value = 0;
        sentBytes.value = 0;
    };

    return {
        activeUploads,
        totalBytes,
        sentBytes,
        upload,
        abort,
        pendingUploadCount,
    };
});
