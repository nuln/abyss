import { fetchJSON } from "@/shared/api/utils";

export interface DomainEntity {
    id: string;
    name: string;
}

export async function listDomainEntities(): Promise<DomainEntity[]> {
    return fetchJSON<DomainEntity[]>("/api/domain");
}
