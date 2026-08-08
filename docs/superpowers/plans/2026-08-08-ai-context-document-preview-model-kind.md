# AI 模型用途治理与上下文文档预览 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. 本计划按当前约束在同一工作区串行执行，不分派子代理。

**Goal:** 用 `chat | embedding | rerank | image` 修正官方模型、供应商模型、Agent 场景和 Context Profile 的数据所有权，并为上下文文档增加不依赖 Qdrant 的 COS 短链预览。

**Architecture:** `officialmodel` 是模型用途的唯一枚举与受审事实来源；`ai_provider_models` 保存供应商实际路由和 Embedding 规格；Agent 只消费 Chat/Image，Context Profile 只消费 Embedding/Rerank/Memory，并把 Embedding 规格复制成不可变索引快照。文档预览按版本 ID 读取 MySQL 授权事实，经 COS 条件 HEAD 后签发 300 秒 GET URL，Qdrant 不参与查看。

**Tech Stack:** Go 1.26.5、Gin、GORM/MySQL 8、Atlas 0.38、Tencent COS、Vue 3 `<script setup>`、TypeScript 5.9、Element Plus、AppTable、Vitest、generated Admin OpenAPI Contract。

---

## Delivery Boundaries

```text
officialmodel.ModelKind
  -> Provider Model 路由和 Embedding 规格
  -> Agent Scene/Kind 或 Context Profile/Kind 校验

MySQL 文档版本事实 + COS 原始对象
  -> 条件 HEAD 校验 ETag/大小/MIME
  -> 300 秒签名 GET URL
  -> Vue 右侧预览抽屉
```

- 不新增 Embedding Docker、模型探测请求、`unknown` 数据库类型、能力 JSON、多对多用途表或 Office 转换服务。
- 不按名称、前缀、`owned_by`、family 或输入模态猜用途；只接受 canonical ID 或受审 alias 的大小写敏感精确匹配。
- 不重写旧 Context Profile、空间、文档、Chunk 或 Qdrant Collection；更换 Embedding 模型仍通过新 Profile 和新索引代次完成。
- 不改变普通聊天、当前附件、WebSocket、消息持久化、Run/Attempt 终态和计费主链。
- 不运行 `admin-dev`、Compose、Playwright、真实迁移、`go test ./...`、前端全量测试或完整构建。
- 只运行任务中列出的短定向测试、契约生成和静态差异检查；最终业务验收由用户执行。

## File Responsibility Map

### Backend

- `internal/module/ai/officialmodel/catalog.go`: 唯一 `ModelKind`、官方 Embedding 规格和目录一致性校验。
- `internal/module/ai/officialmodel/catalog/official_models_v1.json`: 23 个文本模型标记 `chat`，`gpt-image-2` 标记 `image`。
- `internal/module/ai/officialmodel/dto.go`: Admin 官方模型 DTO 暴露用途。
- `internal/module/ai/officialmodel/management.go`: 官方用途投影。
- `internal/module/ai/provider/model.go`: Provider Model 领域结构和用途 alias。
- `internal/module/ai/provider/dto.go`: Provider Model 输入、输出和候选 DTO。
- `internal/module/ai/provider/service.go`: Provider Model 校验、写入和同步服务。
- `internal/module/ai/provider/repository.go`: Provider Model 查询、协调和引用保护。
- `internal/module/ai/provider/transport/admin/request.go`: 保留旧 Chat 输入，新增完整 `models[]` 输入。
- `database/migrations/202608080101_ai_provider_model_kind_expand.sql`: 扩展列和 `image` CHECK。
- `database/migrations/202608080102_ai_provider_model_kind_backfill.sql`: 守卫后原地迁移 `gpt-image-2` 并回填 Embedding 规格。
- `database/migrations/202608080103_ai_provider_model_kind_contract.sql`: 收紧 Embedding 规格 CHECK。
- `database/schema/admin.hcl`: 最终数据库事实。
- `internal/module/ai/agent/dto.go`: Agent/Provider Model kind 输出。
- `internal/module/ai/agent/model.go`: Agent 持久化结构。
- `internal/module/ai/agent/service.go`: Scene/Kind 闭合矩阵。
- `internal/module/ai/agent/repository.go`: Agent 选择器和稳定主键连接。
- `internal/module/ai/image/repository.go`: 图片链只连接 `image` 路由。
- `internal/module/ai/contextengine/admin_dto.go`: Profile 规格快照和预览 DTO。
- `internal/module/ai/contextengine/admin_service.go`: Profile 自动复制规格和文档版本预览。
- `internal/module/ai/contextengine/repository.go`: Profile/文档授权查询。
- `internal/module/ai/contextengine/transport/admin/route.go`: Preview Admin API 路由。
- `internal/module/ai/contextengine/transport/admin/handler.go`: Preview 参数和错误映射。
- `internal/module/ai/contextengine/transport/admin/request.go`: Preview/Profile 请求兼容。
- `internal/infra/storage/conditional_object.go`: 条件对象预览契约。
- `internal/infra/storage/cos/conditional_object_reader.go`: 上下文对象条件 HEAD 和签名 URL。
- `internal/infra/storage/cos/object_inspector.go`: 上下文对象前缀校验。
- `internal/platform/admin/build.go`: 把同一个 COS adapter 同时注入读取与预览能力。
- `internal/admincontract/openapi_models_test.go`: Model kind/Profile schema contract。
- `internal/admincontract/openapi_test.go`: Preview operation contract。
- `internal/admincontract/permissions_test.go`: Preview permission/audit contract。
- `contracts/admin/v1/openapi.json`: Admin OpenAPI bundle。
- `contracts/admin/v1/permissions.json`: Admin permission bundle。
- `contracts/admin/v1/views.json`: Admin view bundle。
- `contracts/admin/v1/manifest.json`: Bundle manifest and source SHA。
- `contracts/admin/v1/realtime/envelope.schema.json`: Realtime envelope bundle。
- `contracts/admin/v1/realtime/events.schema.json`: Realtime event bundle。

### Frontend

- `src/api/ai/official-models.ts`: 官方用途和 Embedding 规格 API 类型。
- `src/api/ai/providers.ts`: 完整 Provider Model 输入和候选 API 类型。
- `src/api/ai/agents.ts`: Scene/Kind Agent API 类型。
- `src/api/ai/context.ts`: Profile 快照和 Preview Operation。
- `src/views/Main/ai/providers/composables/useProviderForm.ts`: 每个模型行作为唯一数据源，不再使用按 model ID 索引的三个 Map。
- `src/views/Main/ai/providers/components/ProviderModelEditor.vue`: 候选确认、Embedding 规格和用途控件。
- `src/views/Main/ai/providers/components/ProviderModelList.vue`: Provider Model 表格。
- `src/views/Main/ai/providers/components/ProviderFormDialog.vue`: Provider 表单组合。
- `src/views/Main/ai/agents/use-agent-admin-page.ts`: Scene/Kind 联动状态。
- `src/views/Main/ai/agents/index.vue`: 图片场景与 Chat 场景互斥的 Agent 表单。
- `src/views/Main/ai/context/components/ContextProfileDialog.vue`: 选择 Embedding 路由并只读显示规格快照来源。
- `src/views/Main/ai/context/components/ContextDocumentPreviewDrawer.vue`: 单一职责的右侧预览抽屉。
- `src/views/Main/ai/context/components/ContextDocumentPanel.vue`: 只持有选中版本并打开抽屉。
- `src/i18n/locales/zh-CN/ai.ts`: 中文 Provider/Agent 文案。
- `src/i18n/locales/zh-CN/ai-extended.ts`: 中文 Context/Preview 文案。
- `src/i18n/locales/en-US/ai.ts`: English Provider/Agent 文案。
- `src/i18n/locales/en-US/ai-extended.ts`: English Context/Preview 文案。
- `tests/component/ai/ProviderModelEditor.test.ts`: Provider editor behavior。
- `tests/component/ai/AgentOfficialModelForm.test.ts`: Agent scene/model behavior。
- `tests/component/ai/ContextProfileDialog.test.ts`: Profile snapshot behavior。
- `tests/component/ai/ContextDocumentPreviewDrawer.test.ts`: Preview drawer behavior。
- `tests/component/ai/ContextAdminTables.test.ts`: Document version selection behavior。
- `tests/shared/ai/ai-provider-api-protocol.test.ts`: Provider request protocol。

## Compatibility And Commit Order

```text
Task 1 officialmodel
  -> Task 2 schema expand/backfill/contract
  -> Tasks 3-7 backend behavior
  -> record backend source SHA
  -> Task 8 backend contract bundle
  -> frontend sync that exact source SHA
  -> Tasks 9-10 frontend behavior
```

- Provider Model 响应只增字段，不删字段。
- 旧 `model_ids + model_display_names + statuses` 继续表示 Chat；当前 `models[{model_id, model_kind}] + 两个 Map` 的混合请求也兼容一个周期。新管理端只提交完整 `models[]`；完整行与旧 Map 混用、或 `model_ids` 与 `models` 混用时返回 400。
- 旧 Profile 请求的三项 Embedding 规格在一个兼容周期内可提交，但必须与 Provider Model 完全一致；新前端不再提交。
- `gpt-image-2` 迁移原地更新 `model_kind`，保留 Provider Model 主键和 Agent 的 `provider_model_id`。
- 每个任务分别提交；不得把后端与前端放进同一个 Git 提交。

### Task 1: 让官方目录唯一拥有 `ModelKind`

**Files:**
- Modify: `internal/module/ai/officialmodel/catalog.go`
- Modify: `internal/module/ai/officialmodel/catalog_test.go`
- Modify: `internal/module/ai/officialmodel/capability_test.go`
- Modify: `internal/module/ai/officialmodel/dto.go`
- Modify: `internal/module/ai/officialmodel/management.go`
- Modify: `internal/module/ai/officialmodel/catalog/official_models_v1.json`
- Modify: `internal/module/ai/provider/model.go`

- [ ] **Step 1: 写目录用途和唯一所有权的失败测试**

在 `catalog_test.go` 中给 `validCatalogModel` 明确 `ModelKindChat`，并增加以下用例：

