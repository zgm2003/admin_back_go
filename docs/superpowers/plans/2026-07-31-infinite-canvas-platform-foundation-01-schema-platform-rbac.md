# 无限画布 Schema 与平台 RBAC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 用可验证的 guarded migrations 建立平台级 RBAC、Canvas 项目/COS/提示词数据结构和确定性初始数据，同时保留 Admin 数据并把 `users.role_id` 降为不可授权的迁移字段。

**Architecture:** 先扩展角色、权限和用户平台绑定，再创建产品表并扩展既有 `ai_assets/ai_prompts`，最后导入平台、权限、默认角色、六个提示词来源和禁用状态的 cron row。所有 DDL 在 persistent change 前执行 information_schema 与数据 preflight；canonical HCL、forward migration 和 Atlas checksum 同提交。

**Tech Stack:** MySQL 8.4、Atlas HCL、PowerShell Atlas wrapper、Go architecture tests。

---

## 执行边界

> **并行与提交覆盖规则：** 实施时同时遵守 `E:\admin\LONG_TASK_PARALLEL_EXECUTION.md` 和 execution index。子执行器只修改分配给自己的文件并返回 diff/测试证据，不运行 `git add`、`git commit`、merge 或 rebase；下文所有“提交”步骤均为主线程审查后的集成检查点。Plan 01 的 migration、canonical HCL 和 `atlas.sum` 是单一写入 lane，其他并发槽位只能做只读审查。

- 所有路径相对 `E:\admin\admin_back_go`。
- 开始前必须证明 reviewed backend capability baseline `d028e17ffd2a66d08b898f905e44cb93cb262bcf` 是当前 HEAD 的祖先；若此后 schema、migration、Contract、COS 或 runtime 又变化，先按 execution index 审查并更新基线。`202607310101..103` 只能追加在当前 `database/migrations` 尾部，并基于最新 `database/schema/admin.hcl`/`atlas.sum` 生成，禁止恢复旧 checksum 或覆盖 AI provider/COS 相关 schema。
- 本 Plan 在 Wave 0 独占 `database/schema/admin.hcl` 和 `database/migrations/atlas.sum`；Plan 07 只可追加已批准的 `202607310104` 数据激活 migration 并重算 checksum，不得改 canonical DDL。
- migrations 只 expand/backfill，不物理删除 `users.role_id`、历史 `ai_assets/ai_prompts` 列或任何数据表。
- 运行 migration 前停止会写 users/roles/permissions/assets/prompts 的 API 和 Worker；自动测试不连接真实业务库。
- 平台字段统一为 `VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin`；值固定 `admin` 或 `infinite_canvas`。
- `users.id/roles.id/permissions.id` 的组合外键列使用 `INT UNSIGNED`；项目、素材、来源和 intent 主键使用 `BIGINT UNSIGNED`。

## 文件结构

**Create:**

- `database/migrations/202607310101_infinite_canvas_platform_rbac.sql`：RBAC expand、Admin binding backfill 和组合约束。
- `database/migrations/202607310102_infinite_canvas_product_data.sql`：项目、素材引用、upload intent、prompt source 及 AI 表扩展。
- `database/migrations/202607310103_infinite_canvas_seed_data.sql`：平台注册数据、权限、默认角色、来源和 disabled cron rows。
- `internal/architecture/infinite_canvas_schema_test.go`：migration/HCL/seed 的静态一致性门禁。

**Modify:**

- `database/schema/admin.hcl`：上述 migrations 执行后的唯一目标 schema。
- `database/migrations/atlas.sum`：由 Atlas hash 命令生成。
- `database/seeds/admin_permissions.sql`：记录新的 Admin prompt 管理权限和 Canvas 产品权限，保持本地初始化与 migration 一致。
- `database/seeds/README.md`：说明 Canvas 默认角色只获得同平台六项权限，Admin 新权限不自动授权。

### Task 1: 先固定 schema 和 migration 不变量

**Files:**
- Create: `internal/architecture/infinite_canvas_schema_test.go`
- Test: `internal/architecture/infinite_canvas_schema_test.go`

