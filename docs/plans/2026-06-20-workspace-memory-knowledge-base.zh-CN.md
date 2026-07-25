# Workspace Memory 与 Ask AI 知识库计划

## 背景

Soniq 当前已经可以完成 recording 上传、转写、总结、思维导图、列表管理、
删除/恢复、基础部署与可观测性。下一阶段的核心产品价值不只是“处理单个音频”，
而是让每个 workspace 随着会议、访谈、课程、memo 的积累，形成一个可检索、
可追问、可引用、可持续增长的公司知识库。

这条路线对应的产品目标是：

```txt
Every workspace grows its own memory from meetings.
```

用户不应该只是在 recording 详情页看一次转写和总结。用户应该可以问：

- 上周客户 A 的核心诉求是什么？
- 最近三次产品会议里反复出现的问题是什么？
- 我们决定过哪些 action items，还没有完成哪些？
- 某个功能为什么这样设计，当时是谁提出的？
- 这个客户之前提到过哪些限制、预算、竞争对手？

## 行业参考

当前市场上相关产品大致分为四类：

- **Enterprise search / Work AI**：Glean、Atlassian Rovo。
  重点是连接企业数据源、权限感知搜索、工作上下文问答。
- **Company AI agent workspace**：Dust。
  重点是让团队把多个数据源配置给 agent，并围绕公司资料执行任务。
- **Professional knowledge work**：Hebbia。
  重点是复杂文档/资料分析、引用、可追溯工作流。
- **Open-source memory / knowledge infrastructure**：Mem0、Cognee、Graphiti/Zep。
  重点是 memory 抽取、向量检索、图谱/实体关系、时间感知事实。

参考资料：

- Atlassian Rovo: https://www.atlassian.com/software/rovo
- Dust Knowledge: https://docs.dust.tt/docs/knowledge
- Hebbia: https://www.hebbia.ai/product
- Mem0 docs: https://docs.mem0.ai/
- Cognee docs: https://docs.cognee.ai/
- Graphiti docs: https://help.getzep.com/graphiti
- pgvector: https://github.com/pgvector/pgvector

## 结论

Soniq 不应该一开始直接照搬 Mem0 的个人/agent memory 模型，也不应该一开始就接入
很重的外部知识库平台。

更合适的路径是：

1. 先做 **workspace-scoped RAG knowledge base**。
2. 由 recording 的 transcript、summary、mind map 生成可引用的 knowledge chunks。
3. 用 Postgres + pgvector 作为第一版存储和检索基础。
4. Ask AI 的回答必须带 citation，能跳回原始 recording、时间段和来源内容。
5. 后续再加入 Mem0/Cognee/Graphiti 里成熟的 memory 思路：
   - durable facts；
   - entity linking；
   - temporal validity；
   - hybrid search；
   - graph relationships；
   - memory update / supersede。

这样做的原因：

- Soniq 的权威数据源是 recording、transcript、summary、mind map。
- 企业用户更关心可追溯、权限、删除、审计，而不是只要一个黑盒 memory。
- 自建第一版可以控制 workspace 权限、citation、软删除/硬删除、备份恢复。
- 后面仍然可以通过 provider abstraction 接入 Mem0、Graphiti 或外部向量库。

## 产品目标

第一阶段目标：

- 用户可以在 workspace 内对所有已完成 recordings 提问。
- Ask AI 回答能引用具体 recording 和 transcript segment。
- 新 recording 处理完成后，自动进入知识库索引。
- 删除或 purge recording 后，相关知识索引不会继续被检索出来。
- 所有行为遵守 workspace 权限边界。

第二阶段目标：

- 系统能从 recording 中抽取稳定事实、决策、任务、客户偏好、项目背景。
- 知识可以按时间演进，而不是把所有旧信息都当成永远有效。
- 用户可以追问“这个结论来自哪里”、“什么时候决定的”、“后来有没有变化”。

## 非目标

第一版不做：

- 跨 workspace 搜索。
- 复杂企业连接器，例如 Slack、Notion、Google Drive、Confluence。
- 完整知识图谱 UI。
- 自动替用户执行外部动作。
- 复杂 agent 编排。
- 直接把 Mem0、Cognee 或 Graphiti 作为硬依赖。
- 把 transcript 原文全部塞进 prompt，不做检索和上下文裁剪。