```go
func TestOfficialCatalogRequiresKindConsistentWithOutput(t *testing.T) {
	chat := validCatalogModel("chat-model")
	chat.ModelKind = ModelKindImage
	if _, err := NewCatalog("test-v1", []Model{chat}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("image kind with text output error = %v", err)
	}

	image := validCatalogModel("image-model")
	image.ModelKind = ModelKindImage
	image.Capabilities.OutputModalities = []string{ModalityImage}
	image.Capabilities.SupportsStreaming = false
	image.Capabilities.SupportsTools = false
	image.Capabilities.SupportedParameters = nil
	if _, err := NewCatalog("test-v1", []Model{image}); err != nil {
		t.Fatal(err)
	}
}

func TestOfficialCatalogDefaultHasReviewedModelKinds(t *testing.T) {
	for _, model := range Default.Models() {
		want := ModelKindChat
		if model.ModelID == "gpt-image-2" {
			want = ModelKindImage
		}
		if model.ModelKind != want {
			t.Fatalf("%s kind=%q want=%q", model.ModelID, model.ModelKind, want)
		}
	}
}

func TestOfficialCatalogEmbeddingKindRequiresCompleteSpec(t *testing.T) {
	embedding := validCatalogModel("embedding-model")
	embedding.ModelKind = ModelKindEmbedding
	embedding.Capabilities.SupportsStreaming = false
	embedding.Capabilities.SupportsTools = false
	embedding.Capabilities.SupportsStructuredOutput = false
	embedding.Capabilities.SupportedParameters = nil
	if _, err := NewCatalog("test-v1", []Model{embedding}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("missing embedding spec error = %v", err)
	}

	embedding.EmbeddingSpec = &EmbeddingSpec{
		Dimensions: 1024, MaxInputTokens: 8192, TokenCounterID: "utf8_bytes_v1",
	}
	if _, err := NewCatalog("test-v1", []Model{embedding}); err != nil {
		t.Fatal(err)
	}

	chat := validCatalogModel("chat-with-embedding-spec")
	chat.EmbeddingSpec = embedding.EmbeddingSpec
	if _, err := NewCatalog("test-v1", []Model{chat}); !errors.Is(err, ErrInvalidCatalog) {
		t.Fatalf("chat embedding spec error = %v", err)
	}
}
```

同时断言 `OfficialModelDTO.ModelKind` 和可空 `EmbeddingSpec` 被序列化；不要根据 `ModelFamily` 或模型 ID 在 DTO 层计算用途。

- [ ] **Step 2: 运行定向测试并确认 RED**

```powershell
go test ./internal/module/ai/officialmodel -run 'ModelKind|ReviewedModelKinds|KindConsistent' -count=1
```

Expected: FAIL，因为 `officialmodel.ModelKind` 和目录 `model_kind` 尚不存在。

- [ ] **Step 3: 实现闭合枚举和目录校验**

在 `officialmodel` 定义唯一枚举；Provider 只做 alias：

```go
type ModelKind string

const (
	ModelKindChat      ModelKind = "chat"
	ModelKindEmbedding ModelKind = "embedding"
	ModelKindRerank    ModelKind = "rerank"
	ModelKindImage     ModelKind = "image"
)

func (kind ModelKind) Validate() error {
	switch kind {
	case ModelKindChat, ModelKindEmbedding, ModelKindRerank, ModelKindImage:
		return nil
	default:
		return fmt.Errorf("invalid model kind %q", kind)
	}
}

type EmbeddingSpec struct {
	Dimensions     uint32 `json:"dimensions"`
	MaxInputTokens int64  `json:"max_input_tokens"`
	TokenCounterID string `json:"token_counter_id"`
}
```

`Model` 和 `catalogModelJSON` 增加 `ModelKind ModelKind` 与 `EmbeddingSpec *EmbeddingSpec`；`cloneModel` 必须深拷贝规格指针。`validateModel` 在通用 capability 校验后执行：

```go
func validateModelKind(model Model) error {
	if err := model.ModelKind.Validate(); err != nil {
		return err
	}
	hasInput := func(value string) bool { return containsString(model.Capabilities.InputModalities, value) }
	hasOutput := func(value string) bool { return containsString(model.Capabilities.OutputModalities, value) }
	switch model.ModelKind {
	case ModelKindChat:
		if !hasInput(ModalityText) || !hasOutput(ModalityText) || hasOutput(ModalityImage) {
			return errors.New("chat kind requires text input and text output")
		}
	case ModelKindImage:
		if !hasInput(ModalityText) || !hasOutput(ModalityImage) || hasOutput(ModalityText) || model.Capabilities.SupportsTools {
			return errors.New("image kind requires text input and image-only output")
		}
	case ModelKindEmbedding:
		if !hasInput(ModalityText) || hasOutput(ModalityImage) || model.Capabilities.SupportsStreaming || model.Capabilities.SupportsTools || model.Capabilities.SupportsStructuredOutput {
			return errors.New("embedding kind has an invalid execution capability")
		}
	case ModelKindRerank:
		if !hasInput(ModalityText) || hasOutput(ModalityImage) || model.Capabilities.SupportsStreaming || model.Capabilities.SupportsTools || model.Capabilities.SupportsStructuredOutput {
			return errors.New("rerank kind has an invalid execution capability")
		}
	}
	if model.ModelKind != ModelKindEmbedding {
		if model.EmbeddingSpec != nil {
			return errors.New("only embedding kind may define an embedding spec")
		}
		return nil
	}
	if model.EmbeddingSpec == nil || model.EmbeddingSpec.Dimensions == 0 || model.EmbeddingSpec.MaxInputTokens <= 0 {
		return errors.New("embedding kind requires a complete embedding spec")
	}
	if _, err := infraai.ResolveTokenCounter(model.EmbeddingSpec.TokenCounterID); err != nil {
		return errors.New("embedding kind has an invalid token counter")
	}
	return nil
}
```

本次目录没有官方 Embedding/Rerank 项，不添加未经核对的条目或规格。给 23 个文本输出项写入 `"model_kind": "chat"`，只给 `gpt-image-2` 写入 `"model_kind": "image"`；当前 24 项的 `embedding_spec` 均不写。以后加入受审官方 Embedding 时，目录项必须同时带完整 `embedding_spec`，Provider 同步直接复制，不再改数据结构。

Provider 保持已有名字兼容：

```go
type ModelKind = officialmodel.ModelKind

const (
	ModelKindChat = officialmodel.ModelKindChat
	ModelKindEmbedding = officialmodel.ModelKindEmbedding
	ModelKindRerank = officialmodel.ModelKindRerank
	ModelKindImage = officialmodel.ModelKindImage
)
```

- [ ] **Step 4: 把用途投影到官方模型 Admin DTO**

```go
type OfficialModelDTO struct {
	CatalogVendor string    `json:"catalog_vendor"`
	ModelFamily   string    `json:"model_family"`
	ModelID       string    `json:"model_id"`
	ModelKind     ModelKind `json:"model_kind" validate:"oneof=chat embedding rerank image"`
	EmbeddingSpec *EmbeddingSpec `json:"embedding_spec"`
}
```

`cloneModel` 已经隔离规格指针，因此 `management.go` 直接赋值 `ModelKind: model.ModelKind, EmbeddingSpec: model.EmbeddingSpec`，不增加 fallback。DTO 的现有字段继续按当前字段名逐项投影，不删除或改名。

- [ ] **Step 5: 验证 GREEN 并提交**

```powershell
go test ./internal/module/ai/officialmodel ./internal/module/ai/provider -run 'ModelKind|Catalog|Mapping' -count=1
git diff --check
git add -- internal/module/ai/officialmodel internal/module/ai/provider/model.go
git commit -m "feat(ai): define authoritative model kinds"
```

Expected: 定向测试 PASS；Provider 中不存在第二份 `type ModelKind string`。

### Task 2: 扩展 Provider Model schema 并原地迁移历史数据

**Files:**
- Modify: `database/schema/admin.hcl`
- Create: `database/migrations/202608080101_ai_provider_model_kind_expand.sql`
- Create: `database/migrations/202608080102_ai_provider_model_kind_backfill.sql`
- Create: `database/migrations/202608080103_ai_provider_model_kind_contract.sql`
- Modify: `database/migrations/atlas.sum`
- Modify: `internal/architecture/ai_context_schema_contract_test.go`
- Modify: `internal/architecture/ai_provider_model_kind_contract_test.go`

- [ ] **Step 1: 写最终 schema 的失败契约测试**

测试必须检查以下最终事实，而不是只搜索列名：

```go
for _, required := range []string{
	`column "embedding_dimensions"`,
	`column "embedding_max_input_tokens"`,
	`column "embedding_token_counter_id"`,
	`_ascii'image'`,
	`check "chk_ai_provider_models_embedding_spec"`,
} {
	if !strings.Contains(providerModels, required) {
		t.Fatalf("ai_provider_models missing %q", required)
	}
}
```

并读取三个新迁移，断言 backfill 同时包含 `gpt-image-2` 原地 `UPDATE`、Agent scene 守卫、Profile 规格冲突守卫，且不包含 `DELETE FROM ai_provider_models`。

- [ ] **Step 2: 运行架构测试并确认 RED**

```powershell
go test ./internal/architecture -run 'ProviderModelKind|ContextSchema' -count=1
```

Expected: FAIL，缺少 `image`、Embedding 规格列和迁移文件。

- [ ] **Step 3: 写 expand migration**

`202608080101_ai_provider_model_kind_expand.sql` 只扩展结构：

```sql
ALTER TABLE `ai_provider_models`
  ADD COLUMN `embedding_dimensions` INT UNSIGNED NULL AFTER `mapped_at`,
  ADD COLUMN `embedding_max_input_tokens` BIGINT UNSIGNED NULL AFTER `embedding_dimensions`,
  ADD COLUMN `embedding_token_counter_id` VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NULL AFTER `embedding_max_input_tokens`,
  DROP CHECK `chk_ai_provider_models_model_kind`,
  ADD CONSTRAINT `chk_ai_provider_models_model_kind`
    CHECK (`model_kind` IN (_ascii'chat', _ascii'embedding', _ascii'rerank', _ascii'image'));
```

不在 expand 阶段增加规格 CHECK，因为历史 Embedding 行尚未回填。

- [ ] **Step 4: 写带守卫的 backfill migration**

使用仓库已有 `atlas:delimiter` + 临时过程模式。迁移必须在任何更新前依次拒绝：同供应商 `gpt-image-2` 身份冲突、引用该模型但场景不是唯一 `image_generate`、其他模型错误声明 `image_generate`、同一 Embedding 路由被 Profile 保存成多套规格。

核心守卫和更新形状：

