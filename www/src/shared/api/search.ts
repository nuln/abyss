import { fetchJSON, removePrefix } from "./utils";
import url from "../utils/url";

export default async function search(base: string, query: string, signal?: AbortSignal) {
  base = removePrefix(base);
  query = encodeURIComponent(query);

  if (!base.endsWith("/")) {
    base += "/";
  }

  let data: any = await fetchJSON(`/api/search${base}?query=${query}`, { signal });

  data = data.map((item: ResourceItem & { dir: boolean }) => {
    item.url = `/files${base}` + url.encodePath(item.path);

    if (item.dir) {
      item.url += "/";
    }

    return item;
  });

  return data;
}
