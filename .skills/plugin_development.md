# Plugin Development

## Description
Guidelines for creating and registering new plugins in the Abyss ecosystem. This includes backend registration and frontend component integration.

## Context
- Backend: `plugin.go`, `plugins/` directory.
- Frontend: `www/src/plugin/`, `plugins/*/www/` directories.

## Instructions
1. **Define the Plugin**: Each plugin must implement the `Plugin` interface found in `plugin.go`.
2. **Metadata**: Implement `Info() PluginInfo` to specify the slug, name, version, and dependencies.
3. **Registration**: Use `Register(p Plugin)` in the plugin's `init()` function or a registration hook.
4. **Lifecycle**:
   - `Init(ctx *StartupContext)`: Called when the plugin is enabled. Use `ctx` to access core resources (DB, Users, Logger).
   - `Stop(ctx context.Context)`: Called when the plugin is disabled or the system shuts down.
5. **Extensions**:
   - Implement `Router` to add HTTP routes.
   - Implement `Authenticator` to add login methods.
   - Implement `StorageProvider` to add storage engines.
   - Implement `UIProvider` to provide frontend pages and assets.
6. **Frontend**:
   - Assets should be provided via `UIAssets() fs.FS`.
   - Use `UIPages()` to define routes and navigation positions.
   - Frontend components are typically Vue 3 files that are compiled and served via the plugin's static assets.