```sql
-- atlas:delimiter $$
DROP PROCEDURE IF EXISTS `_ai_provider_model_kind_backfill`
$$
CREATE PROCEDURE `_ai_provider_model_kind_backfill`()
BEGIN
  DECLARE conflict_id BIGINT UNSIGNED DEFAULT NULL;
  DECLARE conflict_message VARCHAR(255);

  SELECT MIN(source_row.`id`) INTO conflict_id
    FROM `ai_provider_models` AS source_row
    JOIN `ai_provider_models` AS target_row
      ON target_row.`provider_id` = source_row.`provider_id`
     AND BINARY target_row.`model_id` = BINARY source_row.`model_id`
     AND target_row.`model_kind` = _ascii'image'
    WHERE source_row.`model_kind` = _ascii'chat'
      AND BINARY source_row.`model_id` = BINARY _utf8mb4'gpt-image-2';
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('gpt-image-2 provider model identity conflict: provider_model_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(agent.`id`) INTO conflict_id
    FROM `ai_agents` AS agent
    JOIN `ai_provider_models` AS model_row ON model_row.`id` = agent.`provider_model_id`
    WHERE BINARY model_row.`model_id` = BINARY _utf8mb4'gpt-image-2'
      AND (agent.`scenes_json` IS NULL
        OR JSON_TYPE(agent.`scenes_json`) <> 'ARRAY'
        OR JSON_LENGTH(agent.`scenes_json`) <> 1
        OR JSON_CONTAINS(agent.`scenes_json`, JSON_QUOTE('image_generate')) = 0);
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('gpt-image-2 agent scene conflict: agent_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(agent.`id`) INTO conflict_id
    FROM `ai_agents` AS agent
    JOIN `ai_provider_models` AS model_row ON model_row.`id` = agent.`provider_model_id`
    WHERE JSON_TYPE(agent.`scenes_json`) = 'ARRAY'
      AND JSON_CONTAINS(agent.`scenes_json`, JSON_QUOTE('image_generate')) = 1
      AND BINARY model_row.`model_id` <> BINARY _utf8mb4'gpt-image-2';
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('non-gpt-image-2 image scene conflict: agent_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(profile.`id`) INTO conflict_id
    FROM `ai_context_profiles` AS profile
    JOIN `ai_provider_models` AS model_row
      ON model_row.`id` = profile.`embedding_provider_model_id`
    WHERE model_row.`model_kind` <> _ascii'embedding';
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('profile references non-embedding model: profile_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  SET conflict_id = NULL;
  SELECT MIN(conflict.`embedding_provider_model_id`) INTO conflict_id
  FROM (
    SELECT `embedding_provider_model_id`
    FROM `ai_context_profiles`
    GROUP BY `embedding_provider_model_id`
    HAVING COUNT(DISTINCT CONCAT_WS(CHAR(0), `embedding_dimensions`, `embedding_max_input_tokens`, `embedding_token_counter_id`)) > 1
  ) AS conflict;
  IF conflict_id IS NOT NULL THEN
    SET conflict_message = CONCAT('conflicting profile snapshots: provider_model_id=', conflict_id);
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = conflict_message;
  END IF;

  UPDATE `ai_provider_models` AS model_row
  JOIN `ai_context_profiles` AS profile
    ON profile.`embedding_provider_model_id` = model_row.`id`
  SET model_row.`embedding_dimensions` = profile.`embedding_dimensions`,
      model_row.`embedding_max_input_tokens` = profile.`embedding_max_input_tokens`,
      model_row.`embedding_token_counter_id` = profile.`embedding_token_counter_id`
  WHERE model_row.`model_kind` = _ascii'embedding';

  UPDATE `ai_provider_models`
  SET `status` = 2
  WHERE `model_kind` = _ascii'embedding'
    AND (`embedding_dimensions` IS NULL OR `embedding_max_input_tokens` IS NULL OR `embedding_token_counter_id` IS NULL);

  UPDATE `ai_provider_models`
  SET `model_kind` = _ascii'image'
  WHERE `model_kind` = _ascii'chat'
    AND BINARY `model_id` = BINARY _utf8mb4'gpt-image-2';
END
$$
CALL `_ai_provider_model_kind_backfill`()
$$
DROP PROCEDURE `_ai_provider_model_kind_backfill`
$$
```

四个 Agent/Profile 守卫和 Provider 身份冲突守卫必须完整保留在同一过程、全部位于首个 `UPDATE` 之前。`gpt-image-2` 的 `scenes_json IS NULL`、非数组、长度不为 1 或不含唯一 `image_generate` 都算冲突；实现条件时显式写 `agent.scenes_json IS NULL`，不能依赖 SQL 的三值逻辑。发现脏数据即停止，不选赢家、不改场景。

- [ ] **Step 5: 写 contract migration 和最终 HCL**

`202608080103_ai_provider_model_kind_contract.sql` 先用独立过程拒绝任何不满足最终形状的行，再增加 CHECK：

```sql
-- atlas:delimiter $$
DROP PROCEDURE IF EXISTS `_ai_provider_model_kind_contract_guard`
$$
CREATE PROCEDURE `_ai_provider_model_kind_contract_guard`()
BEGIN
  IF EXISTS (
    SELECT 1
    FROM `ai_provider_models`
    WHERE (`model_kind` = _ascii'embedding' AND (
      (`embedding_dimensions` IS NULL AND `embedding_max_input_tokens` IS NULL
        AND `embedding_token_counter_id` IS NULL AND `status` <> 2)
      OR ((`embedding_dimensions` IS NULL OR `embedding_max_input_tokens` IS NULL
          OR `embedding_token_counter_id` IS NULL)
        AND NOT (`embedding_dimensions` IS NULL AND `embedding_max_input_tokens` IS NULL
          AND `embedding_token_counter_id` IS NULL))
      OR COALESCE(`embedding_dimensions` = 0, FALSE)
      OR COALESCE(`embedding_max_input_tokens` = 0, FALSE)
      OR COALESCE(CHAR_LENGTH(`embedding_token_counter_id`) = 0, FALSE)
    )) OR (`model_kind` <> _ascii'embedding' AND (
      `embedding_dimensions` IS NOT NULL OR `embedding_max_input_tokens` IS NOT NULL
      OR `embedding_token_counter_id` IS NOT NULL
    ))
  ) THEN
    SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'provider model embedding spec violates final contract';
  END IF;
END
$$
CALL `_ai_provider_model_kind_contract_guard`()
$$
DROP PROCEDURE `_ai_provider_model_kind_contract_guard`
$$

ALTER TABLE `ai_provider_models`
  ADD CONSTRAINT `chk_ai_provider_models_embedding_spec`
  CHECK (
    (`model_kind` = _ascii'embedding' AND (
      (`status` = 2 AND `embedding_dimensions` IS NULL
        AND `embedding_max_input_tokens` IS NULL AND `embedding_token_counter_id` IS NULL)
      OR (`embedding_dimensions` IS NOT NULL AND `embedding_dimensions` > 0
        AND `embedding_max_input_tokens` IS NOT NULL AND `embedding_max_input_tokens` > 0
        AND `embedding_token_counter_id` IS NOT NULL
        AND CHAR_LENGTH(`embedding_token_counter_id`) > 0)
    ))
    OR (`model_kind` <> _ascii'embedding'
      AND `embedding_dimensions` IS NULL
      AND `embedding_max_input_tokens` IS NULL
      AND `embedding_token_counter_id` IS NULL)
  )
$$
```

在 `admin.hcl` 精确镜像三列、四类 kind 和规格 CHECK。数据库只能检查非空形状；Counter 是否已注册由 Go 服务校验。

- [ ] **Step 6: 只重算 checksum，不应用迁移**

```powershell
atlas version
atlas migrate hash --dir file://database/migrations
```

Expected: 已安装 Atlas 为 `v0.38.0`，`atlas.sum` 只因三个新文件改变。若本机无精确 Atlas，不安装工具、不启动 Docker、不执行 `migrate apply`，在交付中明确留给用户运行仓库脚本。

- [ ] **Step 7: 验证静态契约并提交**

```powershell
go test ./internal/architecture -run 'ProviderModelKind|ContextSchema' -count=1
git diff --check
git add -- database/schema/admin.hcl database/migrations/202608080101_ai_provider_model_kind_expand.sql database/migrations/202608080102_ai_provider_model_kind_backfill.sql database/migrations/202608080103_ai_provider_model_kind_contract.sql database/migrations/atlas.sum internal/architecture/ai_context_schema_contract_test.go internal/architecture/ai_provider_model_kind_contract_test.go
git commit -m "feat(ai): migrate provider model kinds"
```

Expected: PASS；未连接 MySQL，未执行真实迁移。

### Task 3: 让 Provider Model 输入完整表达一条路由

**Files:**
- Modify: `internal/module/ai/provider/model.go`
- Modify: `internal/module/ai/provider/dto.go`
- Modify: `internal/module/ai/provider/repository.go`
- Modify: `internal/module/ai/provider/service.go`
- Modify: `internal/module/ai/provider/transport/admin/request.go`
- Modify: `internal/module/ai/provider/transport/admin/route.go`
- Modify: `internal/module/ai/provider/service_test.go`
- Modify: `internal/module/ai/provider/repository_gorm_test.go`
- Modify: `internal/module/ai/provider/transport/admin/handler_test.go`

- [ ] **Step 1: 写结构化输入、规格和引用保护的失败测试**

覆盖以下行为：

```text
models[] 内联 display_name/status/Embedding 三项规格
已有 models[] 行携带稳定 id；新增行不携带 id
同一 model_id 的 chat 与 embedding 可以同时提交
同一 (model_id, model_kind) 重复返回 400
重复 id、id 不属于当前 Provider 或 id 与另一行身份冲突返回 400/409
未确认用途返回 ai.provider.model_kind_confirmation_required
未映射且启用的 embedding 缺任一规格返回 ai.provider.embedding_spec_invalid
非 embedding 携带规格返回 ai.provider.embedding_spec_invalid
迁移生成的“禁用且三项全空”历史 embedding 只有携带原 id 且保持禁用时可原样回传
官方映射用途与人工用途冲突返回 ai.provider.model_kind_conflict
被 Agent/Profile 引用后修改 kind 或 Embedding 规格返回 ai.provider.model_in_use (409)
旧 model_ids 仍生成 Chat
当前 models[] + 两个 Map 的旧混合请求继续成功，且同 model ID 不同 kind 仍按旧能力拒绝
完整 models[] 与两个 Map 混用返回 400；model_ids 与 models[] 混用返回 400
```

新的输入断言使用完整行：

