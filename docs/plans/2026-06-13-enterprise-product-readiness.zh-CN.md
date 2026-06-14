# 企业与开发就绪路线计划

**目标：** 把 Soniq 从“能跑通音频智能 pipeline 的本地产品原型”推进到“企业/团队可以认真试用、开发可以持续迭代”的状态。

这份计划记录后续开发顺序。除非有明确业务优先级变化，后续开发按这里的 step 拆小任务推进。

## 当前判断

Auth、workspace、上传、转写、总结、思维导图、失败重试已经够支撑下一阶段产品闭环。下一步不优先做 JWT 或复杂企业权限，而是先让 recording 结果变成可管理、可搜索、可导出、可再生成的知识资产。

## Step 1: Recording Library & Result Actions

**目标：** 让用户能日常管理和复用已处理的 recording。

范围：

- Recording 重命名。
- Recording 删除。
- 删除时清理：
  - `recordings` 及子表；
  - local/S3 object artifacts；
  - 明确 Temporal history 不会立刻物理删除。
- Recording 列表搜索、筛选、分页：
  - title 搜索；
  - status 筛选；
  - workflow type 筛选；
  - cursor 或稳定分页。
- 结果导出：
  - transcript `.txt` / `.md`；
  - summary `.md`；
  - mind map `.md` / JSON。
- 结果复制：
  - copy summary；
  - copy transcript；
  - copy mind map markdown。
- 历史结果重新生成：
  - regenerate summary；
  - regenerate mind map。

验收：

- Web UI 能完成上传后结果浏览、搜索、导出、删除。
- API 有明确 workspace 权限校验。
- 删除行为有文档说明。
- smoke 覆盖至少一条导出或删除路径。

## Step 2: Enterprise Data Safety

**目标：** 给企业用户解释清楚数据怎么进入、怎么处理、怎么删除、谁做过什么。

范围：

- Retention policy 基础：
  - original audio 保留策略；
  - normalized audio 保留策略；
  - transcript/summary/mind-map 保留策略。
- Audit log 基础事件：
  - signup/signin/signout；
  - upload；
  - workflow start/completed/failed；
  - transcript generated；
  - summary generated；
  - mind map generated；
  - export；
  - delete；
  - provider configuration changed。
- Provider usage log：
  - provider；
  - model；
  - request time；
  - result status；
  - recording id；
  - workspace id。
- Workspace privacy setting：
  - allow/disallow external model providers。
- 删除语义文档：
  - Postgres；
  - object storage；
  - Temporal history retention。

验收：

- 企业管理员能回答“谁上传了什么、什么时候处理、用了哪个 provider、是否导出/删除”。
- 外部模型使用有 workspace 级开关和记录。

## Step 3: Workspace & RBAC Productization

**目标：** 从基础 workspace membership 进入企业团队协作。

范围：

- Workspace 成员管理 UI。
- 邀请成员。
- 基础角色：
  - owner；
  - admin；
  - editor；
  - viewer。
- 权限边界：
  - 上传 recording；
  - 查看 recording；
  - 删除 recording；
  - 导出结果；
  - 配置 provider；
  - 管理成员。
- API middleware 中集中做 role check。

验收：

- owner 可以邀请和移除成员。
- viewer 不能删除、导出受限资源或修改 provider 配置。
- API 和 Web UI 行为一致。

## Step 4: Production Self-Hosting

**目标：** 让企业可以用 Docker/Compose 或 Kubernetes 方式部署。

范围：

- API Docker image。
- Worker Docker image。
- Production docker-compose。
- Migration job/command。
- S3-compatible storage provider，优先 MinIO 本地验证。
- TLS/reverse proxy 文档。
- Backup/restore 文档：
  - Soniq Postgres；
  - object storage；
  - Temporal database。
- Health/readiness endpoints。
- Structured logs。
- 基础 metrics。

验收：

- 一套 production-ish compose 能从空库启动、迁移、上传、处理、查看结果。
- 文档包含备份和恢复路径。

## Step 5: Workflow Robustness

**目标：** 面对长音频、失败、重跑和 provider 波动时仍然可恢复。

范围：

- `workflow_runs` 表。
- 处理取消：
  - cancel processing API；
  - UI 操作。
- 单步重试：
  - retry transcription；
  - retry summary；
  - retry mind map。
- Reprocess：
  - with different transcription provider；
  - with different LLM provider/model；
  - with different summary/mind-map template。
- Provider timeout/retry/backoff 策略。
- 长音频 chunking。

验收：

- 用户能看见每次 workflow run 的状态和失败原因。
- 失败后能针对具体步骤恢复，不必整条 recording 从头重跑。

## Step 6: Developer Engineering

**目标：** 让项目可以长期快速迭代，降低回归风险。

范围：

- CI：
  - `make fmt`；
  - `make lint`；
  - `make test`；
  - `pnpm test`；
  - `pnpm typecheck`；
  - `pnpm web:build`。
- Migration 测试。
- OpenAPI contract check。
- Smoke 分层：
  - fast smoke；
  - full local pipeline smoke；
  - provider fake-server smoke；
  - manual real-provider smoke。
- Demo seed data。
- Playwright Web E2E：
  - signup/signin；
  - upload；
  - completed details；
  - export；
  - delete。

验收：

- PR 级检查能覆盖主要回归。
- 新开发者能用文档和 seed data 快速跑起完整 demo。

## 推荐执行顺序

1. Recording rename/delete/export。
2. Recording search/filter/pagination。
3. Regenerate summary/mind map。
4. Audit log + provider usage log。
5. Workspace member management + RBAC。
6. Production compose + S3/MinIO。
7. Workflow runs + cancel/retry/reprocess。
8. CI + Playwright E2E。

## 当前不优先

- JWT migration：当前 server-side session 足够。
- 复杂多租户计费。
- 高级组织层级。
- 完整插件系统。
- 大规模分布式微服务拆分。