## 核心概念

### Source

Source 是知识的原始来源。第一版只支持 recording 派生内容：

```txt
recording transcript
recording transcript segment
recording summary
recording mind map
```

以后可以扩展到 imported documents、web pages、integrations。

### Chunk

Chunk 是用于检索的最小文本单元。

建议第一版 chunk 来源：

- transcript segment 合并后的时间窗口；
- summary section；
- mind map node path；
- action item / decision 结构化条目。

Chunk 必须保存 provenance：

```txt
workspace_id
recording_id
source_type
source_id
start_ms
end_ms
text
metadata
```

### Embedding

Embedding 是 chunk 的向量表示，用于语义检索。

第一版建议：

- 表结构先 provider-neutral；
- embedding provider 通过配置选择；
- 默认 automated tests 使用 fake embedding provider；
- 真实 provider 只在 manual smoke 或本地人工验证中启用。

### Memory Item

Memory item 是比 chunk 更“知识化”的事实或结论。

例如：

```txt
Customer Acme requires on-prem deployment.
The team decided to keep server-side sessions for now.
Project Soniq prioritizes workspace memory before release automation.
```

第一版可以先不实现 memory item，只做 chunk RAG。第二版再从 recordings 中抽取
memory item，并保留 source citations。

### Citation

Citation 是 Ask AI 答案可信的基础。

每条答案中的关键结论都应能回到：

```txt
recording_id
recording_title
timestamp range
source text snippet
```

没有 citation 的回答只能作为模型推理，不能当成 workspace 已知事实。

## 推荐架构

第一版架构：

```txt
RecordingProcessingWorkflow completed
  ↓
IndexRecordingKnowledge activity
  ↓
Build chunks from transcript / summary / mind map
  ↓
Embed chunks
  ↓
Upsert knowledge chunks + embeddings into Postgres/pgvector

Web Ask AI panel
  ↓
POST /workspaces/{workspace_id}/ask
  ↓
Authenticate + authorize workspace
  ↓
Retrieve top chunks by semantic search and filters
  ↓
Call LLM with compact context
  ↓
Return answer + citations
```

长期架构：

```txt
Recordings
  ↓
Knowledge chunks
  ↓
Memory extraction
  ↓
Entities / facts / relationships
  ↓
Hybrid retrieval
  ↓
Ask AI / Agent workflows / Reports
```

## 数据模型草案

### `knowledge_chunks`

保存可检索文本块。

字段草案：

```txt
id
workspace_id
recording_id
source_type
source_id
chunk_type
text
start_ms
end_ms
metadata jsonb
content_hash
created_at
updated_at
deleted_at
```

说明：

- `content_hash` 用于幂等 upsert，避免 workflow retry 产生重复 chunk。
- `deleted_at` 用于 recording soft delete 或知识索引失效。
- 不把 `recording_id` 放进 metrics label，只放 DB 和 logs。

### `knowledge_chunk_embeddings`

保存 chunk embedding。

字段草案：

```txt
chunk_id
provider
model
dimensions
embedding vector(...)
created_at
```

说明：

- 如果第一版使用 pgvector，需要 migration 启用 `vector` extension。
- 维度和模型绑定，换 embedding model 时可以重建索引。

### `ask_ai_sessions`

保存一次 Ask AI 对话的元信息。

字段草案：

```txt
id
workspace_id
user_id
title
created_at
updated_at
```

### `ask_ai_messages`

保存用户问题、模型回答、引用结果。

字段草案：

```txt
id
session_id
role
content
citations jsonb
model
created_at
```

第一版可以先只做一次性 question/answer，不做多轮 session；但表结构可以预留。

### `memory_items` 后续表

第二阶段再加。

字段草案：

```txt
id
workspace_id
kind
subject
predicate
object
text
confidence
valid_from
valid_to
superseded_by_id
metadata jsonb
created_at
updated_at
deleted_at
```

这部分参考 Mem0 的 durable memory、Graphiti 的 temporal fact，但必须保留 Soniq
自己的 recording citations。

## API 草案

### Ask AI

```txt
POST /workspaces/{workspace_id}/ask
```

Request:

```json
{
  "question": "上周产品会议决定了什么？",
  "filters": {
    "recording_ids": [],
    "from": null,
    "to": null
  }
}
```

