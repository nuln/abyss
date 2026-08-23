<template>
  <div class="modal-overlay" @click.self="$emit('close')">
    <div class="modal-card file-selector-modal">
      <h2>{{ title }}</h2>
      
      <!-- Filter options -->
      <div v-if="showFilter" class="filter-options">
        <label class="filter-checkbox">
          <input type="checkbox" v-model="importImages" />
          <span>{{ t('albums.importImages') }}</span>
        </label>
        <label class="filter-checkbox">
          <input type="checkbox" v-model="importVideos" />
          <span>{{ t('albums.importVideos') }}</span>
        </label>
      </div>
      
      <!-- Current path breadcrumb -->
      <div class="path-breadcrumb">
        <span 
          class="breadcrumb-item" 
          :class="{ active: currentPath === '/' }"
          @click="navigateTo(-1)"
        >
          <i class="material-icons">home</i>
        </span>
        <template v-for="(segment, i) in pathSegments" :key="i">
          <span class="breadcrumb-separator">/</span>
          <span 
            class="breadcrumb-item" 
            :class="{ active: i === pathSegments.length - 1 }"
            @click="navigateTo(i)"
          >
            {{ segment }}
          </span>
        </template>
      </div>
      
      <!-- Loading state -->
      <div v-if="loading" class="folder-loading">
        <div class="spinner">
          <div class="bounce1"></div>
          <div class="bounce2"></div>
          <div class="bounce3"></div>
        </div>
      </div>
      
      <!-- Folder and file list -->
      <div v-else class="folder-list">
        <div v-if="allItems.length === 0" class="folder-empty">
          <i class="material-icons">folder_off</i>
          <span>{{ t('albums.noMediaFiles') }}</span>
        </div>
        
        <!-- Folders with checkbox -->
        <div 
          v-for="item in folders" 
          :key="item.name"
          class="folder-item directory"
          :class="{ active: selectedFolders.has(item.path) }"
        >
          <label class="item-checkbox" @click.stop>
            <input 
              type="checkbox" 
              :checked="selectedFolders.has(item.path)"
              @change="toggleFolderSelection(item.path)"
              :disabled="selectedFiles.size > 0"
            />
          </label>
          <div class="item-content" @click="enterFolder(item.path)">
            <i class="material-icons">folder</i>
            <span class="item-name">{{ item.name }}</span>
            <span class="item-hint">
              <i class="material-icons">chevron_right</i>
            </span>
          </div>
        </div>
        
        <!-- Media files with checkbox -->
        <div 
          v-for="item in mediaFiles" 
          :key="item.name"
          class="folder-item file"
          :class="{ active: selectedFiles.has(item.name) }"
        >
          <label class="item-checkbox" @click.stop>
            <input 
              type="checkbox" 
              :checked="selectedFiles.has(item.name)"
              @change="toggleFileSelection(item.name)"
              :disabled="selectedFolders.size > 0"
            />
          </label>
          <div class="item-content" @click="toggleFileSelection(item.name)">
            <i class="material-icons">{{ item.type === 'video' ? 'movie' : 'image' }}</i>
            <span class="item-name">{{ item.name }}</span>
          </div>
        </div>
      </div>
      
      <!-- Import mode hints -->
      <div class="import-hints">
        <!-- Multiple Folders Selected -->
        <div v-if="selectedFolders.size > 0" class="hint-item selected-folder">
          <i class="material-icons">create_new_folder</i>
          <span>{{ t('albums.importAsSubAlbum') }}: <strong>{{ selectedFolders.size }}</strong> {{ t('albums.foldersSelected') }}</span>
        </div>
        <!-- Specific Files Selected -->
        <div v-else-if="selectedFiles.size > 0" class="hint-item selected-files">
          <i class="material-icons">check_circle</i>
          <span>{{ t('albums.importSpecificFiles') }}: <strong>{{ selectedFiles.size }}</strong> {{ t('albums.mediaFilesCount') }}</span>
        </div>
        <!-- In a folder, no selection -> Import all media files here -->
        <div v-else-if="currentPath !== '/' && mediaFiles.length > 0" class="hint-item current-folder">
          <i class="material-icons">photo_library</i>
          <span>{{ t('albums.importFilesHere') }}: <strong>{{ mediaFiles.length }}</strong> {{ t('albums.mediaFilesCount') }}</span>
        </div>
        <!-- Empty folder or root -->
        <div v-else-if="currentPath !== '/'" class="hint-item empty-folder">
          <i class="material-icons">info</i>
          <span>{{ t('albums.selectFoldersToImport') }}</span>
        </div>
      </div>
      
      <div class="modal-actions">
        <button class="button button--flat button--grey" @click="$emit('close')">
          {{ t('buttons.cancel') }}
        </button>
        <!-- Import selected folders -->
        <button 
          v-if="selectedFolders.size > 0"
          class="button button--flat"
          :disabled="!importImages && !importVideos"
          @click="confirmImportAsSubAlbums"
        >
          {{ t('albums.importAsAlbum') }} ({{ selectedFolders.size }})
        </button>
        <!-- Import selected files OR all files if none selected -->
        <button 
          v-else
          class="button button--flat" 
          :disabled="currentPath === '/' || (mediaFiles.length === 0 && selectedFiles.size === 0) || (!importImages && !importVideos)"
          @click="confirmImportFiles"
        >
          {{ selectedFiles.size > 0 ? t('albums.importSelected') + ' (' + selectedFiles.size + ')' : confirmLabel }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { fetchJSON } from '@/domains/files/api'

const { t } = useI18n()

defineProps<{
  title: string
  showFilter?: boolean
  confirmLabel: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'select', path: string, options: { importImages: boolean, importVideos: boolean, createSubAlbum: boolean, subAlbumName?: string, fileNames?: string[] }): void
  (e: 'selectMultiple', folders: string[], options: { importImages: boolean, importVideos: boolean }): void
}>()

