import { fetchURL, fetchJSON } from './utils'

// Types
export interface IUser {
    id: number
    username: string
    email: string
    locale: string
}

export interface IAlbum {
    id: number
    userId: number
    parentId?: number  // 0 or undefined means root album
    name: string  // URL-friendly slug
    title: string
    description?: string
    coverPhotoId?: number
    photoCount: number
    childCount: number  // Number of sub-albums
    createdAt: number
    updatedAt: number
    isShared?: boolean
    hasShares?: boolean
}

export interface IShareDetail {
    recipientId: number
    recipientEmail: string
}

export interface IExifData {
    camera?: string
    lens?: string
    aperture?: string
    shutterSpeed?: string
    iso?: number
    focalLength?: string
    latitude?: number
    longitude?: number
}

export interface IPhoto {
    id: number
    albumId: number
    filename: string
    width: number
    height: number
    fileSize: number
    mimeType: string
    isVideo: boolean
    duration?: number
    takenAt?: number
    exif?: IExifData
    createdAt: number
}

export type ThumbnailSize = 'small' | 'medium' | 'large'

// Album API

export async function getAlbums(): Promise<IAlbum[]> {
    return fetchJSON<IAlbum[]>('/api/albums', {})
}

export async function getAlbum(id: number): Promise<IAlbum> {
    return fetchJSON<IAlbum>(`/api/albums/${id}`, {})
}


// Get album by path (for nested albums, e.g., "parent/child/grandchild")
export async function getAlbumByPath(path: string): Promise<IAlbum> {
    return fetchJSON<IAlbum>(`/api/albums/by-path/${path}`, {})
}

// Get sub-albums by path
export async function getSubAlbumsByPath(path: string): Promise<IAlbum[]> {
    return fetchJSON<IAlbum[]>(`/api/albums/by-path/${path}/albums`, {})
}

// Get photos by path
export async function getAlbumPhotosByPath(path: string): Promise<IPhoto[]> {
    return fetchJSON<IPhoto[]>(`/api/albums/by-path/${path}/photos`, {})
}

// Upload photo by path
export async function uploadPhotoByPath(
    path: string,
    file: File
): Promise<IPhoto> {
    const formData = new FormData()
    formData.append('file', file)

    return fetchJSON<IPhoto>(`/api/albums/by-path/${path}/photos`, {
        method: 'POST',
        body: formData
    })
}

// Create a sub-album inside an album by path
export async function createSubAlbumByPath(
    parentPath: string,
    title: string,
    description?: string
): Promise<IAlbum> {
    return fetchJSON<IAlbum>(`/api/albums/by-path/${parentPath}/albums`, {
        method: 'POST',
        body: JSON.stringify({ title, description }),
        headers: { 'Content-Type': 'application/json' }
    })
}

export async function createAlbum(
    title: string,
    description?: string
): Promise<IAlbum> {
    return fetchJSON<IAlbum>('/api/albums', {
        method: 'POST',
        body: JSON.stringify({ title, description }),
        headers: { 'Content-Type': 'application/json' }
    })
}

export async function updateAlbum(
    id: number,
    data: { title?: string; description?: string; coverPhotoId?: number }
): Promise<IAlbum> {
    return fetchJSON<IAlbum>(`/api/albums/${id}`, {
        method: 'PUT',
        body: JSON.stringify(data),
        headers: { 'Content-Type': 'application/json' }
    })
}

export async function deleteAlbum(id: number): Promise<void> {
    const res = await fetchURL(`/api/albums/${id}`, {
        method: 'DELETE'
    })
    if (res.status !== 204) {
        throw new Error(`Failed to delete album: ${res.status}`)
    }
}

export async function exportAlbum(id: number, name?: string): Promise<void> {
    const res = await fetchURL(`/api/albums/${id}/export`, {
        method: 'POST',
        body: JSON.stringify({ targetName: name }),
        headers: {
            'Content-Type': 'application/json'
        }
    })
    if (res.status !== 204) {
        throw new Error(`Failed to export album: ${res.status}`)
    }
}