```go
input := ProviderModelInput{
	ModelID: "BAAI/bge-m3", ModelKind: ModelKindEmbedding,
	DisplayName: stringPointer("BGE M3"), Status: intPointer(enum.CommonYes),
	EmbeddingDimensions: uint32Pointer(1024),
	EmbeddingMaxInputTokens: int64Pointer(8192),
	EmbeddingTokenCounterID: stringPointer("utf8_bytes_v1"),
}

persisted := ProviderModelInput{
	ID: uint64Pointer(31), ModelID: "gpt-5.6", ModelKind: ModelKindChat,
	DisplayName: stringPointer("GPT 5.6"), Status: intPointer(enum.CommonYes),
}

func uint64Pointer(value uint64) *uint64 { return &value }
func uint32Pointer(value uint32) *uint32 { return &value }
func intPointer(value int) *int          { return &value }
func int64Pointer(value int64) *int64    { return &value }
func stringPointer(value string) *string { return &value }
```

- [ ] **Step 2: 运行 Provider 定向测试并确认 RED**

```powershell
go test ./internal/module/ai/provider -run 'Structured|EmbeddingSpec|ModelInUse|LegacyModelIDs|ModelKindConflict' -count=1
```

Expected: FAIL，因为新字段、引用查询和稳定错误尚不存在。

- [ ] **Step 3: 扩展领域模型与 DTO，不保留新形状的 Map**

```go
type ProviderModel struct {
	ID                      uint64                      `gorm:"column:id;primaryKey"`
	ProviderID              uint64                      `gorm:"column:provider_id"`
	ModelID                 string                      `gorm:"column:model_id"`
	ModelKind               ModelKind                   `gorm:"column:model_kind"`
	DisplayName             string                      `gorm:"column:display_name"`
	OfficialModelID         *string                     `gorm:"column:official_model_id"`
	OfficialCatalogVersion  *string                     `gorm:"column:official_catalog_version"`
	MappingStatus           officialmodel.MappingStatus `gorm:"column:mapping_status"`
	MappedAt                *time.Time                  `gorm:"column:mapped_at"`
	EmbeddingDimensions     *uint32                     `gorm:"column:embedding_dimensions"`
	EmbeddingMaxInputTokens *int64                      `gorm:"column:embedding_max_input_tokens"`
	EmbeddingTokenCounterID *string                     `gorm:"column:embedding_token_counter_id"`
	Status                  int                         `gorm:"column:status"`
	CreatedAt               time.Time                   `gorm:"column:created_at"`
	UpdatedAt               time.Time                   `gorm:"column:updated_at"`
}

type ProviderModelInput struct {
	ID                      *uint64   `json:"id,omitempty" binding:"omitempty,gt=0"`
	ModelID                 string    `json:"model_id" binding:"required,max=191"`
	ModelKind               ModelKind `json:"model_kind"`
	DisplayName             *string   `json:"display_name,omitempty" binding:"omitempty,max=191"`
	Status                  *int      `json:"status,omitempty" binding:"omitempty,oneof=1 2"`
	EmbeddingDimensions     *uint32   `json:"embedding_dimensions,omitempty"`
	EmbeddingMaxInputTokens *int64    `json:"embedding_max_input_tokens,omitempty"`
	EmbeddingTokenCounterID *string   `json:"embedding_token_counter_id,omitempty" binding:"omitempty,max=64"`
}
```

`ProviderModelDTO` 返回稳定 `id`、同样三个可空规格、`mapping_status` 和用途。`Status` 使用指针只是为了在传输层区分“旧混合行未提交状态”和“新完整行提交状态”；进入领域校验后立即变成非空 `ProviderModel.Status`。`ID=nil` 表示新增或旧混合调用者，`ID!=nil` 表示编辑指定持久化行。

请求形状在任何模型校验和写事务之前闭合分类：

```text
model_ids 非空 + models 非空                                      -> 400
model_ids 非空                                                    -> 旧 Chat 分支，读取两个 Map
models 非空 + 两个 Map 非空 + 行内 id/display_name/status/spec 全部缺省 -> 旧混合分支，读取两个 Map，并按 model_id 去重
models 非空 + 两个 Map 非空 + 任一行内 id/display_name/status/spec 存在  -> 400
models 非空 + 两个 Map 为空 + 每行 status 存在                     -> 新完整行分支，只读行内字段
models 非空 + 两个 Map 为空 + 任一行 status 缺省                    -> 400
```

`route.go` 的三个写 operation 通过同一只读规则切片发布上述互斥关系，避免 Contract 隐藏兼容语义：

```go
var providerModelMutationRules = []string{
	"model_ids and models are mutually exclusive",
	"legacy models plus model_display_names/statuses must omit inline id, display_name, status, and embedding fields",
	"complete models must include inline status and must not include model_display_names or statuses",
}
```

`mutationRequest` 和 `updateModelsRequest` 对应的 `adminroute.HTTPContract.ParameterRules` 都复制这三条规则。

- [ ] **Step 4: 集中校验模型形状**

```go
func normalizeProviderModelInput(input ProviderModelInput) (ProviderModel, *apperror.Error) {
	model := ProviderModel{
		ModelID: strings.TrimSpace(input.ModelID), ModelKind: input.ModelKind,
		EmbeddingDimensions: cloneUint32(input.EmbeddingDimensions),
		EmbeddingMaxInputTokens: cloneInt64(input.EmbeddingMaxInputTokens),
		EmbeddingTokenCounterID: trimOptionalString(input.EmbeddingTokenCounterID),
	}
	if input.DisplayName != nil {
		model.DisplayName = strings.TrimSpace(*input.DisplayName)
	}
	if input.ID != nil {
		model.ID = *input.ID
	}
	if input.Status == nil {
		return ProviderModel{}, apperror.BadRequest("AI模型状态不能为空")
	}
	model.Status = *input.Status
	if model.ModelKind == "" {
		return ProviderModel{}, providerValidationError("ai.provider.model_kind_confirmation_required", "请先确认AI模型用途")
	}
	if model.ModelID == "" {
		return ProviderModel{}, apperror.BadRequest("AI模型ID不能为空")
	}
	if model.ModelKind.Validate() != nil {
		return ProviderModel{}, providerValidationError("ai.provider.model_kind_invalid", "AI模型用途无效")
	}
	if !enum.IsCommonStatus(model.Status) {
		return ProviderModel{}, apperror.BadRequest("AI模型状态无效")
	}
	if err := validateEmbeddingSpec(model); err != nil {
		return ProviderModel{}, providerValidationError("ai.provider.embedding_spec_invalid", "向量模型规格不完整或无效")
	}
	return model, nil
}
```

`validateEmbeddingSpec` 允许三种闭合形状：非 Embedding 三项全 nil；Embedding 三项完整、正数且 Counter 可由 `infraai.ResolveTokenCounter` 解析；或 `ID>0 + status=2 + Embedding 三项全 nil` 的历史兼容候选。第三种不能仅凭请求放行，仓储锁行后必须证明该 ID 原本就是同一 Provider 下同 kind、同样三项全 nil 的禁用行；否则返回 `ai.provider.embedding_spec_invalid`。因此可以改历史行显示名，但不能新建缺规格行、不能启用它、不能把已有完整规格清空。

- [ ] **Step 5: 在仓储事务中保护被引用的不可变事实**

增加一个闭合引用结果，不增加通用依赖图：

```go
type ProviderModelReferences struct {
	Agent            bool
	EmbeddingProfile bool
	RerankerProfile  bool
	MemoryProfile    bool
}

func (value ProviderModelReferences) Any() bool {
	return value.Agent || value.EmbeddingProfile || value.RerankerProfile || value.MemoryProfile
}
```

`ReconcileModels` 锁定当前 Provider 的全部模型后先建立 `byID` 与 `byIdentity[(model_id, model_kind)]`。输入带 `id` 时必须命中当前 Provider 的唯一行，并以该行作为修改对象；输入不带 `id` 时按复合身份匹配以兼容现有结构化调用者，未命中才新增。这样“同 model ID 新增另一用途”和“修改已有行用途”不再靠猜。重复 `id`、跨 Provider `id`、一个输入命中另一行的复合身份都在写库前失败。

若指定已有 `id` 的请求改变 `model_id`/`model_kind`，或改变 Embedding 三项规格，先查询引用。被引用时返回 sentinel `ErrProviderModelInUse` 并回滚；未引用时允许更新且继续保留同一主键。迁移留下的禁用空规格 Embedding 只允许保持空规格和禁用状态，填写完整规格后才可启用。允许修改显示名和状态，但不得因请求遗漏而禁用被引用行。所有新增/更新先完成唯一性和引用检查，再执行首条 SQL，避免半套协调结果。

服务层映射为：

```go
return apperror.Wrap(
	"ai.provider.model_in_use", apperror.CategoryConflict, http.StatusConflict,
	apperror.Permanent, "", nil, "模型已被智能体或上下文配置引用，不能修改用途或向量规格", err,
)
```

- [ ] **Step 6: 验证 GREEN 并提交**

```powershell
go test ./internal/module/ai/provider -run 'Structured|EmbeddingSpec|ModelInUse|LegacyModelIDs|ModelKindConflict|ReconcileModels' -count=1
git diff --check
git add -- internal/module/ai/provider
git commit -m "feat(ai): persist complete provider model routes"
```

Expected: PASS；新 `models[]` 不再生成 `model_display_names` 或 `statuses` Map，旧混合请求测试仍为 PASS。

### Task 4: 把模型发现改为受审分类和非破坏性同步

**Files:**
- Modify: `internal/module/ai/provider/dto.go`
- Modify: `internal/module/ai/provider/service.go`
- Modify: `internal/module/ai/provider/repository.go`
- Modify: `internal/module/ai/provider/service_test.go`
- Modify: `internal/module/ai/provider/repository_gorm_test.go`

- [ ] **Step 1: 写候选态和同步语义的失败测试**

```go
func TestModelCandidatesClassifyOnlyReviewedIdentity(t *testing.T) {
	result := service.modelCandidates([]provider.Model{
		{ID: "gpt-image-2", OwnedBy: "openai"},
		{ID: "GPT-IMAGE-2", OwnedBy: "openai"},
		{ID: "vendor-embedding-large", OwnedBy: "trusted-looking-name"},
	})
	if result[0].ModelKind == nil || *result[0].ModelKind != ModelKindImage {
		t.Fatalf("mapped candidate=%#v", result[0])
	}
	if result[1].ModelKind != nil || result[2].ModelKind != nil {
		t.Fatalf("unreviewed candidates were guessed: %#v", result)
	}
}
```

同步测试还必须断言：未知新候选不入库；既有人工 Embedding/Rerank 的 kind、规格和状态不变；远端缺失的旧行不禁用；映射到官方但用途冲突时整次写事务返回 `ai.provider.model_kind_conflict`。

- [ ] **Step 2: 运行定向测试并确认 RED**