- [ ] **Step 1: 写失败的架构测试**

测试读取三个精确 migration 文件和 `database/schema/admin.hcl`，至少包含以下断言：

```go
func TestInfiniteCanvasPlatformSchema(t *testing.T) {
    schema := readArchitectureText(t, "database/schema/admin.hcl")
    rbac := readArchitectureText(t, "database/migrations/202607310101_infinite_canvas_platform_rbac.sql")
    product := readArchitectureText(t, "database/migrations/202607310102_infinite_canvas_product_data.sql")
    seed := readArchitectureText(t, "database/migrations/202607310103_infinite_canvas_seed_data.sql")

    for _, table := range []string{
        "user_platform_roles", "canvas_projects", "canvas_project_assets",
        "asset_upload_intents", "ai_prompt_sources",
    } {
        if !strings.Contains(schema, `table "`+table+`"`) {
            t.Errorf("canonical schema missing %s", table)
        }
    }
    for _, required := range []string{
        "CREATE TEMPORARY TABLE `_infinite_canvas_rbac_guard`",
        "ADD COLUMN `platform` VARCHAR(32)",
        "INSERT INTO `user_platform_roles`",
        "UNIQUE KEY `uk_roles_active_default_platform`",
        "FOREIGN KEY (`platform`, `role_id`)",
        "FOREIGN KEY (`platform`, `permission_id`)",
        "MODIFY COLUMN `role_id` INT UNSIGNED NULL DEFAULT NULL",
    } {
        if !strings.Contains(rbac, required) {
            t.Errorf("RBAC migration missing %q", required)
        }
    }
    for _, required := range []string{
        "CREATE TABLE `canvas_projects`", "CREATE TABLE `canvas_project_assets`",
        "CREATE TABLE `asset_upload_intents`", "CREATE TABLE `ai_prompt_sources`",
        "ADD COLUMN `origin_type` VARCHAR(16)", "ADD COLUMN `object_key` VARCHAR(191)",
    } {
        if !strings.Contains(product, required) {
            t.Errorf("product migration missing %q", required)
        }
    }
    for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN `role_id`", "TRUNCATE TABLE"} {
        if strings.Contains(strings.ToUpper(rbac+product+seed), forbidden) {
            t.Errorf("migration contains forbidden destructive operation %q", forbidden)
        }
    }
}
```

再加入独立子测试，验证六个 source code、六个 raw URL、六个 homepage、两个 cron 名称、Canvas 六项 permission code 只出现一次；验证两条 cron seed 的 `status=2`，且不存在给现有 Admin role 自动授权 prompt 权限的 SQL。

- [ ] **Step 2: 运行测试并确认因文件不存在失败**

Run: `go test ./internal/architecture -run TestInfiniteCanvasPlatformSchema -count=1`

Expected: FAIL，错误明确指出 `202607310101_infinite_canvas_platform_rbac.sql` 不存在；不得因测试编译错误失败。

- [ ] **Step 3: 提交测试骨架**

```bash
git add internal/architecture/infinite_canvas_schema_test.go
git commit -m "test(canvas): 固定平台数据结构约束"
```

### Task 2: 建立 platform role 和 principal 真相源

**Files:**
- Create: `database/migrations/202607310101_infinite_canvas_platform_rbac.sql`
- Modify: `database/schema/admin.hcl`
- Test: `internal/architecture/infinite_canvas_schema_test.go`

- [ ] **Step 1: 在 migration 最前面加入无副作用 preflight**

guard 必须逐项证明：目标表/列/索引尚不存在；所有现有 role、permission、role_permission 可回填为 Admin；每个 user 的非空 `role_id` 指向有效 role；不存在重复 `(role_id, permission_id)`；当前仅有一个有效全局默认 role。

