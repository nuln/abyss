# Abyss 仓库拆分与发布方案（plugins / pro / www）

## 目标

1. `plugins` 与 `pro` 分别作为独立私有仓库。
2. 这两个仓库中每个插件目录保持独立 Go module，支持按插件单独导入与独立版本。
3. `www` 作为独立私有仓库，通过 git submodule 挂到主仓库。
4. 主仓库、`plugins`、`pro`、`www` 都是私有仓库，方案需覆盖在公开仓库引用时的权限配置。

## 总体仓库设计

推荐拆分为 4 个仓库：

- `github.com/nuln/abyss-core`（核心后端，私有）
- `github.com/nuln/abyss-plugins`（免费插件集合，私有）
- `github.com/nuln/abyss-pro`（Pro 插件集合，私有）
- `github.com/nuln/abyss-www`（前端，私有）

主仓库中：

- `www` 使用 submodule 指向 `abyss-www`。
- `plugins` 与 `pro` 建议不再作为源码目录直接维护（可在开发态使用本地 replace 指向工作副本）。

## plugins / pro 的模块与版本策略

### 1) 每个插件目录是独立 module

`abyss-plugins` 仓库示例：

- `totp/go.mod` -> `module github.com/nuln/abyss-plugins/totp`
- `trash/go.mod` -> `module github.com/nuln/abyss-plugins/trash`
- `webdav/go.mod` -> `module github.com/nuln/abyss-plugins/webdav`

`abyss-pro` 仓库示例：

- `oidc/go.mod` -> `module github.com/nuln/abyss-pro/oidc`
- `passkey/go.mod` -> `module github.com/nuln/abyss-pro/passkey`
- 其他同理

这样做可以保证外部项目只导入单个插件 module，而不是整个仓库。

### 2) 单插件独立版本

在多 module 单仓库里，按目录前缀打 tag：

- `totp/v0.3.1`
- `trash/v1.2.0`
- `oidc/v0.1.0`

Go 会按 module path + 前缀 tag 解析对应版本，实现“单插件单版本”。

### 3) 发布建议

插件发布步骤：

1. 在插件目录内完成变更和测试。
2. 合并到 `main`。
3. 打目录前缀 tag，例如：`git tag oidc/v0.1.0`。
4. 推送 tag：`git push origin oidc/v0.1.0`。

## 主仓库对插件的依赖方式

生产依赖（远程）：

- 在主仓库 `go.mod` 中直接 `require` 需要的插件 module。
- 不需要的插件不引入。

开发依赖（本地联调）：

- 用 `replace` 指向本地路径（例如 `../abyss-pro/oidc`）。
- 开发结束后可移除或保留在示例工程中。

## www 作为 submodule

主仓库 `.gitmodules` 示例：

```ini
[submodule "www"]
  path = www
  url = git@github.com:nuln/abyss-www.git
```

初始化/更新：

```bash
git submodule update --init --recursive
```

更新前端版本（固定到某个 commit）：

```bash
cd www
git checkout <commit-or-tag>
cd ..
git add www .gitmodules
git commit -m "chore: bump www submodule"
```

## 私有仓库权限与公开仓库引用策略

这是关键点：公开仓库无法“匿名”拉取私有依赖，必须提供凭据。

### 1) 本地开发者机器

配置：

- `GOPRIVATE=github.com/nuln/*`
- 使用 SSH（推荐）或 PAT（细粒度 token）拉取私有仓库。

示例：

```bash
go env -w GOPRIVATE=github.com/nuln/*
git config --global url."ssh://git@github.com/".insteadOf "https://github.com/"
```

### 2) CI（GitHub Actions）

在工作流里注入只读凭据：

- 优先 GitHub App token（短期、可审计）
- 或 Fine-grained PAT（最小权限）

并设置：

- `GOPRIVATE=github.com/nuln/*`
- `GONOSUMDB=github.com/nuln/*`

### 3) 公开仓库引用当前私有核心仓库

可行路径：

1. 公开仓库仅提供“接口层”或“SDK 层”，不直接构建私有依赖。
2. 公开仓库在 CI 中显式配置私有凭据后再构建。
3. 如果要完全零凭据使用，必须把对应依赖改为公开仓库（或发布公开镜像）。

结论：若核心/插件保持私有，公开项目构建时一定要配置凭据。

## 仓库迁移与提交方式（建议执行顺序）

1. 创建新私有仓库：`abyss-plugins`、`abyss-pro`、`abyss-www`。
2. 将现有目录历史拆分到新仓库（推荐 `git filter-repo` 或 `git subtree split`）。
3. 在新仓库补齐根文档与 CI（README、.gitignore、workflows）。
4. 主仓库将 `www` 改为 submodule 指向新地址。
5. 主仓库将插件依赖改为远程 module（并减少本地源码耦合）。
6. 在 `example` 和 `example/pro` 中保留 `replace` 作为开发态样例。
7. 完成一次全链路 CI 验证（核心 + 插件 + 前端）。

## 当前仓库约束

- 核心仓库 (`abyss-core`) 的 `.gitignore` 直接忽略 `plugins/` 与 `pro/` 整个目录。
- `www/` 通过 git submodule 关联到 `abyss-www`，不在核心仓库提交前端构建产物。

## 已在当前仓库落地的基础文件

已补齐：

- `plugins/README.md`
- `plugins/.gitignore`
- `plugins/.github/workflows/validate.yml`
- `pro/README.md`
- `pro/.gitignore`
- `pro/.github/workflows/validate.yml`
- 根 `.gitignore` 已从“整目录忽略”改为“构建产物忽略”。

这些文件可直接作为未来独立仓库的初始模板。