```powershell
go test ./internal/module/ai/provider -run 'Candidate|Sync.*NonDestructive|Sync.*Conflict|ReviewedIdentity' -count=1
```

Expected: FAIL，当前同步仍把所有上游模型改成 Chat 并用 destructive reconcile。

- [ ] **Step 3: 定义候选 DTO，空 kind 只存在于未持久化响应**

```go
type ModelOptionDTO struct {
	ModelID                  string                      `json:"model_id"`
	DisplayName              string                      `json:"display_name"`
	OwnedBy                  string                      `json:"owned_by"`
	MappingStatus            officialmodel.MappingStatus `json:"mapping_status"`
	OfficialModelID          string                      `json:"official_model_id,omitempty"`
	OfficialCatalogVersion   string                      `json:"official_catalog_version,omitempty"`
	ModelKind                *ModelKind                  `json:"model_kind,omitempty" validate:"omitempty,oneof=chat embedding rerank image"`
	EmbeddingDimensions      *uint32                     `json:"embedding_dimensions,omitempty"`
	EmbeddingMaxInputTokens  *int64                      `json:"embedding_max_input_tokens,omitempty"`
	EmbeddingTokenCounterID  *string                     `json:"embedding_token_counter_id,omitempty"`
}
```

候选构造只调用 `officialmodel.IdentityMatcher` 和 `Catalog.ResolveIdentity`。映射成功时从官方 `Model.ModelKind` 赋值；若用途为 Embedding，同时深拷贝官方 `EmbeddingSpec` 三项。未映射时 kind 与规格指针全部保持 nil，不能从名称或上游 `owned_by` 推断。

- [ ] **Step 4: 新增只合并可信候选的仓储方法**

不要给现有 destructive `ReconcileModels` 增加难懂 scope。新增职责单一的方法：

```go
type discoveredModelMerger interface {
	MergeDiscoveredModels(context.Context, uint64, []ProviderModel) error
}
```

`MergeDiscoveredModels` 在一个事务内只 upsert 参数中已官方映射、用途一致且规格完整的行；保留所有未出现行，不改人工用途/规格/状态。遇到同 model ID 的已存人工用途与官方用途冲突时返回 `ErrProviderModelKindConflict`，不留下部分更新。

- [ ] **Step 5: 重写 `SyncModels` 的数据流**

```go
candidates := s.modelCandidates(upstream)
merge := make([]ProviderModel, 0, len(candidates))
for _, candidate := range candidates {
	if candidate.ModelKind == nil {
		continue
	}
	merge = append(merge, providerModelFromMappedCandidate(candidate))
}
if err := repo.MergeDiscoveredModels(ctx, id, merge); err != nil {
	return nil, syncModelAppError(err)
}
return &ModelOptionsResponse{List: candidates}, nil
```

`providerModelFromMappedCandidate` 必须复制候选的官方用途、官方身份和 Embedding 规格；非 Embedding 规格保持 nil。删除 `providerModels[index].ModelKind = ModelKindChat`。同步状态只有 merge 提交成功后才能写 `ok`；冲突时写 `failed` 和有界错误摘要。

- [ ] **Step 6: 验证 GREEN 并提交**

```powershell
go test ./internal/module/ai/provider -run 'Candidate|Sync|MergeDiscovered|ReviewedIdentity' -count=1
git diff --check
git add -- internal/module/ai/provider
git commit -m "feat(ai): classify provider model candidates"
```

Expected: PASS；搜索 `providerModels[index].ModelKind = ModelKindChat` 无结果。

### Task 5: 用 Scene/Kind 矩阵收口 Agent 和图片执行链

**Files:**
- Modify: `internal/module/ai/agent/model.go`
- Modify: `internal/module/ai/agent/dto.go`
- Modify: `internal/module/ai/agent/service.go`
- Modify: `internal/module/ai/agent/repository.go`
- Modify: `internal/module/ai/agent/service_test.go`
- Modify: `internal/module/ai/agent/repository_gorm_test.go`
- Modify: `internal/module/ai/image/repository.go`
- Modify: `internal/module/ai/image/repository_test.go`

- [ ] **Step 1: 写 Scene/Kind 矩阵失败测试**

必须覆盖：三个文本场景可组合且只接受 Chat；`image_generate` 只能单独出现且只接受 Image；Embedding/Rerank 永不进入 Agent；`gpt-image-2` 图片 Agent 保留原 `provider_model_id`；图片仓储只查询 `ModelKindImage`。

```go
func TestRequiredModelKindForScenes(t *testing.T) {
	tests := []struct {
		scenes []string
		want   aiprovider.ModelKind
		valid  bool
	}{
		{[]string{"chat", "agent_generate", "text_generate"}, aiprovider.ModelKindChat, true},
		{[]string{"image_generate"}, aiprovider.ModelKindImage, true},
		{[]string{"chat", "image_generate"}, "", false},
	}
	for _, test := range tests {
		got, err := requiredModelKind(test.scenes)
		if (err == nil) != test.valid || got != test.want {
			t.Fatalf("scenes=%v kind=%q err=%v", test.scenes, got, err)
		}
	}
}
```

- [ ] **Step 2: 运行 Agent/Image 定向测试并确认 RED**

```powershell
go test ./internal/module/ai/agent ./internal/module/ai/image -run 'RequiredModelKind|Scene.*Kind|Image.*ModelKind|CanonicalGenerationScenes' -count=1
```

Expected: FAIL；当前 Agent 和 Image repository 都硬编码 Chat。

- [ ] **Step 3: 建立一个 Scene 到 Kind 的权威函数**

```go
func requiredModelKind(scenes []string) (aiprovider.ModelKind, error) {
	if len(scenes) == 0 {
		return aiprovider.ModelKindChat, nil
	}
	kind := aiprovider.ModelKindChat
	for _, scene := range scenes {
		if scene == capability.SceneImageGenerate {
			if len(scenes) != 1 {
				return "", errors.New("image_generate must be the only scene")
			}
			kind = aiprovider.ModelKindImage
		}
	}
	return kind, nil
}
```

`normalizeMutationFields` 保存规范化后的 `scenes` 或计算出的 `requiredKind`，`ensureProviderModel` 同时按 model ID 和 required kind 查找。用途不匹配返回稳定错误 `ai.agent.model_scene_mismatch`（409），不回退到 Chat。

- [ ] **Step 4: 让 Agent 查询按主键连接并公开 kind**

`agentSelectDB` 使用已经存在的稳定主键引用：

```go
Joins("LEFT JOIN ai_provider_models pm ON pm.id = a.provider_model_id AND pm.provider_id = a.provider_id AND pm.model_id = a.model_id")
```

`AgentWithProvider`、`ModelOption`、`AgentDTO` 增加 `ProviderModelKind`/`ModelKind`。Page Init 只返回已映射、启用、生命周期可用且 kind 为 Chat/Image 的模型；`gpt-image-2` 仍需通过现有 `image.RequiredModelID` 支持守卫，不能因 `image` 枚举放开未适配图片模型。

- [ ] **Step 5: 把图片仓储两处 Chat 改成 Image**

```go
Joins(
	"JOIN ai_provider_models AS m ON m.id = a.provider_model_id AND m.provider_id = a.provider_id AND m.model_id = a.model_id AND m.model_kind = ? AND m.status = ? AND m.mapping_status = ?",
	aiprovider.ModelKindImage, enum.CommonYes, officialmodel.MappingStatusMapped,
)
```

普通 Chat、Tool、Message repository 继续明确要求 `ModelKindChat`，这是隔离执行链，不是重复特判。

- [ ] **Step 6: 验证 GREEN 并提交**

```powershell
go test ./internal/module/ai/agent ./internal/module/ai/image -run 'RequiredModelKind|Scene.*Kind|Image.*ModelKind|ProviderModels' -count=1
git diff --check
git add -- internal/module/ai/agent internal/module/ai/image/repository.go internal/module/ai/image/repository_test.go
git commit -m "feat(ai): enforce agent scene model kinds"
```

Expected: PASS；Chat 和 Image 的运行查询各自只接受正确 kind。

### Task 6: 由 Provider Model 生成不可变 Profile 规格快照

**Files:**
- Modify: `internal/module/ai/contextengine/admin_dto.go`
- Modify: `internal/module/ai/contextengine/admin_service.go`
- Modify: `internal/module/ai/contextengine/repository.go`
- Modify: `internal/module/ai/contextengine/admin_service_test.go`
- Modify: `internal/module/ai/contextengine/repository_test.go`
- Modify: `internal/module/ai/contextengine/transport/admin/request.go`
- Modify: `internal/module/ai/contextengine/transport/admin/handler_test.go`

- [ ] **Step 1: 写自动复制和兼容请求的失败测试**

```go
func TestCreateProfileCopiesEmbeddingSpecFromProviderModel(t *testing.T) {
	repository := &fakeAdminRepository{models: map[uint64]ProviderModelCapability{
		11: {
			ID: 11, Kind: aiprovider.ModelKindEmbedding, Enabled: true, ProviderEnabled: true,
			EmbeddingDimensions: uint32Pointer(1024),
			EmbeddingMaxInputTokens: int64Pointer(8192),
			EmbeddingTokenCounterID: stringPointer("utf8_bytes_v1"),
		},
	}}
	service := NewAdminService(repository, nil, nil)
	_, appErr := service.CreateProfile(context.Background(), 7, CreateProfileInput{
		Name: "bge-profile", EmbeddingProviderModelID: 11,
		DenseDistance: "cosine", DenseMinScore: "0.200000",
	})
	if appErr != nil {
		t.Fatal(appErr)
	}
	if repository.createdProfile.EmbeddingDimensions != 1024 || repository.createdProfile.EmbeddingMaxInputTokens != 8192 {
		t.Fatalf("snapshot=%#v", repository.createdProfile)
	}
}
```

再覆盖：旧请求三项完全一致时成功；任一不一致返回 `ai.provider.embedding_spec_invalid`（409）；缺规格/禁用/用途错误的 Provider Model 不可选；Page Init 的 Embedding option 返回只读三项规格；Image 不进入任何 Context option。

- [ ] **Step 2: 运行 Context Admin 定向测试并确认 RED**

```powershell
go test ./internal/module/ai/contextengine -run 'CreateProfile.*Embedding|LegacyEmbeddingSpec|PageInit.*ModelOptions' -count=1
```

Expected: FAIL，当前服务仍信任请求中的 1536/8191/Counter。

- [ ] **Step 3: 扩展 capability 查询和 Page Init option**