```sql
DROP TEMPORARY TABLE IF EXISTS `_infinite_canvas_rbac_guard`;
CREATE TEMPORARY TABLE `_infinite_canvas_rbac_guard` (
  `violations` BIGINT NOT NULL,
  CHECK (`violations` = 0)
);

INSERT INTO `_infinite_canvas_rbac_guard`
SELECT IF(COUNT(*) = 4, 0, 1)
FROM `information_schema`.`TABLES`
WHERE `TABLE_SCHEMA` = DATABASE()
  AND `TABLE_NAME` IN ('users', 'roles', 'permissions', 'role_permissions');

INSERT INTO `_infinite_canvas_rbac_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `information_schema`.`TABLES`
WHERE `TABLE_SCHEMA` = DATABASE() AND `TABLE_NAME` = 'user_platform_roles';

INSERT INTO `_infinite_canvas_rbac_guard`
SELECT IF(COUNT(*) = 0, 0, 1)
FROM `users` AS u
LEFT JOIN `roles` AS r ON r.`id` = u.`role_id` AND r.`is_del` = 2
WHERE u.`role_id` IS NOT NULL AND r.`id` IS NULL;

INSERT INTO `_infinite_canvas_rbac_guard`
SELECT IF(COUNT(*) <= 1, 0, 1)
FROM `roles`
WHERE `is_default` = 1 AND `is_del` = 2;
```

补齐 information_schema 检查：`roles.platform`、`role_permissions.platform`、`roles.active_default_platform` 不能预先存在；当前 `permissions.platform` 的非空值必须全部满足已批准平台格式且旧数据必须为 `admin`。

- [ ] **Step 2: 扩展 roles/permissions/role_permissions 并建立数据库约束**

migration 的 DDL 顺序固定如下：

```sql
ALTER TABLE `permissions`
  MODIFY COLUMN `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'admin',
  ADD UNIQUE KEY `uk_permissions_platform_id` (`platform`, `id`);

ALTER TABLE `roles`
  ADD COLUMN `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'admin' AFTER `id`,
  ADD COLUMN `active_default_platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin
    GENERATED ALWAYS AS (CASE WHEN `is_default` = 1 AND `is_del` = 2 THEN `platform` ELSE NULL END) STORED,
  DROP INDEX `uk_roles_name`,
  ADD UNIQUE KEY `uk_roles_platform_name` (`platform`, `name`),
  ADD UNIQUE KEY `uk_roles_platform_id` (`platform`, `id`),
  ADD UNIQUE KEY `uk_roles_active_default_platform` (`active_default_platform`),
  ADD CONSTRAINT `chk_roles_platform` CHECK (`platform` IN ('admin', 'infinite_canvas'));

ALTER TABLE `role_permissions`
  ADD COLUMN `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'admin' AFTER `id`,
  DROP INDEX `uniq_role_permission`,
  ADD UNIQUE KEY `uk_role_permissions_platform_role_permission` (`platform`, `role_id`, `permission_id`),
  ADD KEY `idx_role_permissions_platform_permission` (`platform`, `permission_id`, `is_del`, `role_id`),
  ADD CONSTRAINT `fk_role_permissions_platform_role`
    FOREIGN KEY (`platform`, `role_id`) REFERENCES `roles` (`platform`, `id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `fk_role_permissions_platform_permission`
    FOREIGN KEY (`platform`, `permission_id`) REFERENCES `permissions` (`platform`, `id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `chk_role_permissions_platform` CHECK (`platform` IN ('admin', 'infinite_canvas'));
```

在添加 `role_permissions` foreign key 前显式把所有旧行更新为 `platform='admin'`，并以 join guard 证明 role 和 permission 也为 Admin。

- [ ] **Step 3: 创建 user_platform_roles 并回填 Admin membership**

```sql
CREATE TABLE `user_platform_roles` (
  `user_id` INT UNSIGNED NOT NULL,
  `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `role_id` INT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`user_id`, `platform`),
  KEY `idx_user_platform_roles_platform_role_user` (`platform`, `role_id`, `user_id`),
  CONSTRAINT `fk_user_platform_roles_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_user_platform_roles_platform_role` FOREIGN KEY (`platform`, `role_id`) REFERENCES `roles` (`platform`, `id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_user_platform_roles_platform` CHECK (`platform` IN ('admin', 'infinite_canvas'))
);

INSERT INTO `user_platform_roles` (`user_id`, `platform`, `role_id`, `created_at`, `updated_at`)
SELECT `id`, 'admin', `role_id`, CURRENT_TIMESTAMP(6), CURRENT_TIMESTAMP(6)
FROM `users`
WHERE `role_id` IS NOT NULL;

ALTER TABLE `users`
  MODIFY COLUMN `role_id` INT UNSIGNED NULL DEFAULT NULL;
```

回填后用 guard 比较 `users.role_id IS NOT NULL` 和 Admin binding 的数量及 role_id，一项不一致即失败；不得清空旧列。

- [ ] **Step 4: 在 HCL 中逐列镜像 RBAC 目标**

`roles`、`permissions`、`role_permissions` 和 `users` 的 HCL 必须与上面 DDL 完全一致；新增表使用以下关键 HCL 约束：

```hcl
table "user_platform_roles" {
  schema = schema.admin
  column "user_id" { type = int unsigned = true null = false }
  column "platform" { type = varchar(32) charset = "ascii" collate = "ascii_bin" null = false }
  column "role_id" { type = int unsigned = true null = false }
  primary_key { columns = [column.user_id, column.platform] }
  index "idx_user_platform_roles_platform_role_user" {
    columns = [column.platform, column.role_id, column.user_id]
  }
}
```

同时写全两个 datetime(6)、两个 foreign key 和 platform check，不能只依赖 migration 文本测试。

- [ ] **Step 5: 运行 RBAC schema 测试**

Run: `go test ./internal/architecture -run TestInfiniteCanvasPlatformSchema -count=1`

Expected: 仍 FAIL，但 RBAC 子测试 PASS；失败只来自产品 migration/seed 尚未创建。

- [ ] **Step 6: 提交 RBAC schema**

```bash
git add database/migrations/202607310101_infinite_canvas_platform_rbac.sql database/schema/admin.hcl internal/architecture/infinite_canvas_schema_test.go
git commit -m "feat(rbac): 建立平台角色绑定结构"
```

### Task 3: 创建项目、素材上传和提示词来源结构

**Files:**
- Create: `database/migrations/202607310102_infinite_canvas_product_data.sql`
- Modify: `database/schema/admin.hcl`
- Test: `internal/architecture/infinite_canvas_schema_test.go`

- [ ] **Step 1: 为产品 migration 写完整 preflight**

创建 `_infinite_canvas_product_guard`，验证上一个 migration 的 `user_platform_roles` 已存在、四张产品目标表 `canvas_projects/canvas_project_assets/asset_upload_intents/ai_prompt_sources` 尚不存在、`ai_assets/ai_prompts` 已存在、所有新增列和索引名不存在、历史 asset/prompt 主键唯一。DDL 前还要证明所有既有 `ai_assets.user_id` 可保留、所有 prompt slug 非空且唯一，以便回填 `origin_type='manual'`。

- [ ] **Step 2: 创建 canvas_projects 和引用表**

```sql
CREATE TABLE `canvas_projects` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `user_id` INT UNSIGNED NOT NULL,
  `request_id` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `request_fingerprint` BINARY(32) NOT NULL,
  `title` VARCHAR(120) NOT NULL,
  `schema_version` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `document_json` JSON NOT NULL,
  `revision` BIGINT UNSIGNED NOT NULL DEFAULT 1,
  `is_del` TINYINT UNSIGNED NOT NULL DEFAULT 2,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_canvas_projects_platform_user_request` (`platform`, `user_id`, `request_id`),
  KEY `idx_canvas_projects_owner_updated` (`platform`, `user_id`, `is_del`, `updated_at`, `id`),
  CONSTRAINT `fk_canvas_projects_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_canvas_projects_platform` CHECK (`platform` = 'infinite_canvas'),
  CONSTRAINT `chk_canvas_projects_revision` CHECK (`revision` >= 1),
  CONSTRAINT `chk_canvas_projects_is_del` CHECK (`is_del` IN (1, 2))
);

CREATE TABLE `canvas_project_assets` (
  `project_id` BIGINT UNSIGNED NOT NULL,
  `asset_id` BIGINT UNSIGNED NOT NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`project_id`, `asset_id`),
  KEY `idx_canvas_project_assets_asset_project` (`asset_id`, `project_id`),
  CONSTRAINT `fk_canvas_project_assets_project` FOREIGN KEY (`project_id`) REFERENCES `canvas_projects` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `fk_canvas_project_assets_asset` FOREIGN KEY (`asset_id`) REFERENCES `ai_assets` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT
);
```

- [ ] **Step 3: 扩展 ai_assets 并创建 upload intents**

`ai_assets` 新增以下列，历史行回填 `platform='admin'`；`object_key` 使用受限长度的 ASCII binary-collated key，`sha256` 为 `BINARY(32) NULL`，两者只有图片必填，文本行必须为 NULL。现有 `slug/cover_url/url/content` 暂不删除，runtime 不再把 URL 当图片真相。

```sql
ALTER TABLE `ai_assets`
  ADD COLUMN `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'admin' AFTER `id`,
  ADD COLUMN `storage_provider` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT '' AFTER `content`,
  ADD COLUMN `object_key` VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER `storage_provider`,
  ADD COLUMN `sha256` BINARY(32) NULL AFTER `object_key`,
  ADD COLUMN `mime_type` VARCHAR(128) NOT NULL DEFAULT '' AFTER `sha256`,
  ADD COLUMN `size_bytes` BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER `mime_type`,
  ADD COLUMN `width` INT UNSIGNED NOT NULL DEFAULT 0 AFTER `size_bytes`,
  ADD COLUMN `height` INT UNSIGNED NOT NULL DEFAULT 0 AFTER `width`,
  DROP INDEX `uk_ai_assets_user_slug`,
  ADD UNIQUE KEY `uk_ai_assets_platform_user_slug` (`platform`, `user_id`, `slug`),
  ADD KEY `idx_ai_assets_owner_type_updated` (`platform`, `user_id`, `type`, `is_del`, `updated_at`, `id`),
  ADD UNIQUE KEY `uk_ai_assets_platform_object_key` (`platform`, `object_key`);
```

