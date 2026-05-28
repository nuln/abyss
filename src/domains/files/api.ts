import { useAuthStore } from "@/domains/auth";
import { useLayoutStore } from "@/app/stores/layout";
import { baseURL } from "@/shared/utils/constants";
import { upload as postTus, useTus } from "@/shared/api/tus";
import { createURL, fetchJSON, fetchURL, removePrefix, StatusError } from "@/shared/api/utils";
import search from "@/shared/api/search";

async function fetchResource(url: string, signal?: AbortSignal) {
  url = removePrefix(url);
  let data: Resource;
  try {
    data = await fetchJSON<Resource>(`/api/resources${url}`, { signal });
  } catch (e) {
    if (e instanceof Error && e.name === "AbortError") {
      throw new StatusError("000 No connection", 0, true);
    }
    throw e;
  }
  data.url = `/files${url}`;

  if (data.isDir) {
    if (!data.url.endsWith("/")) data.url += "/";

    const authStore = useAuthStore();
    const currentUserID = authStore.user?.id;

    data.items = (data.items || []).map((item: any, index: any) => {
      item.index = index;
      if (item.ownerId && item.ownerId !== currentUserID && url === "") {
        item.url = `/shared/${item.id}`;
      } else {
        item.url = `${data.url}${encodeURIComponent(item.name)}`;
      }

      if (item.isDir && !item.url.endsWith("/")) {
        item.url += "/";
      }
      return item;
    });
  }

  return data;
}

async function resourceAction(url: string, method: ApiMethod, content?: any) {
  url = removePrefix(url);

  const opts: ApiOpts = { method };
  if (content) {
    opts.body = content;
  }

  return fetchURL(`/api/resources${url}`, opts);
}

async function remove(url: string, permanent = false) {
  url = removePrefix(url);
  const queryParam = permanent ? "?permanent=true" : "";
  return fetchURL(`/api/resources${url}${queryParam}`, { method: "DELETE" });
}

async function put(url: string, content = "") {
  return resourceAction(url, "PUT", content);
}

function download(format: any, ...filesToDownload: string[]) {
  let url = `${baseURL}/api/raw`;

  if (filesToDownload.length === 1) {
    url += removePrefix(filesToDownload[0]) + "?";
  } else {
    let arg = "";
    for (const file of filesToDownload) {
      arg += removePrefix(file) + ",";
    }
    arg = encodeURIComponent(arg.substring(0, arg.length - 1));
    url += `/?files=${arg}&`;
  }

  if (format) {
    url += `algo=${format}&`;
  }

  window.open(url);
}

async function post(url: string, content: ApiContent = "", overwrite = false, onupload: any = () => { }) {
  const useResourcesApi =
    url.endsWith("/") ||
    (content instanceof Blob && !["http:", "https:"].includes(window.location.protocol)) ||
    !(await useTus(content));

  return useResourcesApi
    ? postResources(url, content, overwrite, onupload)
    : postTus(url, content, overwrite, onupload);
}

async function postResources(url: string, content: ApiContent = "", overwrite = false, onupload: any) {
  url = removePrefix(url);

  let bufferContent: ArrayBuffer | undefined;
  if (content instanceof Blob && !["http:", "https:"].includes(window.location.protocol)) {
    bufferContent = await new Response(content).arrayBuffer();
  }

  return new Promise((resolve, reject) => {
    const request = new XMLHttpRequest();
    request.open("POST", `${baseURL}/api/resources${url}?override=${overwrite}`, true);
    request.setRequestHeader("X-Auth", useAuthStore().jwt);

    if (typeof onupload === "function") {
      request.upload.onprogress = onupload;
    }

    request.onload = () => {
      if (request.status >= 200 && request.status < 300) {
        resolve(request.responseText);
      } else {
        const message = request.responseText || `${request.status} ${request.statusText}`;
        reject(new StatusError(message, request.status));
      }
    };

    request.onerror = () => reject(new Error("001 Connection aborted"));
    request.send((bufferContent || content) as XMLHttpRequestBodyInit);
  });
}

function moveCopy(items: any[], copy = false, overwrite = false, rename = false) {
  const layoutStore = useLayoutStore();
  const promises = [];

  for (const item of items) {
    const from = item.from;
    const to = encodeURIComponent(removePrefix(item.to ?? ""));
    const url = `${from}?action=${copy ? "copy" : "rename"}&destination=${to}&override=${overwrite}&rename=${rename}`;
    promises.push(resourceAction(url, "PATCH"));
  }

  layoutStore.closeHovers();
  return Promise.all(promises);
}

function move(items: any[], overwrite = false, rename = false) {
  return moveCopy(items, false, overwrite, rename);
}

function copy(items: any[], overwrite = false, rename = false) {
  return moveCopy(items, true, overwrite, rename);
}

async function checksum(url: string, algo: ChecksumAlg) {
  const body: any = await fetchJSON(`/api/resources${url}?checksum=${algo}`, { method: "GET" });
  return body.checksums[algo];
}

function getDownloadURL(file: ResourceItem, inline: any) {
  const params = { ...(inline && { inline: "true" }) };
  return createURL("api/raw/" + file.path.replace(/^\//, ""), params);
}

function getPreviewURL(file: ResourceItem, size: string) {
  const params = {
    inline: "true",
    key: Date.parse(file.modified),
  };

  return createURL("api/preview/" + size + "/" + file.path.replace(/^\//, ""), params);
}

function getFileShares(url: string): Promise<any[]> {
  url = removePrefix(url);
  return fetchJSON<any[]>(`/api/resources${url}/shares`);
}

export const files = {
  fetch: fetchResource,
  remove,
  put,
  download,
  post,
  move,
  copy,
  checksum,
  getDownloadURL,
  getPreviewURL,
  getFileShares,
};

// Minimal user update API needed by files domain without pulling settings domain internals.
export const users = {
  update(user: Partial<IUser>, which = ["all"], currentPassword?: string, auth = true) {
    return fetchURL(
      `/api/users/${user.id}`,
      {
        method: "PUT",
        body: JSON.stringify({
          what: "user",
          which,
          data: user,
          currentPassword,
        }),
      },
      auth,
    );
  },
};

export { createURL, fetchJSON, fetchURL, removePrefix, StatusError, search };
export * as tus from "@/shared/api/tus";