interface FolderItem {
  name: string
  isDir: boolean
  path: string
  type: string  // 'image', 'video', 'dir', etc.
}

const loading = ref(false)
const currentPath = ref('/')
const allItems = ref<FolderItem[]>([])
const selectedFolders = ref<Set<string>>(new Set())
const selectedFiles = ref<Set<string>>(new Set())
const importImages = ref(true)
const importVideos = ref(true)

const pathSegments = computed(() => {
  if (currentPath.value === '/') return []
  return currentPath.value.split('/').filter(s => s)
})

// Separate folders and media files
const folders = computed(() => allItems.value.filter(item => item.isDir))
const mediaFiles = computed(() => {
  return allItems.value.filter(item => {
    if (item.isDir) return false
    const type = item.type || ''
    // API returns 'image' or 'video', not MIME types
    const isImage = type === 'image'
    const isVideo = type === 'video'
    // Filter based on checkboxes
    if (isImage && importImages.value) return true
    if (isVideo && importVideos.value) return true
    return false
  })
})

onMounted(() => {
  loadFolder('/')
})

async function loadFolder(path: string) {
  loading.value = true
  selectedFolders.value.clear()
  selectedFiles.value.clear()
  try {
    const apiPath = path.startsWith('/') ? path : `/${path}`
    const data = await fetchJSON<any>(`/api/resources${apiPath}`)
    allItems.value = (data.items || [])
      .filter((item: any) => {
        if (item.isDir) return true
        // Only include image/video files
        const type = item.type || ''
        return type === 'image' || type === 'video'
      })
      .map((item: any) => ({
        name: item.name,
        isDir: item.isDir,
        type: item.type || '',
        path: path === '/' ? `/${encodeURIComponent(item.name)}` : `${path}/${encodeURIComponent(item.name)}`
      }))
    currentPath.value = path
  } catch (_e) {
    // console.error('Failed to load folder:', e)
    allItems.value = []
  } finally {
    loading.value = false
  }
}