完整 intent 表：

```sql
CREATE TABLE `asset_upload_intents` (
  `id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
  `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `user_id` INT UNSIGNED NOT NULL,
  `object_key` VARCHAR(191) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
  `original_filename` VARCHAR(255) NOT NULL,
  `declared_mime_type` VARCHAR(128) NOT NULL,
  `declared_size_bytes` BIGINT UNSIGNED NOT NULL,
  `declared_sha256` BINARY(32) NOT NULL,
  `status` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'pending',
  `expires_at` DATETIME(6) NOT NULL,
  `consumed_at` DATETIME(6) NULL,
  `created_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  `updated_at` DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_asset_upload_intents_object_key` (`object_key`),
  KEY `idx_asset_upload_intents_cleanup` (`status`, `expires_at`, `id`),
  KEY `idx_asset_upload_intents_owner` (`platform`, `user_id`, `id`),
  CONSTRAINT `fk_asset_upload_intents_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  CONSTRAINT `chk_asset_upload_intents_platform` CHECK (`platform` = 'infinite_canvas'),
  CONSTRAINT `chk_asset_upload_intents_status` CHECK (`status` IN ('pending', 'consumed', 'expired')),
  CONSTRAINT `chk_asset_upload_intents_size` CHECK (`declared_size_bytes` BETWEEN 1 AND 20971520),
  CONSTRAINT `chk_asset_upload_intents_consumed` CHECK ((`status` = 'consumed' AND `consumed_at` IS NOT NULL) OR (`status` <> 'consumed' AND `consumed_at` IS NULL))
);
```

- [ ] **Step 4: 创建 prompt source 并扩展 ai_prompts**

`ai_prompt_sources` 精确列为 `id/platform/code/name/feed_url/homepage_url/status/last_attempt_at/last_success_at/last_error_summary/etag/last_modified/is_del/created_at/updated_at`。`code` 使用 ASCII 64、URL 2048、error 512、etag/last_modified 255；唯一键 `(platform,code)`，列表索引 `(platform,status,is_del,id)`，platform check 固定 Infinite Canvas。

`ai_prompts` 扩展并回填：

```sql
ALTER TABLE `ai_prompts`
  ADD COLUMN `platform` VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'admin' AFTER `id`,
  ADD COLUMN `origin_type` VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL DEFAULT 'manual' AFTER `platform`,
  ADD COLUMN `source_id` BIGINT UNSIGNED NULL AFTER `origin_type`,
  ADD COLUMN `external_id` VARCHAR(191) NOT NULL DEFAULT '' AFTER `source_id`,
  ADD COLUMN `manual_slug_active` VARCHAR(191)
    GENERATED ALWAYS AS (CASE WHEN `origin_type` = 'manual' THEN `slug` ELSE NULL END) STORED AFTER `slug`,
  ADD COLUMN `description` TEXT NULL AFTER `title`,
  ADD COLUMN `reference_urls_json` JSON NULL AFTER `cover_url`,
  DROP INDEX `uk_ai_prompts_slug`,
  ADD UNIQUE KEY `uk_ai_prompts_platform_manual_slug` (`platform`, `manual_slug_active`),
  ADD UNIQUE KEY `uk_ai_prompts_source_external` (`source_id`, `external_id`),
  ADD KEY `idx_ai_prompts_canvas_public` (`platform`, `status`, `is_del`, `source_id`, `updated_at`, `id`),
  ADD CONSTRAINT `fk_ai_prompts_source` FOREIGN KEY (`source_id`) REFERENCES `ai_prompt_sources` (`id`) ON UPDATE RESTRICT ON DELETE RESTRICT,
  ADD CONSTRAINT `chk_ai_prompts_origin` CHECK ((`origin_type` = 'manual' AND `source_id` IS NULL AND `external_id` = '') OR (`origin_type` = 'source' AND `source_id` IS NOT NULL AND `external_id` <> ''));