```go
type ProviderModelCapability struct {
	ID                       uint64
	Kind                     aiprovider.ModelKind
	Enabled                  bool
	ProviderEnabled          bool
	OfficialModelID          string
	EmbeddingDimensions      *uint32
	EmbeddingMaxInputTokens  *int64
	EmbeddingTokenCounterID  *string
}

type ProviderModelOptionDTO struct {
	Value                    uint64  `json:"value"`
	Label                    string  `json:"label"`
	ProviderName             string  `json:"provider_name"`
	ModelID                  string  `json:"model_id"`
	EmbeddingDimensions      *uint32 `json:"embedding_dimensions,omitempty"`
	EmbeddingMaxInputTokens  *int64  `json:"embedding_max_input_tokens,omitempty"`
	EmbeddingTokenCounterID  *string `json:"embedding_token_counter_id,omitempty"`
}
```

Repository 只返回启用 Provider/Model；Embedding option 若缺任一规格视为数据库异常，不塞默认值。

- [ ] **Step 4: 把旧请求规格改为可选兼容字段**

Transport 和 domain 输入使用指针表达“未提交”：

```go
type profileCreateRequest struct {
	Name                     string  `json:"name" binding:"required,max=191"`
	EmbeddingProviderModelID uint64  `json:"embedding_provider_model_id" binding:"required"`
	EmbeddingDimensions      *uint32 `json:"embedding_dimensions"`
	EmbeddingMaxInputTokens  *int64  `json:"embedding_max_input_tokens"`
	EmbeddingTokenCounterID  *string `json:"embedding_token_counter_id" binding:"omitempty,max=64"`
	DenseDistance            string  `json:"dense_distance" binding:"required"`
	DenseMinScore            string  `json:"dense_min_score" binding:"required"`
	RerankerProviderModelID  *uint64 `json:"reranker_provider_model_id"`
	RerankerMinScore         *string `json:"reranker_min_score"`
	MemoryProviderModelID    *uint64 `json:"memory_provider_model_id"`
}
```

服务端先加载 Embedding Provider Model，验证其规格完整和 Counter 已注册，再构造 Profile：

```go
profile := ContextProfile{
	Name: input.Name,
	EmbeddingProviderModelID: embedding.ID,
	EmbeddingDimensions: *embedding.EmbeddingDimensions,
	EmbeddingMaxInputTokens: *embedding.EmbeddingMaxInputTokens,
	EmbeddingTokenCounterID: *embedding.EmbeddingTokenCounterID,
	DenseDistance: string(distance),
	DenseMinScore: dense.String(),
	SparseEncoder: SparseEncoderUnicodeLexicalV1,
	SparseEncoderVersion: SparseEncoderVersionV1,
	RerankerProviderModelID: cloneUint64(input.RerankerProviderModelID),
	RerankerMinScore: cloneString(input.RerankerMinScore),
	MemoryProviderModelID: cloneUint64(input.MemoryProviderModelID),
	Status: ProfileEnabled,
	TargetIndexGeneration: &target,
	IndexState: ProfileIndexProvisioning,
	CreatedBy: actorID,
}
```

如果旧请求提交任一规格，则要求三项同时存在且逐项相等；禁止静默忽略冲突值。

- [ ] **Step 5: 验证 GREEN 并提交**

```powershell
go test ./internal/module/ai/contextengine -run 'CreateProfile|LegacyEmbeddingSpec|PageInit.*ModelOptions|ProviderModelOptionsQuery' -count=1
git diff --check
git add -- internal/module/ai/contextengine
git commit -m "feat(ai): snapshot embedding model specs"
```

Expected: PASS；`CreateProfile` 不再包含 1536、8191 或 Counter fallback。

### Task 7: 增加基于 COS 原始对象的文档版本预览 API

**Files:**
- Modify: `internal/infra/storage/conditional_object.go`
- Modify: `internal/infra/storage/conditional_object_test.go`
- Modify: `internal/infra/storage/cos/object_inspector.go`
- Modify: `internal/infra/storage/cos/object_inspector_test.go`
- Modify: `internal/infra/storage/cos/conditional_object_reader.go`
- Modify: `internal/infra/storage/cos/conditional_object_reader_test.go`
- Modify: `internal/module/ai/contextengine/admin_dto.go`
- Modify: `internal/module/ai/contextengine/admin_service.go`
- Modify: `internal/module/ai/contextengine/repository.go`
- Modify: `internal/module/ai/contextengine/admin_service_test.go`
- Modify: `internal/module/ai/contextengine/repository_test.go`
- Modify: `internal/module/ai/contextengine/transport/admin/route.go`
- Modify: `internal/module/ai/contextengine/transport/admin/handler.go`
- Create: `internal/module/ai/contextengine/transport/admin/preview_handler_test.go`
- Modify: `internal/platform/admin/build.go`
- Modify: `internal/platform/admin/build_test.go`

- [ ] **Step 1: 写存储和服务失败测试**

覆盖以下事实：

```text
只接受 cos + ai_context_documents/ 规范 key
条件 HEAD 必须匹配 ETag、size、MIME
签名方法固定 GET，TTL <= 300 秒
版本通过 document -> space -> platform=admin 且未删除的链加载
queued/processing/failed/ready 都能查看原文件
TXT/Markdown 超过 2 MiB 返回 external
对象不存在或事实变化 -> 409 ai.context.document_version.source_changed
配置/网络/签名失败 -> 503 ai.context.document_version.preview_unavailable
响应带 Cache-Control: no-store, private 和 Pragma: no-cache
```

服务测试的 previewer 只返回事实，不暴露 Secret：

```go
previewer := &fakeConditionalPreviewer{result: storage.ConditionalObjectPreview{
	URL: "https://cos.example/report.md?signature=secret",
	ExpiresIn: 300,
	Metadata: storage.ConditionalObjectMetadata{ETag: `"v1"`, Size: 1024, MIMEType: "text/markdown"},
}}
```

- [ ] **Step 2: 运行 COS/Context 定向测试并确认 RED**

```powershell
go test ./internal/infra/storage/... ./internal/module/ai/contextengine/... ./internal/platform/admin -run 'ContextDocument.*Preview|Conditional.*Preview|PreviewRoute' -count=1
```

Expected: FAIL，尚无文档 preview contract、签名方法和路由。

- [ ] **Step 3: 定义最小存储预览契约并校验命名空间**

```go
type ConditionalObjectPreviewInput struct {
	Object   ConditionalObjectInput
	MIMEType string
}

type ConditionalObjectPreview struct {
	URL       string
	ExpiresIn int64
	Metadata  ConditionalObjectMetadata
}

type ConditionalObjectPreviewer interface {
	Preview(context.Context, ConditionalObjectPreviewInput) (ConditionalObjectPreview, error)
}
```

`TrustedAIContextDocumentObjectKey` 规范化 `/`、拒绝绝对路径和 `..`，并只接受 `ai_context_documents/`。`ConditionalObjectReader.Head/Open/Preview` 都调用它；不能只在 Admin Service 做一次字符串前缀判断。

`Preview` 复用 reader 的 `COSObjectStreamer.objectClient` 和 `conditionalHead`，比较 MIME 后调用：

```go
signedURL, err := client.Object.GetPresignedURL2(ctx, http.MethodGet, objectKey, 5*time.Minute, nil)
```

不把 URL 或 Secret 写日志，不新建第二套 COS 配置读取器。

- [ ] **Step 4: 定义版本预览 DTO 和单次授权查询**

```go
type DocumentPreviewKind string

const (
	DocumentPreviewText     DocumentPreviewKind = "text"
	DocumentPreviewMarkdown DocumentPreviewKind = "markdown"
	DocumentPreviewPDF      DocumentPreviewKind = "pdf"
	DocumentPreviewExternal DocumentPreviewKind = "external"
)

type DocumentVersionPreviewResponse struct {
	URL         string              `json:"url"`
	ExpiresIn   int64               `json:"expires_in"`
	Filename    string              `json:"filename"`
	MIMEType    string              `json:"mime_type"`
	SizeBytes   int64               `json:"size_bytes"`
	PreviewKind DocumentPreviewKind `json:"preview_kind" validate:"oneof=text markdown pdf external"`
}
```

Repository 增加：

```go
FindDocumentVersion(ctx context.Context, platform string, versionID uint64) (*ContextDocumentVersion, error)
```

查询必须 inner join `ai_context_documents` 和 `ai_context_spaces`，限制 `version.id`、`space.platform`、两层 `deleted_at IS NULL`。不存在统一返回 nil，避免泄漏其他平台 ID。

- [ ] **Step 5: 实现 Preview 服务和稳定错误**

```go
func documentPreviewKind(mimeType string, size int64) DocumentPreviewKind {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "text/plain":
		if size <= 2<<20 { return DocumentPreviewText }
	case "text/markdown", "text/x-markdown":
		if size <= 2<<20 { return DocumentPreviewMarkdown }
	case "application/pdf":
		return DocumentPreviewPDF
	}
	return DocumentPreviewExternal
}
```

`PreviewDocumentVersion` 验证正整数 ID 和 `platform=admin`，读取版本后调用 previewer。错误映射必须是：400 `ai.context.document_version.invalid_id`、404 `ai.context.document_version.not_found`、409 `ai.context.document_version.source_changed`、503 `ai.context.document_version.preview_unavailable`。Qdrant、parser state 和 ingestion queue 不出现在该函数中。

- [ ] **Step 6: 注册 GET 路由并设置 no-store**

```go
register(adminroute.Definition{
	Method: http.MethodGet,
	Path: "/api/admin/v1/ai/context-document-versions/:id/preview",
	OperationID: "ai_context_document_version_preview",
	Access: adminroute.Permission("ai_context_view"),
	Audit: adminroute.NoAudit("read-only"),
	Contract: &adminroute.HTTPContract{Response: contextengine.DocumentVersionPreviewResponse{}},
}, handler.PreviewDocumentVersion)
```

Handler 在 `write` 前设置：

```go
c.Header("Cache-Control", "no-store, private")
c.Header("Pragma", "no-cache")
```

`build.go` 将同一个 `*storagecos.ConditionalObjectReader` 传给 constructor，并通过 `WithDocumentPreviewer(reader)` 注入新能力，保留旧 constructor 对测试 fake 的兼容。

- [ ] **Step 7: 验证 GREEN 并提交**

```powershell
go test ./internal/infra/storage/... ./internal/module/ai/contextengine/... ./internal/platform/admin -run 'ContextDocument.*Preview|Conditional.*Preview|PreviewRoute|BuildWiresContext' -count=1
git diff --check
git add -- internal/infra/storage internal/module/ai/contextengine internal/platform/admin/build.go internal/platform/admin/build_test.go
git commit -m "feat(ai): preview context document versions"
```