export async function importFilesToAlbum(
    albumId: number,
    sourcePath: string,
    options?: { importImages?: boolean, importVideos?: boolean, fileNames?: string[] }
): Promise<{ imported: number }> {
    return fetchJSON<{ imported: number }>(`/api/albums/${albumId}/import`, {
        method: 'POST',
        body: JSON.stringify({
            sourcePath,
            importImages: options?.importImages ?? true,
            importVideos: options?.importVideos ?? true,
            fileNames: options?.fileNames ?? []
        }),
        headers: { 'Content-Type': 'application/json' }
    })
}

// Get sub-albums for an album
export async function getSubAlbums(albumId: number): Promise<IAlbum[]> {
    return fetchJSON<IAlbum[]>(`/api/albums/${albumId}/albums`, {})
}

// Create a sub-album inside an album
export async function createSubAlbum(
    parentId: number,
    title: string,
    description?: string
): Promise<IAlbum> {
    return fetchJSON<IAlbum>(`/api/albums/${parentId}/albums`, {
        method: 'POST',
        body: JSON.stringify({ title, description }),
        headers: { 'Content-Type': 'application/json' }
    })
}

// Photo API

export async function getAlbumPhotos(albumId: number): Promise<IPhoto[]> {
    return fetchJSON<IPhoto[]>(`/api/albums/${albumId}/photos`, {})
}


export async function getPhoto(id: number): Promise<IPhoto> {
    return fetchJSON<IPhoto>(`/api/photos/${id}`, {})
}

export async function uploadPhoto(
    albumId: number,
    file: File
): Promise<IPhoto> {
    const formData = new FormData()
    formData.append('file', file)

    return fetchJSON<IPhoto>(`/api/albums/${albumId}/photos`, {
        method: 'POST',
        body: formData
    })
}


export async function deletePhoto(id: number): Promise<void> {
    const res = await fetchURL(`/api/photos/${id}`, {
        method: 'DELETE'
    })
    if (res.status !== 204) {
        throw new Error(`Failed to delete photo: ${res.status}`)
    }
}

export async function movePhoto(id: number, targetAlbumId: number): Promise<void> {
    const res = await fetchURL(`/api/photos/${id}/move`, {
        method: 'POST',
        body: JSON.stringify({ targetAlbumId }),
        headers: { 'Content-Type': 'application/json' }
    })
    if (res.status !== 204) {
        throw new Error(`Failed to move photo: ${res.status}`)
    }
}

export async function copyPhoto(id: number, targetAlbumId: number): Promise<IPhoto> {
    return fetchJSON<IPhoto>(`/api/photos/${id}/copy`, {
        method: 'POST',
        body: JSON.stringify({ targetAlbumId }),
        headers: { 'Content-Type': 'application/json' }
    })
}

// URL helpers

export function getThumbnailUrl(
    photoId: number,
    size: ThumbnailSize = 'medium'
): string {
    return `/api/photos/${photoId}/thumbnail/${size}`
}

export function getOriginalUrl(photoId: number): string {
    return `/api/photos/${photoId}/original`
}

export async function shareAlbum(id: number, email: string): Promise<void> {
    const res = await fetchURL(`/api/albums/${id}/shares?action=add`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email })
    })
    if (res.status !== 204) {
        const error = await res.text()
        throw new Error(error || 'Failed to share album')
    }
}

export async function unshareAll(id: number): Promise<void> {
    const res = await fetchURL(`/api/albums/${id}/shares?action=removeAll`, {
        method: 'POST'
    })
    if (res.status !== 204) throw new Error('Failed to unshare album')
}

export async function unshareRecipient(id: number): Promise<void> {
    const res = await fetchURL(`/api/albums/${id}/shares?action=leave`, {
        method: 'POST'
    })
    if (res.status !== 204) throw new Error('Failed to unshare album')
}

export async function getAlbumShares(id: number): Promise<IShareDetail[]> {
    return fetchJSON<IShareDetail[]>(`/api/albums/${id}/shares`, {})
}

export async function unshareRecipientByID(albumId: number, recipientId: number): Promise<void> {
    const res = await fetchURL(`/api/albums/${albumId}/shares?action=remove`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ recipientId })
    })
    if (res.status !== 204) throw new Error('Failed to remove share')
}