```

来源提示词的 slug 也必须非空且可能跨来源重复，因此 `manual_slug_active` 仅在 `origin_type='manual'` 时返回 slug；MySQL 唯一键允许多个 NULL，恰好只约束手工提示词的 `(platform, slug)`。HCL 和架构测试必须表达这一精确行为，不得再增加直接的 `(platform, slug)` 唯一键。

- [ ] **Step 5: 在 HCL 中完整镜像五张表和两个扩展表**

每个 column 的 null/default/unsigned/charset/collation、所有 unique/index/check/FK 名称必须与 migration 一致。对 JSON 列使用 Atlas `type = json`，对 SHA 使用 `type = binary(32)`；不得把 SHA 存成可变字符串。

- [ ] **Step 6: 运行产品 schema 子测试**

Run: `go test ./internal/architecture -run TestInfiniteCanvasPlatformSchema -count=1`

Expected: schema/RBAC/product 子测试 PASS，seed 子测试因 `202607310103` 不存在而 FAIL。

- [ ] **Step 7: 提交产品数据结构**

```bash
git add database/migrations/202607310102_infinite_canvas_product_data.sql database/schema/admin.hcl internal/architecture/infinite_canvas_schema_test.go
git commit -m "feat(canvas): 建立项目素材与提示词结构"
```

### Task 4: 导入可信平台注册项和确定性初始数据

**Files:**
- Create: `database/migrations/202607310103_infinite_canvas_seed_data.sql`
- Modify: `database/seeds/admin_permissions.sql`
- Modify: `database/seeds/README.md`
- Test: `internal/architecture/infinite_canvas_schema_test.go`

- [ ] **Step 1: 插入但不覆盖 auth platform 运维配置**

```sql
INSERT INTO `auth_platforms` (
  `code`, `name`, `login_types`, `captcha_type`, `access_ttl`, `refresh_ttl`,
  `bind_platform`, `bind_device`, `bind_ip`, `max_sessions`, `allow_register`, `status`, `is_del`
)
SELECT 'infinite_canvas', '无限画布', JSON_ARRAY('email', 'password'), 'slide',
       14400, 1209600, 1, 2, 2, 5, 1, 1, 2
