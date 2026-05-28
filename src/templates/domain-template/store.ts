import { defineStore } from "pinia";
import { listDomainEntities, type DomainEntity } from "./api";

export const useDomainStore = defineStore("domain-template", {
    state: () => ({
        entities: [] as DomainEntity[],
        loading: false,
    }),
    actions: {
        async fetchAll() {
            this.loading = true;
            try {
                this.entities = await listDomainEntities();
            } finally {
                this.loading = false;
            }
        },
    },
});