function toggleFolderSelection(path: string) {
  if (selectedFiles.value.size > 0) return // Folder and file selection are mutually exclusive for UX clarity
  
  if (selectedFolders.value.has(path)) {
    selectedFolders.value.delete(path)
  } else {
    selectedFolders.value.add(path)
  }
  // Trigger reactivity
  selectedFolders.value = new Set(selectedFolders.value)
}

function toggleFileSelection(name: string) {
  if (selectedFolders.value.size > 0) return // Folder and file selection are mutually exclusive
  
  if (selectedFiles.value.has(name)) {
    selectedFiles.value.delete(name)
  } else {
    selectedFiles.value.add(name)
  }
  // Trigger reactivity
  selectedFiles.value = new Set(selectedFiles.value)
}

function enterFolder(path: string) {
  selectedFolders.value.clear()
  selectedFiles.value.clear()
  loadFolder(path)
}

function navigateTo(index: number) {
  selectedFolders.value.clear()
  selectedFiles.value.clear()
  if (index === -1) {
    loadFolder('/')
    return
  }
  const segments = pathSegments.value.slice(0, index + 1)
  const path = '/' + segments.join('/')
  loadFolder(path)
}

function getBasename(path: string): string {
  return path.split('/').pop() || ''
}

function confirmImportAsSubAlbums() {
  if (selectedFolders.value.size === 0) return
  
  // If single folder selected, use single emit
  if (selectedFolders.value.size === 1) {
    const folderPath = Array.from(selectedFolders.value)[0]
    emit('select', folderPath, {
      importImages: importImages.value,
      importVideos: importVideos.value,
      createSubAlbum: true,
      subAlbumName: getBasename(folderPath)
    })
  } else {
    // Multiple folders - emit all
    emit('selectMultiple', Array.from(selectedFolders.value), {
      importImages: importImages.value,
      importVideos: importVideos.value
    })
  }
}

function confirmImportFiles() {
  // If specific files are selected, send them
  const fileNames = selectedFiles.value.size > 0 ? Array.from(selectedFiles.value) : undefined
  
  emit('select', currentPath.value, {
    importImages: importImages.value,
    importVideos: importVideos.value,
    createSubAlbum: false,
    fileNames
  })
}
</script>

<style scoped>
/* Modal base styles */
.modal-overlay {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.5);
  display: flex;
  align-items: center;
  justify-content: center;
  z-index: 1000;
}