WHERE NOT EXISTS (SELECT 1 FROM `auth_platforms` WHERE `code` = 'infinite_canvas');
```

若 code 已存在，不 UPDATE TTL、设备/IP、max session、register 或 status；seed 后 guard 只验证 code 唯一且未使用退役值 `canvas`。

- [ ] **Step 2: 导入 Canvas 权限和默认角色**

以 code 查 ID，不依赖自增值。六项权限固定为：

```text
infinite_canvas_workspace
infinite_canvas_project_read
infinite_canvas_project_write
infinite_canvas_asset_read
infinite_canvas_asset_write
infinite_canvas_prompt_read
```

权限全部 `platform='infinite_canvas'`、`show_menu=2`、`status=1`、`is_del=2`。创建/恢复角色 `无限画布用户`，`platform='infinite_canvas'`、`is_default=1`，再向 `role_permissions` 只写上述同平台六项。SQL 必须通过 `INSERT ... SELECT` 和 `ON DUPLICATE KEY UPDATE is_del=2` 恢复软删除记录，不能按固定 ID 写入。

- [ ] **Step 3: 导入 Admin prompt 权限但不静默授权**

创建以下 Admin codes，并挂到现有 AI 管理目录；不得向任何 Admin `role_permissions` 插入它们：

```text
ai_prompt_list ai_prompt_detail ai_prompt_create ai_prompt_update ai_prompt_delete ai_prompt_status
ai_prompt_source_list ai_prompt_source_detail ai_prompt_source_create ai_prompt_source_update
ai_prompt_source_delete ai_prompt_source_status ai_prompt_sync
```

`database/seeds/admin_permissions.sql` 使用相同 code/name/platform/type/parent 关系；README 明确由管理员在角色页手工授权。

- [ ] **Step 4: 逐字迁移六个 prompt source**

```sql
INSERT INTO `ai_prompt_sources` (`platform`, `code`, `name`, `feed_url`, `homepage_url`, `status`, `is_del`) VALUES
('infinite_canvas','banana-prompt-quicker','Banana Prompt Quicker','https://raw.githubusercontent.com/yukkcat/image-prompts/main/dist/sources/banana-prompt-quicker.json','https://glidea.github.io/banana-prompt-quicker/',1,2),
('infinite_canvas','davidwu-gpt-image2-prompts','DavidWu GPT Image 2','https://raw.githubusercontent.com/yukkcat/image-prompts/main/dist/sources/davidwu-gpt-image2-prompts.json','https://github.com/davidwuw0811-boop/awesome-gpt-image2-prompts',1,2),
('infinite_canvas','awesome-gpt-image','Awesome GPT Image','https://raw.githubusercontent.com/yukkcat/image-prompts/main/dist/sources/awesome-gpt-image.json','https://github.com/ZeroLu/awesome-gpt-image',1,2),
('infinite_canvas','awesome-gpt4o-image-prompts','Awesome GPT-4o','https://raw.githubusercontent.com/yukkcat/image-prompts/main/dist/sources/awesome-gpt4o-image-prompts.json','https://github.com/ImgEdify/Awesome-GPT4o-Image-Prompts',1,2),
('infinite_canvas','youmind-gpt-image-2','YouMind GPT Image 2','https://raw.githubusercontent.com/yukkcat/image-prompts/main/dist/sources/youmind-gpt-image-2.json','https://github.com/YouMind-OpenLab/awesome-gpt-image-2',1,2),
('infinite_canvas','youmind-nano-banana-pro','YouMind Nano Banana Pro','https://raw.githubusercontent.com/yukkcat/image-prompts/main/dist/sources/youmind-nano-banana-pro.json','https://github.com/YouMind-OpenLab/awesome-nano-banana-pro-prompts',1,2)
ON DUPLICATE KEY UPDATE `name`=VALUES(`name`), `feed_url`=VALUES(`feed_url`), `homepage_url`=VALUES(`homepage_url`);
```

初始导入允许更新这六个产品预置来源的名称和 URL；后续版本 migration 不再覆盖运维修改。

- [ ] **Step 5: 以 disabled 状态创建 cron rows**

按现有 `cron_task` schema 插入：

```text
name                                      cron expression  handler                                           status
infinite_canvas_prompt_sync               0 */6 * * *      infinite-canvas:prompt-sync-dispatch:v1            2
infinite_canvas_asset_upload_cleanup      15 * * * *       infinite-canvas:asset-upload-cleanup:v1             2
```

description 明确“同步无限画布提示词来源”和“清理无限画布过期上传意图”；`handler` 保存 registry entry 的 `TaskType`，`name` 保存 registry lookup key。`cron_task` 没有 payload 列，payload 由对应 `BuildTask` 固定构造。Plan 07 部署 handler 后才启用。

- [ ] **Step 6: 运行完整 schema 静态测试**

Run: `go test ./internal/architecture -run TestInfiniteCanvasPlatformSchema -count=1`

Expected: PASS；六个 source URL/homepage、权限、默认 role、disabled cron 和无 destructive DDL 全部通过。

- [ ] **Step 7: 提交 seed**

```bash
git add database/migrations/202607310103_infinite_canvas_seed_data.sql database/seeds/admin_permissions.sql database/seeds/README.md internal/architecture/infinite_canvas_schema_test.go
git commit -m "feat(canvas): 导入平台权限与提示词来源"
```

### Task 5: 生成 Atlas checksum 并验证 canonical schema

**Files:**
- Modify: `database/migrations/atlas.sum`
- Verify: `database/schema/admin.hcl`
- Verify: `database/migrations/202607310101_infinite_canvas_platform_rbac.sql`
- Verify: `database/migrations/202607310102_infinite_canvas_product_data.sql`
- Verify: `database/migrations/202607310103_infinite_canvas_seed_data.sql`

- [ ] **Step 1: 格式化和校验 HCL**

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 schema fmt --dir file://database/schema`

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 schema inspect --env local --url file://database/schema/admin.hcl`