Expected: PASS；搜索 Preview 服务实现不包含 `qdrant`。

### Task 8: 发布后端 Admin Contract Bundle

**Files:**
- Modify: `internal/admincontract/openapi_models_test.go`
- Modify: `internal/admincontract/openapi_test.go`
- Modify: `internal/admincontract/permissions_test.go`
- Regenerate: `contracts/admin/v1/openapi.json`
- Regenerate: `contracts/admin/v1/permissions.json`
- Regenerate: `contracts/admin/v1/views.json`
- Regenerate: `contracts/admin/v1/manifest.json`
- Regenerate: `contracts/admin/v1/realtime/envelope.schema.json`
- Regenerate: `contracts/admin/v1/realtime/events.schema.json`

- [ ] **Step 1: 写 OpenAPI 闭合枚举和路由失败测试**

在 `openapi_models_test.go` 扩展 `TestAIProviderModelKindAndAgentContextProfileContracts`，断言 Provider/Agent/Official Model 的 `model_kind` enum 精确等于四类，Provider 结构化输入公开可选 `id` 和三项可空 Embedding 规格。在 `openapi_test.go` 增加 `TestAIContextDocumentVersionPreviewContract`，断言候选响应允许 `model_kind` 缺省、Preview Operation 为 GET、Profile create 的旧 Embedding 规格不再 required。在 `permissions_test.go` 增加 Preview Operation 的 `ai_context_view` 与 `no_audit/read-only` 断言。

兼容期的 Provider 请求 schema 必须公开 `display_name/status/Embedding` 字段但不能把它们列入父对象 `required`；对应测试同时断言三个写 operation 的 `x-admin-parameter-rules` 写明“旧 Map 混合形状”和“新完整行”互斥，防止生成契约把服务端兼容规则说错。

- [ ] **Step 2: 运行 source contract 定向测试**

```powershell
go test ./internal/admincontract ./internal/server -run 'ModelKind|ContextDocumentVersionPreview|ContextProfileCreate' -count=1
```

Expected: PASS；前七个任务已经实现 DTO、校验标签和 Preview 路由，反射生成的内存 Contract 必须先自洽。若这里失败，只修对应 DTO/route 定义后重跑，不手改生成 JSON。

- [ ] **Step 3: 提交 Contract 断言并记录 source SHA**

```powershell
git add -- internal/admincontract/openapi_models_test.go internal/admincontract/openapi_test.go internal/admincontract/permissions_test.go
git commit -m "test(contract): lock model kind and document preview"
git status --short
$backendSourceCommit = (git rev-parse HEAD).Trim()
$backendSourceCommit
```

Expected: 工作区 clean；输出 40 位小写 SHA。从此到 bundle 生成结束不得修改后端 source。

- [ ] **Step 4: 生成并核对 Bundle**

```powershell
$backendSourceCommit = (git rev-parse HEAD).Trim()
pwsh -NoProfile -File scripts/generate-admin-contract.ps1 -BackendCommit $backendSourceCommit
go test ./internal/admincontract ./internal/server -run 'ModelKind|ContextDocumentVersionPreview|ContextProfileCreate' -count=1
git diff --check
```

Expected: PASS；`manifest.json.backend_commit` 等于 `$backendSourceCommit`；Preview path 只出现一次。

- [ ] **Step 5: 提交 Bundle**

```powershell
git add -- internal/admincontract contracts/admin/v1
git commit -m "chore(contract): publish model kind and document preview"
```

### Task 9: 同步契约并改造 Provider/Agent 前端

**Files:**
- Regenerate: `../admin_front_ts/contracts/backend/admin/lock.json`
- Regenerate: `../admin_front_ts/contracts/backend/admin/v1/openapi.json`
- Regenerate: `../admin_front_ts/contracts/backend/admin/v1/permissions.json`
- Regenerate: `../admin_front_ts/contracts/backend/admin/v1/views.json`
- Regenerate: `../admin_front_ts/contracts/backend/admin/v1/manifest.json`
- Regenerate: `../admin_front_ts/contracts/backend/admin/v1/realtime/envelope.schema.json`
- Regenerate: `../admin_front_ts/contracts/backend/admin/v1/realtime/events.schema.json`
- Regenerate: `../admin_front_ts/src/modules/http/generated/admin.ts`
- Regenerate: `../admin_front_ts/src/modules/http/generated/operations.ts`
- Regenerate: `../admin_front_ts/src/modules/routing/generated/permissions.ts`
- Regenerate: `../admin_front_ts/src/modules/routing/generated/views.ts`
- Modify: `../admin_front_ts/src/api/ai/official-models.ts`
- Modify: `../admin_front_ts/src/api/ai/providers.ts`
- Modify: `../admin_front_ts/src/api/ai/agents.ts`
- Modify: `../admin_front_ts/src/views/Main/ai/providers/composables/useProviderForm.ts`
- Modify: `../admin_front_ts/src/views/Main/ai/providers/components/ProviderModelEditor.vue`
- Modify: `../admin_front_ts/src/views/Main/ai/providers/components/ProviderModelList.vue`
- Modify: `../admin_front_ts/src/views/Main/ai/providers/components/ProviderFormDialog.vue`
- Modify: `../admin_front_ts/src/views/Main/ai/agents/use-agent-admin-page.ts`
- Modify: `../admin_front_ts/src/views/Main/ai/agents/index.vue`
- Modify: `../admin_front_ts/src/i18n/locales/zh-CN/ai.ts`
- Modify: `../admin_front_ts/src/i18n/locales/en-US/ai.ts`
- Modify: `../admin_front_ts/tests/shared/ai/ai-provider-api-protocol.test.ts`
- Modify: `../admin_front_ts/tests/component/ai/ProviderModelEditor.test.ts`
- Modify: `../admin_front_ts/tests/component/ai/AgentOfficialModelForm.test.ts`

- [ ] **Step 1: 同步锁定的后端 source SHA 并生成类型**

在前端仓库执行：

```powershell
$backendSourceCommit = [string]((Get-Content '..\admin_back_go\contracts\admin\v1\manifest.json' -Raw | ConvertFrom-Json).backend_commit)
npm run contract:sync -- --backend ..\admin_back_go --commit $backendSourceCommit
npm run contract:generate
```

Expected: `contracts/backend/admin/lock.json.backend_commit` 等于 `$backendSourceCommit`，生成类型包含四类 kind、Provider Embedding 规格和 preview operation。

- [ ] **Step 2: 写 Provider/Agent 前端失败测试**

覆盖：候选 `model_kind=null` 时显示“用途待确认”且不能提交；官方候选用途和 Embedding 规格只读；新增空行不默认 Chat；Embedding 三项必填；编辑已有行保留 `id`，新增行不伪造 `id`；相同 model ID 的不同 kind 可共存；新 body 只含完整 `models[]`；Scene 切换到图片时清空 Chat 模型；图片与 Chat 场景不能混选。

```ts
expect(buildProviderMutationParams(form)).toEqual({
  name: 'SiliconFlow',
  engine_type: 'openai',
  base_url: 'https://api.siliconflow.cn/v1',
  api_protocol: 'chat_completions',
  status: 1,
  models: [{
    model_id: 'BAAI/bge-m3', model_kind: 'embedding', display_name: 'BGE M3', status: 1,
    embedding_dimensions: 1024, embedding_max_input_tokens: 8192,
    embedding_token_counter_id: 'utf8_bytes_v1',
  }],
})
```

- [ ] **Step 3: 运行三个定向测试并确认 RED**

```powershell
npm test -- tests/shared/ai/ai-provider-api-protocol.test.ts tests/component/ai/ProviderModelEditor.test.ts tests/component/ai/AgentOfficialModelForm.test.ts
```

Expected: FAIL，当前候选 merge 和新增行仍默认 `chat`。

- [ ] **Step 4: 用一行一个事实重构 Provider 表单状态**

`src/api/ai/official-models.ts` 给 `AiOfficialModelItem` 增加 `model_kind: 'chat' | 'embedding' | 'rerank' | 'image'` 和可空 `embedding_spec`；`normalizeModel` 只复制服务端值，不按 family 或 model ID 推导。

后端为兼容旧混合请求，会把行内新字段发布为可选；这个可选性不能扩散到新前端。在 `src/api/ai/providers.ts` 用生成类型约束枚举，再定义严格写入类型：

```ts
type AiProviderModelCompatibilityInput = NonNullable<AiProviderMutationContractBody['models']>[number]
export type AiProviderModelKind = AiProviderModelCompatibilityInput['model_kind']

export interface AiProviderModelInput {
  id?: number
  model_id: string
  model_kind: AiProviderModelKind
  display_name: string
  status: AiProviderStatus
  embedding_dimensions?: number
  embedding_max_input_tokens?: number
  embedding_token_counter_id?: string
}
```

`AiProviderMutationParams.models` 和 `AiProviderModelsUpdateParams.models` 都使用这个严格类型；API 层不提供构造旧混合 Map 的新入口。

```ts
export interface ProviderModelDraft {
  id?: number
  client_key: string
  model_id: string
  model_kind: AiProviderModelKind | null
  display_name: string
  status: AiProviderStatus
  mapping_status: 'mapped' | 'unmapped'
  official_model_id?: string
  embedding_dimensions: number | null
  embedding_max_input_tokens: number | null
  embedding_token_counter_id: string | null
}
```

`createProviderEditForm` 把响应 `id` 原样写入 draft；新行只生成前端本地 `client_key`，不把它提交后端。AppTable 使用下面的稳定 row key，用户修改 kind 时不改变行身份：

```ts
const providerModelRowKey = (row: ProviderModelDraft): string =>
  row.id ? `persisted:${row.id}` : row.client_key
```

`mergeProviderModelCandidates` 以 `${model_id}\0${model_kind ?? ''}` 判断候选是否已存在，但保留当前行的 `id/client_key`。映射候选复制后端 kind 和官方 Embedding 规格；未映射候选保留 null。`buildProviderMutationParams` 以 `(model_id, model_kind)` 去重并对每行严格验证，然后直接投影 `id/model_id/model_kind/display_name/status` 与三项规格；`id` 只在持久化行存在，不生成三个 Map。

`ProviderModelEditor` 增加 `image` 选项；官方映射 kind 只读；只有 `embedding` 行显示三个输入；其他类型不携带规格。迁移留下的禁用空规格 Embedding 显示“补齐规格后才能启用”，保持禁用时允许保存，切换启用时执行三项必填。继续使用 AppTable 和 Element Plus 默认样式，不增加 `:deep`。

