# Abyss 项目代码审计报告

> 审计范围：后端核心代码（`app.go`(原 abyss.go), `identity.go`(原 auth.go+user.go+util.go 加密部分), `api.go`(原 http.go), `boltdb.go`(原 db.go), `storage.go`, `plugin.go`, `task.go`, `config.go`(含 settings.go), `errors.go`）
> 
> 注：本报告为审计时的快照，文中残留的旧文件名与行号以括号内映射为准。
> 
> 审计基准文档：[project-context.md](./project-context.md)

---

## 一、Bug 发现（按严重程度排序）

### 🔴 BUG-1：`handleRevokeSession` 缺少用户归属校验（IDOR 越权漏洞）

**严重程度：Critical** | **文件：** [http.go](../api.go#L443-L455)

```go
func (a *App) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
    uid := AuthUserIDFromContext(r.Context())
    // ...
    id := mux.Vars(r)["id"]
    if err := a.authSvc.RevokeSession(r.Context(), id); err != nil { // ❌ 直接用 URL 里的 id 撤销
```

**问题**：任何已登录用户可以通过构造任意 `session ID` 撤销**其他用户**的会话（Insecure Direct Object Reference）。当前代码没有验证该 session 是否属于当前用户。

**修复方向**：撤销前需先查询该 session 的 `UserID`，确认与当前登录用户一致，或限制为仅 admin 可跨用户操作。

---

### 🔴 BUG-2：`handleUpdateUser` 非管理员可篡改任意用户的角色和权限

**严重程度：Critical** | **文件：** [http.go](../api.go#L540-L613)

```go
func (a *App) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
    if !AuthIsAdminFromContext(r.Context()) {
        id, _ := parseIDVar(r, "id")
        if id != AuthUserIDFromContext(r.Context()) {
            // 非管理员只能更新自己 → 但下面的逻辑没有限制可修改的字段
```

**问题**：当非管理员更新自己时，请求体中的 `User` 对象包含了 `Role`、`Permissions` 等字段，但代码没有过滤。虽然当前前端不发送这些字段，但恶意用户可以构造请求将自己提升为 Admin：

```json
{"data": {"role": "admin", "permissions": {"admin": true}}}
```

代码第 577-604 行只处理了 `DisplayName`、`Username`、`Email`、`Preferences`、`Password`，但 `User` struct 反序列化时 `Role` 和 `Permissions` 字段已经被填充到 `userData` 中。虽然当前没有直接赋值 `Role`/`Permissions`，但这是一个**极其脆弱的设计**——任何后续开发者添加 `u.Role = userData.Role` 都会直接导致权限提升。

**修复方向**：对非管理员用户，应明确禁止修改 `Role`、`Permissions` 字段。建议在非 admin 分支中使用白名单方式（仅允许 `DisplayName`、`Preferences`）。

---

### 🟠 BUG-3：`WriteErr` 中 `appErr` 为 nil 时可能 panic

**严重程度：High** | **文件：** [http.go](../api.go#L85-L115)

```go
func WriteErr(w http.ResponseWriter, err error) {
    var appErr *Error
    msg := err.Error()
    code := http.StatusInternalServerError

    if errors.As(err, &appErr) {
        msg = appErr.Message
        // ...
    }

    // ...
    if msg == "" {
        msg = appErr.Code  // ❌ 如果 errors.As 失败，appErr 仍为 nil → panic!
    }
```

**问题**：当 `err` 不是 `*Error` 类型且 `err.Error()` 返回空字符串时，`appErr` 为 nil，访问 `appErr.Code` 会触发 **nil pointer dereference panic**。虽然概率不高，但在插件返回非标准错误时可能被触发。

**修复方向**：将 L108-109 的 `appErr.Code` 替换为安全的 fallback，如 `http.StatusText(code)`。

---

### 🟠 BUG-4：`Shutdown` 提前返回导致数据库未关闭

**严重程度：High** | **文件：** [abyss.go](../app.go#L445-L465)

```go
func (a *App) Shutdown(ctx context.Context) error {
    // ...
    if a.Server != nil {
        if err := a.Server.Shutdown(shutdownCtx); err != nil {
            return err  // ❌ HTTP shutdown 失败就直接返回了
        }
    }
    if a.pluginMgr != nil {
        if err := a.pluginMgr.StopAll(shutdownCtx); err != nil {
            return err  // ❌ 插件 stop 失败也直接返回了
        }
    }
    if a.DB != nil {
        return a.DB.Close()  // 永远不会执行到
    }
```

**问题**：如果 `Server.Shutdown` 或 `pluginMgr.StopAll` 返回错误，数据库 `DB.Close()` 永远不会被调用，可能导致 **BoltDB 数据文件损坏**（未 flush 的脏页）。

**修复方向**：改用 `errors.Join` 收集所有错误，确保无论前面是否失败，DB 始终被关闭。

---

### 🟡 BUG-5：`handleFileUpload` 未限制请求体大小

**严重程度：Medium** | **文件：** [http.go](../api.go#L634-L667)

```go
file, err = a.storageSvc.WriteFile(r.Context(), uid, filePath, r.Body)
```

**问题**：`r.Body` 直接传入存储引擎写入，没有任何大小限制。恶意用户可以上传超大文件，耗尽磁盘空间。HTTP Server 的 `ReadHeaderTimeout` 不保护 body 读取。

**修复方向**：使用 `http.MaxBytesReader` 或在存储引擎层面实现配额检查。

---

### 🟡 BUG-6：`DecodeJSON` 未限制请求体大小

**严重程度：Medium** | **文件：** [http.go](../api.go#L118-L120)

```go
func DecodeJSON(r *http.Request, out any) error {
    return json.NewDecoder(r.Body).Decode(out)
}
```

**问题**：所有使用 `DecodeJSON` 和直接 `json.NewDecoder(r.Body)` 的地方（登录、注册、设置等至少 10 处），都没有限制 body 大小。攻击者可以发送超大 JSON 导致 OOM。

**修复方向**：使用 `io.LimitReader(r.Body, maxBodySize)` 包装，推荐 1MB 上限。

---

### 🟡 BUG-7：`ensureDemoUser` 函数在 `Bootstrap` 中未被使用

**严重程度：Medium（死代码）** | **文件：** [abyss.go](../app.go#L389-L425)

`ensureDemoUser` 是一个完整的独立函数，但 `Bootstrap` 中使用的是一段内联的 demo 用户创建逻辑（L134-L168）。两段逻辑存在行为不一致：

- `ensureDemoUser` 分配了更多权限（`Execute`, `Copy`, `Move`, `Shell`, `Upload`）
- `Bootstrap` 中的内联逻辑仅设置 `RoleAdmin` + `Permissions.Admin`

**修复方向**：统一使用 `ensureDemoUser` 或删除死代码。

---

## 二、优化建议

### ⚡ OPT-1：`signJWT` 中嵌入完整 User 对象导致 Token 膨胀

**文件：** [auth.go](../identity.go#L211-L225)

```go
claims := jwt.MapClaims{
    "user": u.ToFrontend(),  // ❌ 整个用户对象被塞入 JWT
```

每个 access token 包含了完整的用户 profile（权限、偏好设置等），导致 JWT 可能达到 **1-2KB**。这会：
- 增大每个 HTTP 请求的 header 大小
- 增加 cookie 存储开销
- 用户修改偏好后旧 token 中的数据会 stale

**建议**：JWT 中只保留 `uid`, `role`, `admin`，用户详情通过 `/api/me` 按需获取。

---

### ⚡ OPT-2：`settingsService.Get` 每次请求都查 DB

**文件：** [settings.go](../config.go#L67-L120)

`handleIndex`、`handleRegister`、`handleUpdateUser` 等热路径每次都调用 `settingsSvc.Get()` 去 BoltDB 读取，而 Settings 几乎不变。

**建议**：添加内存缓存 + Save 时失效机制，避免重复 DB 读取。

---

### ⚡ OPT-3：`storageService.engines` 缓存无上限

**文件：** [storage.go](../storage.go#L460-L468)

`engines map[uint64]StorageEngine` 和 `uuidCache map[uint64]string` 只增不减。如果系统长期运行且有大量用户，这两个 map 会无限增长。

**建议**：使用 LRU 缓存或在用户删除时清理对应条目。

---

### ⚡ OPT-4：`boltUserStore.Update` 中索引未做唯一性冲突检查

**文件：** [db.go](../boltdb.go#L242-L289)

修改 email/username 时，代码先删除旧索引再写入新索引，但**没有检查新 email/username 是否已被其他用户占用**。这可能导致两个用户拥有相同的 email 或 username，破坏索引唯一性约束。

**建议**：写入新索引前，检查目标 key 是否已存在（且指向不同的 PK）。

---

### ⚡ OPT-5：Task 系统的 `runCtx` 在 cancel 后仍被用于 DB 写入

**文件：** [task.go](../task.go#L148-L171)

```go
runCtx, cancel := context.WithCancel(ctx)
// ...
t.Status = TaskCanceled
_ = s.store.Save(runCtx, t)  // ❌ runCtx 已被 cancel，Save 内部检查 ctx.Err() 会直接返回
```

当任务被取消后，`runCtx` 已经 done，最终状态的 `Save` 可能失败（`boltTaskStore.Save` 第一行就检查了 `ctx.Err()`），导致任务状态永远卡在 `running`。

**建议**：最终状态写入使用 `context.Background()` 而非已取消的 `runCtx`。

---

### ⚡ OPT-6：`handleIndex` 每次请求都解析 HTML 模板

**文件：** [http.go](../api.go#L1041-L1111)

每个 SPA 页面请求（非 API、非静态资源）都会：
1. `fs.ReadFile` 读取 `index.html`
2. `strings.ReplaceAll` 做 5 次字符串替换
3. `template.New().Parse()` 解析模板

**建议**：在 `Bootstrap` 阶段预编译模板，缓存结果。仅在 dev 模式下每次重新解析。

---

### ⚡ OPT-7：`aesEncrypt/aesDecrypt` 使用 JWT Secret 作为 AES 密钥

**文件：** [util.go](../identity.go#L107-L134)

JWT Secret（32 bytes）直接作为 AES-256 密钥使用。如果 JWT Secret 泄露，所有加密数据（如插件中的敏感配置）都会被解密。更好的做法是使用 KDF 派生独立的加密密钥。

**建议**：使用 `HKDF` 从 JWT Secret 派生独立的加密密钥。

---

### ⚡ OPT-8：前后端契约偏差 — `GET /api/setup/status` 未实现

**来源**：[project-context.md](./project-context.md#L627-L631)

`docs/project-context.md` 已经识别到前端存在 `GET /api/setup/status` 调用，但后端未实现。

**建议**：确认前端是否仍在调用此端点。如果是，补充实现；如果是历史残留，清理前端代码。

---

## 三、问题汇总表

| ID | 类型 | 严重程度 | 文件 | 简述 |
|---|---|---|---|---|
| BUG-1 | 🐛 Bug | 🔴 Critical | `http.go:443` | Session 撤销无归属验证（IDOR） |
| BUG-2 | 🐛 Bug | 🔴 Critical | `http.go:540` | 非 admin 可篡改自身角色/权限 |
| BUG-3 | 🐛 Bug | 🟠 High | `http.go:108` | `WriteErr` 可能 nil panic |
| BUG-4 | 🐛 Bug | 🟠 High | `abyss.go:445` | Shutdown 失败跳过 DB 关闭 |
| BUG-5 | 🐛 Bug | 🟡 Medium | `http.go:634` | 文件上传无大小限制 |
| BUG-6 | 🐛 Bug | 🟡 Medium | `http.go:118` | JSON 解码无 body 大小限制 |
| BUG-7 | 🐛 Bug | 🟡 Medium | `abyss.go:389` | 死代码 `ensureDemoUser` |
| OPT-1 | ⚡ 优化 | 🟡 Medium | `auth.go:217` | JWT Token 膨胀 |
| OPT-2 | ⚡ 优化 | 🟡 Medium | `settings.go:67` | Settings 每请求查 DB |
| OPT-3 | ⚡ 优化 | 🟢 Low | `storage.go:464` | Engine 缓存无上限 |
| OPT-4 | ⚡ 优化 | 🟠 High | `db.go:261` | 用户更新索引无唯一性检查 |
| OPT-5 | ⚡ 优化 | 🟠 High | `task.go:169` | 取消后的 ctx 用于最终状态写入 |
| OPT-6 | ⚡ 优化 | 🟢 Low | `http.go:1041` | Index 模板每请求重新解析 |
| OPT-7 | ⚡ 优化 | 🟢 Low | `util.go:107` | AES 密钥未独立派生 |
| OPT-8 | ⚡ 优化 | 🟢 Low | 前后端契约 | `/api/setup/status` 未实现 |

---

## 四、AI 修复提示词

> 将以下 Prompt 复制给其他 AI 编码助手（如 Cursor、Cline、Gemini），它即可开始修复工作。

````markdown
# Role & Goal

你是一位 Go 安全工程师和代码质量专家。请根据以下审计结果，对 Abyss 项目进行精确修复。

## 关键约束
- 请先阅读项目根目录下的 `docs/project-context.md`，了解完整架构。
- **修改后必须保持向前兼容**：不改变 API 响应结构、bucket 名称、索引 key 格式。
- 每个修复完成后运行 `go test ./...` 确保所有测试通过。
- 优先修复 Critical 和 High 级别问题。

---

## 修复任务清单

### 🔴 TASK-1: 修复 `handleRevokeSession` IDOR 漏洞
**文件**: `http.go`，函数 `handleRevokeSession`（约第 443 行）

**当前问题**: 当前代码直接使用 URL path 参数 `id` 调用 `authSvc.RevokeSession`，任何已登录用户可以撤销任意用户的会话。

**修复方案**:
1. 在调用 `RevokeSession` 前，先通过 `sessionStore` 获取该 session 记录
2. 验证 session 的 `UserID` 是否等于当前登录用户的 `uid`
3. 如果不匹配且当前用户不是 admin，返回 403 Forbidden
4. 你可能需要给 `SessionStore` 接口添加一个 `GetByID(ctx, id) (*RefreshToken, error)` 方法，并在 `boltSessionStore` 中实现（从 `identity_sessions` bucket 中按 ID 查找）

**参考代码结构**:
```go
func (a *App) handleRevokeSession(w http.ResponseWriter, r *http.Request) {
    uid := AuthUserIDFromContext(r.Context())
    if uid == 0 { ... }
    id := mux.Vars(r)["id"]
    
    // 新增：归属验证
    session, err := a.sessionStore.GetByID(r.Context(), id)  // 需要新增此方法
    if err != nil { WriteErr(w, err); return }
    if session.UserID != uid && !AuthIsAdminFromContext(r.Context()) {
        WriteJSON(w, http.StatusForbidden, ErrorResponse("cannot revoke another user's session"))
        return
    }
    
    if err := a.authSvc.RevokeSession(r.Context(), id); err != nil { ... }
}
```

---

### 🔴 TASK-2: 加固 `handleUpdateUser` 非管理员字段白名单
**文件**: `http.go`，函数 `handleUpdateUser`（约第 540 行）

**当前问题**: 非管理员用户通过 `PUT /api/users/{自己的id}` 可以在请求体中注入 `role`、`permissions` 等字段。虽然当前代码没有直接赋值这些字段，但这是一个极其脆弱的设计。

**修复方案**:
1. 将当前的 `handleUpdateUser` 拆分为两个逻辑分支
2. **非管理员分支**：仅允许修改 `DisplayName`、`Preferences`、`Password`，其他字段一律忽略
3. **管理员分支**：可以修改所有字段，包括 `Role`、`Permissions`、`Email`、`Username`
4. 管理员更新其他用户时，如果修改了 `Role` 或 `Permissions`，也应正确赋值

**注意**: 管理员更新时，如果前端发送了 `Role`/`Permissions`，需要正确地将其写入数据库。当前代码漏了 `Role` 和 `Permissions` 的赋值。

---

### 🟠 TASK-3: 修复 `WriteErr` 的 nil panic
**文件**: `http.go`，函数 `WriteErr`（约第 85 行）

将第 108-110 行修改为：
```go
if msg == "" {
    if appErr != nil {
        msg = appErr.Code
    } else {
        msg = http.StatusText(code)
    }
}
```

---

### 🟠 TASK-4: 修复 `Shutdown` 确保 DB 始终关闭
**文件**: `abyss.go`，函数 `Shutdown`（约第 445 行）

改为收集所有错误，始终执行 DB.Close()：
```go
func (a *App) Shutdown(ctx context.Context) error {
    if a == nil { return nil }
    shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    
    var errs []error
    if a.Server != nil {
        if err := a.Server.Shutdown(shutdownCtx); err != nil {
            errs = append(errs, fmt.Errorf("http shutdown: %w", err))
        }
    }
    if a.pluginMgr != nil {
        if err := a.pluginMgr.StopAll(shutdownCtx); err != nil {
            errs = append(errs, fmt.Errorf("plugin stop: %w", err))
        }
    }
    if a.DB != nil {
        if err := a.DB.Close(); err != nil {
            errs = append(errs, fmt.Errorf("db close: %w", err))
        }
    }
    return errors.Join(errs...)
}
```
需要在文件顶部确保 `import "errors"` 已引入。

---

### 🟠 TASK-5: 修复 Task 取消后状态写入失败
**文件**: `task.go`，函数 `Submit`（约第 135 行）

将第 169 行的 `runCtx` 改为 `context.Background()`：
```go
// 任务完成/取消/失败后的最终状态写入，使用独立的 context 防止已取消的 ctx 阻塞写入
_ = s.store.Save(context.Background(), t)
s.broadcast(t)
```

---

### 🟠 TASK-6: 修复 `boltUserStore.Update` 索引唯一性检查
**文件**: `db.go`，函数 `Update`（约第 242 行）

在写入新 email/username 索引前，检查是否已被其他用户占用：
```go
if newEmail != oldEmail {
    // 检查新 email 是否已被其他用户占用
    if existingPK := emailIdx.Get([]byte(newEmail)); existingPK != nil {
        existingID := binary.BigEndian.Uint64(existingPK)
        if existingID != user.ID {
            return ErrConflict
        }
    }
    _ = emailIdx.Delete([]byte(oldEmail))
    if err := emailIdx.Put([]byte(newEmail), pk); err != nil {
        return err
    }
}
// 对 username 做同样的检查
```

---

### 🟡 TASK-7: 为 JSON 解码添加 body 大小限制
**文件**: `http.go`，函数 `DecodeJSON`（约第 118 行）

```go
const maxJSONBodySize = 1 << 20 // 1MB

func DecodeJSON(r *http.Request, out any) error {
    r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBodySize)
    return json.NewDecoder(r.Body).Decode(out)
}
```

⚠️ **注意**: `handleFileUpload` 中不要用 `DecodeJSON`，它的 body 是文件流。文件上传的大小限制应在配置或存储层面处理。

---

### 🟡 TASK-8: 清理死代码 `ensureDemoUser`
**文件**: `abyss.go`（约第 389-425 行）

由于 `Bootstrap` 中已有内联的 demo 用户创建逻辑（L134-168），`ensureDemoUser` 函数未被使用。两个选择：
1. **推荐**: 删除 `ensureDemoUser` 函数
2. 或者重构 Bootstrap 中的内联逻辑，使其调用 `ensureDemoUser`

---

## 验证步骤

完成所有修复后，请执行以下验证：

```bash
# 1. 编译检查
go build ./...

# 2. 全量单元测试
go test -v ./...

# 3. 代码格式化
gofmt -w .

# 4. Lint 检查
make lint
```

## 修复优先级建议

1. **第一批（安全）**: TASK-1 → TASK-2 → TASK-3 → TASK-7
2. **第二批（稳定性）**: TASK-4 → TASK-5 → TASK-6
3. **第三批（清理）**: TASK-8
````

---

## 五、未覆盖的审计区域（建议后续关注）

由于 API 配额限制，以下区域未深入审计，建议后续补充：

1. **前端代码**（`www/src/`）：XSS、Token 存储、EventSource 泄漏、插件加载器安全性
2. **插件实现**（`plugins/webdav`, `plugins/totp`, `plugins/trash`）：WebDAV token 安全、TOTP 实现正确性
3. **Pro 插件**（`pro/`）：需单独审计
4. **并发压测**：BoltDB 在高并发写入下的锁竞争表现
5. **`plugin.go` 完整审计**（1727 行）：仅审计了前 800 行的接口定义和注册逻辑
