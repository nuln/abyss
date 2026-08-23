<template>
  <div v-show="active" @click="closeHovers" class="overlay"></div>
  
  <!-- Menu toggle button - shown when sidebar is hidden and not on share page -->
  <button 
    v-show="!sidebarVisible" 
    class="sidebar-toggle-btn"
    @click="showSidebar"
    :aria-label="$t('sidebar.showSidebar')"
    :title="$t('sidebar.showSidebar')"
  >
    <i class="material-icons">menu</i>
  </button>

  <nav :class="{ active, hidden: !sidebarVisible }">
    <template v-if="isLoggedIn">
      <!-- Top brand row with add button -->
      <div class="sidebar-brand">
        <button class="brand-btn" @click="toRoot" :title="siteName" :aria-label="siteName">
          <img :src="logoURL" alt="Abyss" class="sidebar-logo" />
          <span class="brand-name">{{ siteName }}</span>
        </button>
        <ActionSlot name="sidebar-top" />
      </div>

      <!-- My Files toggle row (collapses/expands folder tree) -->
      <div
        class="action action-small myfiles-action"
        :aria-label="$t('sidebar.myFiles')"
        :title="$t('sidebar.myFiles')"
      >
        <button
          class="arrow-btn"
          @click.stop="toggleFolderCollapsed"
          :class="{ expanded: !folderCollapsed }"
          :aria-label="folderCollapsed ? '展开' : '收起'"
        >
          <i class="material-icons">chevron_right</i>
        </button>
        <button
          class="label-btn"
          @click="toRoot"
          :aria-label="$t('sidebar.myFiles')"
          :title="$t('sidebar.myFiles')"
        >
          <i class="material-icons">home</i>
          <span>{{ $t("sidebar.myFiles") }}</span>
        </button>
      </div>

      <transition name="folder-expand">
        <div v-if="user?.perm?.create && !folderCollapsed" class="folder-tree-container">
          <FolderTree @navigate="closeHovers" />
        </div>
      </transition>

      <!-- Dynamic Plugin Sidebar Items -->
      <template v-for="page in visibleSidebarPages" :key="page.slugName">
        <div
          class="action action-small myfiles-action"
          :aria-label="$t(page.name)"
          :title="$t(page.name)"
        >
          <button
            v-if="page.sidebarComponent"
            class="arrow-btn"
            @click.stop="toggleSection(page.slugName)"
            :class="{ expanded: isExpanded(page.slugName) }"
            :aria-label="isExpanded(page.slugName) ? '收起' : '展开'"
          >
            <i class="material-icons">chevron_right</i>
          </button>
          <router-link
            class="label-btn"
            :to="page.route"
            :aria-label="$t(page.name)"
            :title="$t(page.name)"
            @click="onPluginClick(page.slugName)"
          >
            <i class="material-icons">{{ page.icon }}</i>
            <span>{{ $t(page.name) }}</span>
          </router-link>
        </div>

        <transition name="folder-expand">
          <div v-if="page.sidebarComponent && isExpanded(page.slugName)" class="folder-tree-container">
            <component :is="page.sidebarComponent" @navigate="closeHovers" />
          </div>
        </transition>
      </template>

      <!-- Spacer to push bottom section down (only when trees collapsed) -->
      <div v-if="folderCollapsed && allSectionsCollapsed" class="sidebar-spacer"></div>

      <!-- Icon-Only Bottom Area (Single Row) -->
      <div class="sidebar-icon-area">
        <div class="footer-row">
          <div class="footer-left">
            <!-- Settings -->
            <button
              class="icon-btn"
              @click="toAccountSettings"
              :aria-label="$t('settings.profileSettings')"
              :title="$t('settings.profileSettings')"
            >
              <i class="material-icons">settings</i>
            </button>

            <!-- App Launcher (Consolidated entry for plugins) -->
            <div 
              v-if="allPluginPages.length > 0"
              class="launcher-container" 
            >
              <button
                class="icon-btn launcher-btn"
                :class="{ active: showAppLauncher }"
                :aria-label="$t('sidebar.apps')"
                :title="$t('sidebar.apps')"
                @click.stop="showAppLauncher = !showAppLauncher"
              >
                <i class="material-icons">apps</i>
              </button>
              
              <transition name="launcher-fade">
                <div 
                  v-if="showAppLauncher" 
                  class="launcher-popup"
                  @click.stop
                >
                  <div class="launcher-grid">
                    <template v-for="page in allPluginPages" :key="page.slugName">
                      <router-link
                        class="launcher-item"
                        :to="page.route"
                        @click="handleLauncherClick(page.slugName)"
                        :title="$t(page.name)"
                      >
                        <div class="launcher-icon">
                          <i class="material-icons">{{ page.icon }}</i>
                        </div>
                      </router-link>
                    </template>
                  </div>
                </div>
              </transition>
            </div>

            <!-- Help -->
            <button
              class="icon-btn"
              @click="help"
              :aria-label="$t('sidebar.help')"
              :title="$t('sidebar.help')"
            >
              <i class="material-icons">help_outline</i>
            </button>
          </div>

          <div class="footer-right">
            <!-- Logout -->
            <button
              v-if="canLogout"
              class="icon-btn"
              @click="logout"
              :aria-label="$t('sidebar.logout')"
              :title="$t('sidebar.logout')"
            >
              <i class="material-icons">exit_to_app</i>
            </button>

            <!-- Hide/Collapse -->
            <button
              class="icon-btn hide-btn"
              @click="hideSidebar"
              :aria-label="$t('sidebar.hideSidebar')"
              :title="$t('sidebar.hideSidebar')"
            >
              <i class="material-icons">chevron_left</i>
            </button>
          </div>
        </div>
      </div>
    </template>
    <template v-else>
      <router-link
        v-if="!hideLoginButton"
        class="action"
        to="/login"
        :aria-label="$t('sidebar.login')"
        :title="$t('sidebar.login')"
      >
        <i class="material-icons">exit_to_app</i>
        <span>{{ $t("sidebar.login") }}</span>
      </router-link>

      <router-link
        v-if="signup"
        class="action"
        to="/login"
        :aria-label="$t('sidebar.signup')"
        :title="$t('sidebar.signup')"
      >
        <i class="material-icons">person_add</i>
        <span>{{ $t("sidebar.signup") }}</span>
      </router-link>
    </template>
  </nav>
