import { defineStore } from "pinia";
import { ref } from "vue";
import { useAuthStore } from "@/domains/auth";
import { fetchURL } from "@/domains/tasks/api";

export interface TaskInfo {
    id: string;
    slug: string;
    type: string;
    status: "pending" | "running" | "completed" | "failed" | "cancelled";
    progress: number;
    message: string;
    description: string;
    userId: number;
    createdAt: number;
    updatedAt: number;
    error?: string;
}

export const useTaskStore = defineStore("tasks", () => {
    const tasks = ref<Record<string, TaskInfo>>({});
    const eventSource = ref<EventSource | null>(null);
    const retryCount = ref(0);
    const maxRetryDelay = 30000;
    const running = ref(false);
    const timeoutId = ref<number | null>(null);

    const stop = () => {
        running.value = false;

        if (timeoutId.value) {
            clearTimeout(timeoutId.value);
            timeoutId.value = null;
        }

        if (eventSource.value) {
            eventSource.value.close();
            eventSource.value = null;
        }
    };

    const init = () => {
        const authStore = useAuthStore();
        if (!authStore.isLoggedIn) {
            running.value = false;
            return;
        }

        if (eventSource.value) return;

        if (timeoutId.value) {
            clearTimeout(timeoutId.value);
            timeoutId.value = null;
        }

        const baseURL = window.Abyss.BaseURL || "";
        running.value = true;
        const es = new EventSource(`${baseURL}/api/tasks/events`);

        es.onopen = () => {
            retryCount.value = 0;
        };

        es.onmessage = (event) => {
            try {
                const info: TaskInfo = JSON.parse(event.data);
                tasks.value[info.id] = info;
            } catch {
                return;
            }
        };

        es.onerror = () => {
            es.close();
            eventSource.value = null;

            if (!running.value) {
                return;
            }

            if (!authStore.isLoggedIn) {
                running.value = false;
                return;
            }

            fetchURL("/api/tasks", { method: "GET" })
                .then(() => {
                    if (!running.value) return;
                    const delay = Math.min(Math.pow(2, retryCount.value) * 1000, maxRetryDelay);
                    retryCount.value++;
                    timeoutId.value = window.setTimeout(init, delay);
                })
                .catch(() => {
                    if (!running.value || !authStore.isLoggedIn) return;
                    const delay = Math.min(Math.pow(2, retryCount.value) * 1000, maxRetryDelay);
                    retryCount.value++;
                    timeoutId.value = window.setTimeout(init, delay);
                });
        };

        eventSource.value = es;
    };

    const removeTask = (id: string) => {
        delete tasks.value[id];
    };

    return {
        tasks,
        init,
        stop,
        removeTask,
    };
});