.modal-card {
  background: var(--surface-card, #fff);
  border-radius: 8px;
  padding: 1.5rem;
  box-shadow: 0 4px 20px rgba(0, 0, 0, 0.15);
}

.modal-card h2 {
  margin: 0 0 1rem;
  font-size: 1.125rem;
}

.file-selector-modal {
  width: 550px;
  max-width: 90vw;
  max-height: 80vh;
  display: flex;
  flex-direction: column;
}

.filter-options {
  display: flex;
  gap: 1.5rem;
  margin-bottom: 1rem;
  padding: 0.5rem 0;
  border-bottom: 1px solid var(--border, #e0e0e0);
}

.filter-checkbox {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  cursor: pointer;
}

.filter-checkbox input {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.path-breadcrumb {
  display: flex;
  align-items: center;
  gap: 0.25rem;
  padding: 0.5rem;
  background: var(--surfaceSecondary, #f5f5f5);
  border-radius: 4px;
  margin-bottom: 1rem;
  overflow-x: auto;
  flex-wrap: nowrap;
}

.breadcrumb-item {
  display: flex;
  align-items: center;
  padding: 0.25rem 0.5rem;
  border-radius: 4px;
  cursor: pointer;
  white-space: nowrap;
}

.breadcrumb-item:hover {
  background: var(--surfaceHover, #e8e8e8);
}

.breadcrumb-item.active {
  font-weight: 500;
  color: var(--primary, #2196f3);
}

.breadcrumb-item i {
  font-size: 18px;
}

.breadcrumb-separator {
  color: var(--textSecondary, #666);
}

.folder-list {
  flex: 1;
  min-height: 200px;
  max-height: 380px;
  overflow-y: auto;
  border: 1px solid var(--border, #e0e0e0);
  border-radius: 4px;
  margin-bottom: 1rem;
}

.folder-loading {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 200px;
}

.folder-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 2rem;
  color: var(--textSecondary, #666);
  gap: 0.5rem;
}

.folder-empty i {
  font-size: 48px;
  opacity: 0.5;
}

.folder-item {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  padding: 0.5rem 1rem;
  border-bottom: 1px solid var(--border, #e0e0e0);
  transition: background-color 0.15s;
}

.folder-item:last-child {
  border-bottom: none;
}

.folder-item.active {
  background: var(--primaryLight, #e3f2fd);
}

.folder-item:hover {
  background: var(--surfaceHover, #f0f0f0);
}

.item-checkbox {
  display: flex;
  align-items: center;
  padding: 0.25rem;
  cursor: pointer;
}

.item-checkbox input {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.item-content {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  flex: 1;
  padding: 0.25rem;
  cursor: pointer;
  border-radius: 4px;
}

.folder-item.file {
  padding-left: 1rem;
}

.folder-item.file .item-checkbox {
  margin-left: 0.25rem;
}

.folder-item i {
  font-size: 22px;
  color: var(--textSecondary, #666);
}

.folder-item.directory .item-content i {
  color: var(--primary, #2196f3);
}

.item-name {
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.item-hint {
  color: var(--textSecondary, #666);
}

.item-hint i {
  font-size: 18px;
}

.import-hints {
  margin-bottom: 1rem;
}

.hint-item {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  padding: 0.75rem;
  border-radius: 4px;
  font-size: 0.875rem;
}

.hint-item i {
  font-size: 20px;
}

.hint-item.selected-folder {
  background: var(--primaryLight, #e3f2fd);
  color: var(--primary, #1976d2);
}

.hint-item.selected-files {
  background: var(--successLight, #e8f5e9);
  color: var(--success, #388e3c);
}

.hint-item.current-folder {
  background: var(--surfaceSecondary, #f5f5f5);
  color: var(--textSecondary, #666);
}

.hint-item.empty-folder {
  background: var(--surfaceSecondary, #f5f5f5);
  color: var(--textSecondary, #666);
}

.hint-item strong {
  font-weight: 600;
}

.modal-actions {
  display: flex;
  justify-content: flex-end;
  gap: 0.75rem;
}

.button {
  padding: 0.5rem 1rem;
  border-radius: 4px;
  border: none;
  cursor: pointer;
  font-size: 0.875rem;
  font-weight: 500;
  transition: background-color 0.2s;
}

.button--flat {
  background: var(--primary, #2196f3);
  color: white;
}

.button--flat:hover {
  background: var(--primary-hover, #1976d2);
}

.button--flat:disabled {
  background: var(--disabled, #ccc);
  cursor: not-allowed;
}

.button--grey {
  background: var(--surface-secondary, #e0e0e0);
  color: var(--text-primary, #333);
}

.button--grey:hover {
  background: var(--surface-hover, #d0d0d0);
}

/* Spinner styles */
.spinner {
  display: flex;
  gap: 4px;
}

.spinner > div {
  width: 12px;
  height: 12px;
  background-color: var(--primary, #2196f3);
  border-radius: 100%;
  animation: bounce 1.4s infinite ease-in-out both;
}

.spinner .bounce1 {
  animation-delay: -0.32s;
}

.spinner .bounce2 {
  animation-delay: -0.16s;
}

@keyframes bounce {
  0%, 80%, 100% {
    transform: scale(0);
  }
  40% {
    transform: scale(1);
  }
}
</style>