</template>

<script>
import { mapActions, mapState } from "pinia";
import { useAuthStore } from "@/domains/auth";
import { useFileStore } from "@/domains/files/store";
import { useLayoutStore } from "@/app/stores/layout";
import { usePluginStore } from "@/domains/settings";
import FolderTree from "@/domains/files/components/FolderTree.vue";
import ActionSlot from "@/shared/ui/ActionSlot.vue";

import * as auth from "@/domains/auth/utils";
import {
  name,
  logoURL,
  version,
  signup,
  hideLoginButton,
  disableExternal,
  noAuth,
  logoutPage,
  loginPage,
  isPro,
} from "@/shared/utils/constants";

const SIDEBAR_FOLDER_COLLAPSED_KEY = "abyss.sidebar.folderCollapsed";
const SIDEBAR_EXPANDED_SECTIONS_KEY = "abyss.sidebar.expandedSections";

function safeParseExpandedSections() {
  try {
    const raw = localStorage.getItem(SIDEBAR_EXPANDED_SECTIONS_KEY);
    if (!raw) return ["albums"];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed.filter((v) => typeof v === "string") : ["albums"];
  } catch {
    return ["albums"];
  }
}

function safeReadFolderCollapsed() {
  try {
    return localStorage.getItem(SIDEBAR_FOLDER_COLLAPSED_KEY) === "1";
  } catch {
    return false;
  }
}