Response:

```json
{
  "answer": "...",
  "citations": [
    {
      "recording_id": "...",
      "recording_title": "...",
      "start_ms": 120000,
      "end_ms": 180000,
      "snippet": "..."
    }
  ]
}
```

### Knowledge indexing status

```txt
GET /workspaces/{workspace_id}/recordings/{recording_id}/knowledge-status
```

用于展示某个 recording 是否已经进入知识库。

### Reindex

```txt
POST /workspaces/{workspace_id}/recordings/{recording_id}/reindex-knowledge
```

用于 embedding provider 更换、chunking 策略升级、索引失败后重试。

## 前端体验

第一版推荐入口：

- Workspace 顶部增加 `Ask AI`。
- Recording detail 页增加 `Ask about this recording`。
- 回答区显示 citation cards。
- 点击 citation 跳回 recording detail，并定位 transcript 时间段。

交互原则：

- 不把 Ask AI 做成营销式聊天页。
- 首屏应该是可用的问题输入、范围选择、答案和引用。
- 回答默认短而可验证。
- 没有足够证据时明确说“当前 workspace 知识库里没有找到依据”。
- 引用比流畅回答更重要。

## 权限与安全

必须满足：

- Ask AI 只能检索当前用户有权限访问的 workspace。
- 不能跨 workspace 混检。
- Soft-deleted recordings 默认不参与检索。
- Purged recordings 必须清理或失效相关 chunks 和 embeddings。
- Logs 不记录完整问题和回答，除非未来有明确审计策略和脱敏机制。
- Metrics 不带 user_id、workspace_id、recording_id、question text。
- 外部 embedding / LLM provider 使用必须受 workspace privacy setting 控制。

## Temporal 与 idempotency

知识索引应该由 worker 处理，不在 API handler 里直接执行。

推荐新增 workflow/activity：

```txt
IndexRecordingKnowledge
  ↓
BuildRecordingKnowledgeChunks
  ↓
EmbedKnowledgeChunks
  ↓
PersistKnowledgeChunks
```

实现要求：

- activity 可以被 Temporal retry。
- chunk key 或 content hash 必须稳定。
- 同一 recording 重跑索引不会产生重复 chunk。
- reindex 可以按 `recording_id + index_version` 重建。
- Ask AI 只读已成功索引的数据。

## Provider 边界

新增 provider 类型：

```txt
EmbeddingProvider
QuestionAnswerProvider 或 LLMProvider 扩展
MemoryExtractionProvider 后续阶段再加
```

第一版建议：

- fake embedding provider：用于单元测试和 smoke。
- OpenAI-compatible embedding provider：用于真实本地人工验证。
- 后续再评估 DashScope / 国内 embedding provider。

不要把具体 vendor 的请求结构泄露到业务层。

## 分阶段实施

## Phase 1 — Knowledge chunk 与 embedding 基础

目标：让 completed recording 可以进入可检索索引。

范围：

- 添加 pgvector migration。
- 添加 `knowledge_chunks` 和 `knowledge_chunk_embeddings`。
- 从 transcript segments 生成稳定 chunks。
- 添加 fake embedding provider。
- 添加 Postgres repository。
- 添加 worker activity，在 recording completed 后索引。
- 添加 reindex 单元测试和幂等测试。

验收：

- 完成一个 recording 后，数据库中出现 chunks 和 embeddings。
- 重跑 indexing 不产生重复 chunk。
- soft-deleted recording 不被检索。
- automated tests 不调用真实 embedding provider。

## Phase 2 — Ask AI API

目标：用户可以对 workspace 提问，并获得带 citation 的回答。

范围：

- `POST /workspaces/{workspace_id}/ask`。
- workspace auth check。
- semantic retrieval。
- prompt assembly。
- LLM answer generation。
- citations response。
- OpenAPI 更新。
- API client 更新。
- backend tests 覆盖权限、空结果、citation shape。

验收：

- 对已完成 recording 提问，可以返回答案和引用。
- 没有相关知识时返回明确的 no-evidence response。
- password、cookie、token、audio/transcript 原文不会进入 logs。

## Phase 3 — Web Ask AI 体验

目标：在 Web UI 中把知识库问答做成可用功能。

范围：