Expected: 两条命令退出 0；inspect 输出包含五张新增表和 `roles.platform`。

- [ ] **Step 2: 更新 migration checksum**

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate hash --dir file://database/migrations`

Expected: exit 0，`atlas.sum` 只增加/更新三个 `20260731` migration hash，旧 migration hash 不变。

- [ ] **Step 3: 验证 migration 目录**

Run: `pwsh -NoProfile -File scripts/database/atlas.ps1 migrate validate --dir file://database/migrations`

Expected: `Migration directory is valid` 或等价成功输出，exit 0。

- [ ] **Step 4: 运行定向测试和 diff 门禁**

```powershell
go test ./internal/architecture -run 'TestInfiniteCanvasPlatformSchema|TestReconciliationSchema' -count=1
git diff --check
git status --short
```

Expected: 测试和 diff 均退出 0；status 只显示本 Task 的 `atlas.sum`/HCL 格式化变化，没有 Contract 或业务代码。

- [ ] **Step 5: 提交 canonical schema 完成态**

```bash
git add database/schema/admin.hcl database/migrations/atlas.sum
git commit -m "chore(database): 锁定无限画布 schema 校验和"
```

## 完成标准

- 三个 guarded migrations、canonical HCL、seed 和 Atlas checksum 一致。
- Admin 用户全部有等价 `user_platform_roles(platform='admin')` 回填，`users.role_id` 可空但未删除。
- 数据库可证明角色名按平台唯一、默认角色按平台唯一、role/permission 不能跨平台绑定。
- Canvas 六项权限只授给 `无限画布用户`；Admin prompt 权限未自动授予任何角色。
- 六个提示词来源 URL 与原项目逐字一致；两条 cron row 初始 disabled。
- 本 Plan 没有激活 HTTP route、Worker handler 或浏览器功能。