export default {
  name: "sidebar",
  components: {
    FolderTree,
    ActionSlot,
  },
  setup() {
    return { isPro };
  },
  data() {
    return {
      folderCollapsed: safeReadFolderCollapsed(),
      expandedSections: safeParseExpandedSections(),
      showAppLauncher: false,
    };
  },
  inject: ["$showError"],
  computed: {
    ...mapState(useAuthStore, ["user", "isLoggedIn"]),
    ...mapState(useFileStore, ["isFiles", "reload"]),
    ...mapState(useLayoutStore, ["currentPromptName", "sidebarVisible"]),
    ...mapState(usePluginStore, ["footerPages", "pages", "sidebarPages"]),
    visibleSidebarPages() {
      return this.sidebarPages.filter(
        (p) => p.slugName !== "video" && p.slugName !== "music",
      );
    },
    active() {
      return this.currentPromptName === "sidebar";
    },
    signup: () => signup,
    hideLoginButton: () => hideLoginButton,
    version: () => version,
    siteName: () => name,
    logoURL: () => logoURL,
    disableExternal: () => disableExternal,
    canLogout: () => !noAuth && (loginPage || logoutPage !== "/login"),
    allSectionsCollapsed() {
      return this.visibleSidebarPages.every(p => !p.sidebarComponent || !this.isExpanded(p.slugName));
    },
    isFilesPage() {
      return this.$route.path.startsWith('/files');
    },
    allPluginPages() {
      // Show all full-page plugins in the launcher, except for those that should be sidebar-only (like albums)
      return this.pages.filter(p => 
        (!p.navPosition || p.navPosition !== 'settings' && p.navPosition !== 'none') && 
        p.slugName !== 'albums'
      );
    },
  },
  mounted() {
    document.addEventListener('click', this.closeAppLauncher);
  },
  beforeUnmount() {
    document.removeEventListener('click', this.closeAppLauncher);
  },
  methods: {
    ...mapActions(useLayoutStore, ["closeHovers", "showHover", "showSidebar", "hideSidebar"]),
    onPluginClick() {
      this.closeHovers();
    },
    closeAppLauncher() {
      this.showAppLauncher = false;
    },
    isLast(p) {
      return this.pages.indexOf(p) === this.pages.length - 1;
    },
    toggleFolderCollapsed() {
      this.folderCollapsed = !this.folderCollapsed;
      if (!this.folderCollapsed) {
        this.expandedSections = [];
      }
      this.persistSidebarState();
      this.closeHovers();
    },
    toggleSection(slug) {
      const index = this.expandedSections.indexOf(slug);
      if (index > -1) {
        this.expandedSections.splice(index, 1);
      } else {
        this.expandedSections.push(slug);
        this.folderCollapsed = true;
      }
      this.persistSidebarState();
      this.closeHovers();
    },
    isExpanded(slug) {
      return this.expandedSections.includes(slug);
    },
    toRoot() {
      this.$router.push({ path: "/files" });
      this.closeHovers();
      this.folderCollapsed = false;
      this.expandedSections = [];
      this.persistSidebarState();
    },
    toAccountSettings() {
      this.$router.push({ path: "/settings/profile" });
      this.closeHovers();
    },
    toGlobalSettings() {
      this.$router.push({ path: "/settings/global" });
      this.closeHovers();
    },
    help() {
      this.showHover("help");
    },
    handleLauncherClick(slug) {
      this.showAppLauncher = false;
      if (slug === 'video' || slug === 'music') {
        this.hideSidebar();
      }
    },
    persistSidebarState() {
      try {
        localStorage.setItem(SIDEBAR_FOLDER_COLLAPSED_KEY, this.folderCollapsed ? "1" : "0");
        localStorage.setItem(
          SIDEBAR_EXPANDED_SECTIONS_KEY,
          JSON.stringify(this.expandedSections),
        );
      } catch {
        // Ignore storage write errors in private mode.
      }
    },
    logout: auth.logout,
  },
};
</script>

<style scoped>
.usage-text {
  font-size: 0.85em;
  color: var(--text-secondary, #666);
  line-height: 1.2;
}

/* Folder tree container */
.folder-tree-container {
  flex: 1;
  overflow-y: auto;
  overflow-x: hidden;
  padding: var(--space-1) var(--space-2);
  padding-left: calc(var(--space-3) + 12px);
  border-top: 1px solid var(--border-subtle);
  margin-bottom: 0;
}

.folder-tree-container::-webkit-scrollbar {
  width: 4px;
}

.folder-tree-container::-webkit-scrollbar-track {
  background: transparent;
}

.folder-tree-container::-webkit-scrollbar-thumb {
  background: var(--color-gray-300);
  border-radius: 2px;
}

/* Spacer to push bottom section down */
.sidebar-spacer {
  flex: 1;
}

/* Icon-Only Bottom Area */
.sidebar-icon-area {
  border-top: 1px solid var(--border-subtle);
  padding: 8px 12px;
  margin-top: auto;
  flex-shrink: 0;
  background: var(--surface-card);
}

.footer-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}

.footer-left, .footer-right {
  display: flex;
  align-items: center;
  gap: 4px;
}

/* Icon Button - square with icon only */
.icon-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  background: transparent;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.2s ease;
  text-decoration: none;
  flex-shrink: 0;
}

.icon-btn:hover {
  background: var(--color-gray-100);
}

.icon-btn.active {
  background: var(--color-accent-soft);
}

.icon-btn i {
  font-size: 18px;
  color: var(--text-primary); /* Always dark to match tree icons */
  transition: none; /* No color transition as it stays dark */
}

.icon-btn.active i {
  color: var(--color-accent);
}

.icon-btn.hide-btn {
  opacity: 0;
  pointer-events: none;
  width: 16px;
  background: var(--color-gray-100);
  border-radius: 4px 0 0 4px;
  margin-right: -12px;
}

.icon-btn.hide-btn i {
  font-size: 16px;
  margin-left: -2px;
}

.sidebar-icon-area:hover .icon-btn.hide-btn {
  opacity: 1;
  pointer-events: auto;
}

/* Sidebar toggle button - shown when sidebar is hidden */
.sidebar-toggle-btn {
  position: fixed;
  bottom: var(--space-6, 1.5rem);
  left: var(--space-6, 1.5rem);
  z-index: var(--z-fixed, 1000);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 48px;
  height: 48px;
  background: var(--surface-card);
  border: 1px solid var(--border-subtle);
  border-radius: 50%;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  cursor: pointer;
  transition: all var(--transition-fast, 0.15s);
}