- Workspace-level Ask AI 页面。
- Recording-level Ask panel。
- Citation card。
- 点击 citation 定位 transcript 时间段。
- loading / error / no evidence states。

验收：

- 用户可以从 recording list 或 detail 进入 Ask AI。
- 回答引用可点击。
- UI 不要求移动端优先，但桌面布局要稳定。

## Phase 4 — Memory extraction

目标：从 recording 中抽取长期有价值的 workspace facts。

范围：

- `memory_items` 表。
- extract decisions / action items / customer facts / project facts。
- source citations。
- memory confidence。
- memory supersede 初版。
- memory search 混合 chunk retrieval。

验收：

- 系统能回答跨 recording 的事实类问题。
- 能看见某条 memory 来自哪些 recordings。
- 后续变化不会简单覆盖历史，而是保留时间线。

## Phase 5 — Hybrid retrieval 与 entity graph

目标：提升企业知识库在真实工作场景下的召回和解释能力。

范围：

- BM25 keyword search。
- semantic + keyword result fusion。
- entity extraction。
- entity linking。
- simple relationships。
- temporal valid_from / valid_to。

验收：

- 人名、客户名、项目名、产品名可以稳定召回。
- 能区分旧结论和新结论。
- 可以回答“什么时候改变的”和“这个说法后来是否被推翻”。

## 可能涉及的文件

后端：

- `backend/migrations/*.sql`
- `backend/internal/domain`
- `backend/internal/knowledge`
- `backend/internal/providers/embedding`
- `backend/internal/activities`
- `backend/internal/workflows`
- `backend/internal/api`
- `backend/doc/openapi.yaml`

前端：

- `packages/api-client`
- `apps/web/src`

文档：

- `docs/architecture.md`
- `docs/workflows.md`
- `docs/providers.md`
- `docs/development.md`

## 测试策略

后端：

- chunking unit tests。
- embedding provider fake tests。
- Postgres repository tests。
- API handler tests。
- workflow/activity testsuite。
- soft delete / purge 与 knowledge index 的一致性测试。

前端：

- Ask AI form tests。
- citation rendering tests。
- API client tests。

Smoke：

- 默认 smoke 使用 fake embedding + fake LLM。
- 真实 embedding / LLM provider 只做 manual opt-in smoke。

验证命令：

```bash
make fmt
make lint
make test
pnpm test
pnpm typecheck
pnpm web:build
```

必要时：

```bash
make smoke-postgres-temporal
```

## 风险

### Citation 不可信

如果回答没有引用或引用太粗，企业用户不会信任 Ask AI。

处理方式：

- 第一版就强制返回 citation。
- prompt 中要求只根据 retrieved context 回答。
- 没有依据时返回 no evidence。

### Chunk 质量差

直接按固定字数切 transcript 可能破坏语义。

处理方式：

- 第一版按 transcript segment 时间窗口合并。
- 保留 start/end timestamp。
- 后续引入 section-aware chunking。

### Embedding provider 更换

不同 embedding model 维度不同，不能混用。

处理方式：

- embedding 表保存 provider/model/dimensions。
- reindex 支持按 index version 重建。

### 企业权限和删除

知识库如果绕开 recording 权限，会造成严重数据泄露。

处理方式：

- 检索必须带 workspace_id。
- Ask API 先做 workspace auth。
- purge 要清理或失效 knowledge rows。

### 过早引入复杂外部系统

直接引入完整 memory 平台会增加部署、备份、权限和调试复杂度。

处理方式：

- 第一版用 Postgres + pgvector。
- 把外部 memory/vector/graph 系统放在 provider abstraction 后面。

## 推荐执行顺序

1. 写详细数据模型和 API 子计划。
2. 添加 pgvector 与 knowledge tables。
3. 实现 transcript-to-chunks。
4. 实现 fake embedding provider。
5. 在 worker 完成 recording 后自动索引。
6. 实现 Ask AI API，先返回 retrieval + citation。
7. 接入 LLM answer generation。
8. 做 Web Ask AI 页面。
9. 做 memory item extraction。
10. 做 hybrid retrieval / entity graph。

## 当前不做

- 不直接接入 Mem0 作为核心存储。
- 不接企业外部数据源。
- 不做跨 workspace agent。
- 不做复杂知识图谱 UI。
- 不做自动执行类 agent action。
