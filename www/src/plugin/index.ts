import { filesize } from "@/shared/utils";
import dayjs from "dayjs";
import * as auth from "@/domains/auth/utils";
import { files as fileApi } from "@/domains/files/api";
import { baseURL } from "@/shared/utils/constants";

export const formatDate = (val: number | string | Date) => {
    const d = typeof val === 'number' ? dayjs(val * 1000) : dayjs(val);
    return d.format('YYYY-MM-DD HH:mm:ss')
}

export const getThumbnailUrl = (id?: number, size: string = 'medium') => {
    return id ? `${baseURL}/api/photos/${id}/thumbnail/${size}` : ''
}

export const getOriginalUrl = (id?: number) => {
    return id ? `${baseURL}/api/photos/${id}/original` : ''
}

export const getTrashThumbnailUrl = (id: string) => {
    return `${baseURL}/api/trash/preview/thumb/${id}`
}

export {
    filesize,
    dayjs,
    auth,
    fileApi,
    baseURL
};

import { inject } from "vue";

export const ui = {
    useToast: () => ({
        success: inject<(msg: string) => void>("$showSuccess")!,
        error: inject<(err: any) => void>("$showError")!,
    })
};