.sidebar-toggle-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 6px 16px rgba(0, 0, 0, 0.2);
  background: var(--surface-card-hover, var(--surface-card));
}

.sidebar-toggle-btn i {
  font-size: 24px;
  color: var(--text-primary);
}

/* Hidden state for nav */
nav.hidden {
  transform: translateX(-100%);
}

html[dir="rtl"] nav.hidden {
  transform: translateX(100%);
}

html[dir="rtl"] .sidebar-toggle-btn {
  left: auto;
  right: var(--space-4, 1rem);
}

html[dir="rtl"] .icon-btn:last-child i {
  transform: scaleX(-1);
}

/* Small icon/label split for the My files action */
.action .icon-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  background: transparent;
  border: none;
  padding: 0;
  margin: 0;
  cursor: pointer;
}

.action .label-btn {
  background: transparent;
  border: none;
  padding: 0;
  margin: 0;
  flex: 1;
  text-align: left;
  display: flex;
  align-items: center;
  gap: 0.5rem;
}

.sidebar-brand {
  padding: var(--space-2) var(--space-3);
  display: flex;
  align-items: center;
  justify-content: space-between;
  border-bottom: 1px solid var(--border-subtle);
}

.brand-btn {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  background: transparent;
  border: none;
  padding: 0;
  margin: 0;
  cursor: pointer;
}

.sidebar-logo {
  width: 20px;
  height: 20px;
  object-fit: contain;
  margin-right: 0.25rem;
  flex-shrink: 0;
}

.brand-name {
  font-weight: 600;
  color: var(--text-primary);
}

.brand-add-btn {
  width: 28px;
  height: 28px;
  flex-shrink: 0;
  background: transparent;
  border: none;
}

.brand-add-btn i {
  font-size: 20px;
  color: var(--text-primary);
}

.brand-add-btn:hover i {
  color: var(--text-secondary);
}

/* My Files action row */
.myfiles-action {
  display: flex;
  align-items: center;
  gap: 0;
  padding-left: var(--space-3) !important;
}

.myfiles-action .arrow-btn + .label-btn {
  margin-left: 0;
}

.myfiles-action .arrow-btn {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 14px;
  height: 20px;
  padding: 0;
  margin: 0;
  margin-right: -3px;
  background: transparent;
  border: none;
  cursor: pointer;
  transition: transform 0.15s ease;
  flex-shrink: 0;
}

.myfiles-action .arrow-btn i {
  font-size: 18px;
  color: var(--text-primary);
}

.myfiles-action .arrow-btn.expanded {
  transform: rotate(90deg);
}

.myfiles-action .arrow-btn:hover i {
  color: var(--text-secondary);
}

.myfiles-action .label-btn {
  gap: 0;
  padding-left: 0;
  cursor: pointer;
}

.myfiles-action .label-btn i {
  font-size: 18px;
  margin-right: 0;
}

/* Folder tree expand/collapse animation */
.folder-expand-enter-active,
.folder-expand-leave-active {
  transition: all 0.25s ease-out;
  overflow: hidden;
}

.folder-expand-enter-from,
.folder-expand-leave-to {
  opacity: 0;
  max-height: 0;
  transform: translateY(-8px);
}

.folder-expand-enter-to,
.folder-expand-leave-from {
  opacity: 1;
  max-height: 1000px;
  transform: translateY(0);
}

.myfiles-action .label-btn i {
  color: var(--text-primary);
}

.myfiles-action .label-btn span {
  color: var(--text-primary);
}

/* App Launcher Styles */
.launcher-container {
  position: relative;
  overflow: visible;
}

.launcher-popup {
  position: absolute;
  bottom: 0px;
  left: 42px;
  width: 180px;
  background-color: var(--surface-elevated);
  border: 1px solid var(--border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  padding: 12px;
  z-index: 10001;
}

.launcher-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 0;
}

.launcher-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 8px 0;
  border-radius: var(--radius-sm);
  text-decoration: none;
  transition: all var(--transition-fast);
}

.launcher-item:hover {
  background-color: var(--color-gray-100);
}

:root.dark .launcher-item:hover {
  background-color: var(--border-subtle);
}

.launcher-icon {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.launcher-icon i {
  font-size: 20px;
  color: var(--text-primary);
}

/* Transitions */
.launcher-fade-enter-active,
.launcher-fade-leave-active {
  transition: all 0.2s ease;
}

.launcher-fade-enter-from,
.launcher-fade-leave-to {
  opacity: 0;
  transform: translateX(-10px);
}
</style>