- [ ] **Step 5: 让 Agent 模型选项由 Scene 派生**

```ts
export function modelKindForScenes(scenes: readonly AiAgentScene[]): 'chat' | 'image' | null {
  const image = scenes.includes('image_generate')
  if (image && scenes.length !== 1) return null
  return image ? 'image' : 'chat'
}

const visibleProviderModels = computed(() => {
  const kind = modelKindForScenes(form.value.scenes)
  if (!kind) return []
  return selectableProviderModels(dict.value.provider_model_options)
    .filter(model => model.model_kind === kind)
})
```

Scene 选项根据当前选择禁用不兼容项；kind 改变时显式清空 `model_path`，不自动选择第一项。已有非法组合打开时显示验证错误，不静默改数据。

- [ ] **Step 6: 验证 GREEN 并提交前端第一批**

```powershell
npm test -- tests/shared/ai/ai-provider-api-protocol.test.ts tests/component/ai/ProviderModelEditor.test.ts tests/component/ai/AgentOfficialModelForm.test.ts
git diff --check
git add -- contracts/backend/admin src/modules/http/generated src/modules/routing/generated src/api/ai/official-models.ts src/api/ai/providers.ts src/api/ai/agents.ts src/views/Main/ai/providers src/views/Main/ai/agents src/i18n/locales/zh-CN/ai.ts src/i18n/locales/en-US/ai.ts tests/shared/ai/ai-provider-api-protocol.test.ts tests/component/ai/ProviderModelEditor.test.ts tests/component/ai/AgentOfficialModelForm.test.ts
git commit -m "feat(ai): configure typed provider models"
```

Expected: 三个测试文件 PASS；Provider 新请求不含 `model_ids`、`model_display_names`、`statuses`。

### Task 10: 完成 Profile 只读规格与文档右侧预览抽屉

**Files:**
- Modify: `../admin_front_ts/src/api/ai/context.ts`
- Modify: `../admin_front_ts/src/views/Main/ai/context/components/ContextProfileDialog.vue`
- Modify: `../admin_front_ts/src/views/Main/ai/context/components/ContextDocumentPanel.vue`
- Create: `../admin_front_ts/src/views/Main/ai/context/components/ContextDocumentPreviewDrawer.vue`
- Modify: `../admin_front_ts/src/i18n/locales/zh-CN/ai-extended.ts`
- Modify: `../admin_front_ts/src/i18n/locales/en-US/ai-extended.ts`
- Modify: `../admin_front_ts/tests/component/ai/ContextProfileDialog.test.ts`
- Create: `../admin_front_ts/tests/component/ai/ContextDocumentPreviewDrawer.test.ts`
- Modify: `../admin_front_ts/tests/component/ai/ContextAdminTables.test.ts`

- [ ] **Step 1: 写 Profile 和 Preview 抽屉失败测试**

Profile 测试断言选择 `value=11` 后只读展示后端给出的 `1024 / 8192 / utf8_bytes_v1`，提交 body 不含三项规格。Preview 测试断言：TXT 用转义后的 `<pre>`、Markdown 复用 `MarkdownRenderer`、PDF iframe 带 `referrerpolicy="no-referrer"`、Office 只显示打开/下载；关闭抽屉后 URL 和文本被清空；403 最多自动刷新一次。

```ts
expect(AiContextApi.documents.preview(31)).resolves.toEqual({
  url: 'https://cos.example/report.md?signed=1',
  expires_in: 300,
  filename: 'report.md',
  mime_type: 'text/markdown',
  size_bytes: 1024,
  preview_kind: 'markdown',
})
```

- [ ] **Step 2: 运行 Context 前端定向测试并确认 RED**

```powershell
npm test -- tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextDocumentPreviewDrawer.test.ts tests/component/ai/ContextAdminTables.test.ts
```

Expected: FAIL，Profile 仍硬编码规格且 Preview 组件不存在。

- [ ] **Step 3: Profile 只保留可编辑策略字段**

`ContextProfileDialog` 用选中 option 派生规格：

```ts
const selectedEmbedding = computed(() => props.embeddingModelOptions.find(
  option => option.value === form.embedding_provider_model_id,
) ?? null)

const canSubmit = computed(() => Boolean(
  form.name.trim()
  && (props.profile !== null || selectedEmbedding.value),
))
```

模板使用 `el-descriptions` 显示三项只读规格和“索引快照”说明；删除三个 `el-input-number/el-input`，新建请求只发送模型 ID 和检索策略。已有 Profile 继续从 `profile` 显示自己的历史快照，不拿当前 Provider 值覆盖。

- [ ] **Step 4: API 只消费生成的 Preview Operation**

```ts
export type AiContextDocumentPreview = Output<'ai_context_document_version_preview'>

preview: (versionID: number, options: ExecuteOptions = {}) => executeAdminOperation(
  adminOperations.ai_context_document_version_preview,
  { path: { id: positiveID(versionID, 'Context document version id') } },
  options,
),
```

不手写 URL，不把签名 URL写入路由、localStorage 或 workspace store。

- [ ] **Step 5: 实现单一职责预览抽屉**

组件契约保持 props down/events up：

```ts
const props = defineProps<{
  document: AiContextDocument | null
  version: AiContextDocumentVersion | null
}>()
const visible = defineModel<boolean>({ required: true })
```

本地只持有 `preview`、`content`、`loading`、`error` 和当前 `AbortController`。每次打开请求 preview；Text/Markdown 用：

```ts
const response = await fetch(preview.url, {
  credentials: 'omit',
  referrerPolicy: 'no-referrer',
  signal: controller.signal,
})
if (response.status === 403 && automaticRefreshes < 1) {
  automaticRefreshes += 1
  await loadPreview()
  return
}
if (!response.ok) throw new Error(`Context document fetch failed: ${response.status}`)
content.value = await response.text()
```

Markdown 传给现有安全 `MarkdownRenderer`；TXT 使用插值；PDF iframe 不加灰色背景板；external 用 `el-button` 打开并设置 `noopener,noreferrer`。关闭时 abort 请求并清空签名事实。

- [ ] **Step 6: 让版本行只负责选择并打开抽屉**

`ContextDocumentPanel` 增加 `selectedVersion` 和 `previewVisible` 两个 `shallowRef`。版本按钮点击时赋值并打开 `ContextDocumentPreviewDrawer`；Panel 不自行 fetch COS 内容。保持现有文档 AppTable 和 300px 版本栏，不增加嵌套 Card 或 `:deep` 样式。

- [ ] **Step 7: 验证 GREEN 并提交**

```powershell
npm test -- tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextDocumentPreviewDrawer.test.ts tests/component/ai/ContextAdminTables.test.ts
git diff --check
git add -- src/api/ai/context.ts src/views/Main/ai/context/components/ContextProfileDialog.vue src/views/Main/ai/context/components/ContextDocumentPanel.vue src/views/Main/ai/context/components/ContextDocumentPreviewDrawer.vue src/i18n/locales/zh-CN/ai-extended.ts src/i18n/locales/en-US/ai-extended.ts tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextDocumentPreviewDrawer.test.ts tests/component/ai/ContextAdminTables.test.ts
git commit -m "feat(ai): preview context documents"
```

Expected: 三个测试文件 PASS；不存在硬编码 `1536`、`8191` 或 Profile Counter fallback。

## Final Focused Verification

以下命令只在所有任务完成后各运行一次；不扩成全量测试：

```powershell
# backend
go test ./internal/module/ai/officialmodel ./internal/module/ai/provider ./internal/module/ai/agent ./internal/module/ai/image ./internal/module/ai/contextengine ./internal/infra/storage/... ./internal/admincontract ./internal/architecture -run 'ModelKind|Candidate|EmbeddingSpec|Scene.*Kind|ContextDocument.*Preview|ContextProfile|ProviderModel' -count=1
git diff --check

# frontend
npm test -- tests/shared/ai/ai-provider-api-protocol.test.ts tests/component/ai/ProviderModelEditor.test.ts tests/component/ai/AgentOfficialModelForm.test.ts tests/component/ai/ContextProfileDialog.test.ts tests/component/ai/ContextDocumentPreviewDrawer.test.ts tests/component/ai/ContextAdminTables.test.ts
git diff --check
```

不运行 `admin-dev`、真实迁移、付费 Provider 请求、Docker E2E、Playwright、`go test ./...`、`npm test` 全量或 `npm run build`。

## Manual Acceptance Checklist

1. 在硅基流动供应商拉取当前官方目录能识别的模型，Chat/Image 用途自动显示且只读；未知模型显示“用途待确认”。
2. 手工把未知 `BAAI/bge-m3` 配成向量，填写真实 1024/8192/Counter 后保存；不填写或填错明确报错，不默认 1536。
3. 同一供应商再添加第二个 Embedding 路由，两个都能分别出现在 Profile 选择器中。
4. 新建 Profile 只选择 Embedding 模型，三项规格自动只读显示；旧 Profile 的历史快照不变化。
5. 普通 Agent 只能选择 Chat；图片 Agent 只能选择 `gpt-image-2`，且 `image_generate` 不能与其他场景混选。
6. 普通聊天、图片附件、文件附件、刷新历史、WebSocket、余额扣减和运行监控保持原行为。
7. 关闭 Agent 的 Context Profile 后，不调用 Embedding/Qdrant，但普通聊天和当前附件仍正常。
8. 新建另一 Profile 并切换 Agent；旧 Profile、旧空间、旧文档和旧 Qdrant Collection 不被重写。
9. 点击 TXT、Markdown、PDF 版本，在右侧抽屉查看；Markdown 不执行原始 HTML，PDF 可滚动。
10. 点击 DOCX/XLSX，只看到正式文件信息和打开/下载，不出现伪预览。
11. 删除或替换 COS 测试对象后再预览，页面明确显示源文件变化或暂不可用，不显示空白内容。
12. 签名 URL 不出现在数据库、浏览器路由、localStorage、业务日志或审计 payload 中。

## Completion Criteria

```text
【数据结构】officialmodel 唯一拥有 ModelKind；Provider Model 拥有路由规格；Profile 只保存不可变快照。
【特殊情况】gpt-image-2 只在一次性迁移中被特殊处理，运行时成为普通 image 路由。
【复杂度】无探测、无名称猜测、无 unknown 运行类型、无 Office 转换、无第二套存储。
【兼容性】旧 Chat 输入、旧 Profile、旧索引、聊天/附件/Run/计费行为保持可用。
【结论】十个任务全部完成、定向验证通过并交由用户做真实迁移和人工终测。
```
