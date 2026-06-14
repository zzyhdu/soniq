# Recording Library UX 蓝图

**目标：** 在进入 recording 重命名、删除、导出、搜索、重新生成等交互开发前，先固定 Soniq Web UI 的整体产品体验方向，作为后续 UI 设计和实现的基准。

这份蓝图偏产品交互，不定义具体视觉稿。视觉稿可以基于本文最后的 Stitch prompt 生成。

## 产品定位

Soniq Web UI 不是营销站，也不是一次性 demo 页面。它应该是一个面向工作流的音频知识工作台：

- 用户上传音频后，可以持续管理、搜索和复用 recording。
- 转写、总结、思维导图是同一个 recording 的不同结果视图。
- 交互应当高密度、清晰、克制，适合重复使用。
- 首屏直接进入可操作的工作区，不做 landing page。

## 信息架构

主结构：

- Top Bar：workspace、全局搜索、上传入口、用户菜单。
- Recording Library：recording 列表、搜索、筛选、分页。
- Recording Detail：当前 recording 的标题、状态、操作和结果内容。

详情内容使用 tabs：

- Summary
- Mind Map
- Transcript
- Metadata

核心对象是 recording。所有结果、导出、删除、重新生成都从 recording detail 发起。

## 桌面端布局

桌面端使用双栏工作台布局：

- 左侧为 Recording Library。
  - 宽度固定或可在 320-420px 内响应式调整。
  - 顶部包含搜索框、筛选入口和上传按钮。
  - 列表项展示 title、status、workflow type、更新时间和少量摘要信息。
- 右侧为 Recording Detail。
  - 顶部是 detail header。
  - 中间是结果 tabs。
  - 底部不放主要导航，避免和内容滚动冲突。

空状态：

- 没有 recording 时，左侧显示简洁空状态和上传入口。
- 右侧显示当前 workspace 的空白 detail，占位应引导上传，但不使用营销式 hero。

## Recording Detail

Detail header 包含：

- 可内联编辑的 title。
- status badge。
- workflow type、language、created/updated 时间。
- 操作工具栏：
  - Export
  - Regenerate
  - Retry failed step
  - More
  - Delete

title 编辑规则：

- 默认展示为普通标题。
- 点击编辑图标或标题区域进入 inline edit。
- Enter 保存，Esc 取消。
- 保存中显示轻量 loading 状态。
- 保存失败保留原 title，并显示错误。

## 结果 Tabs

Summary tab：

- 优先展示结构化 summary。
- 提供 copy summary 和 export markdown。
- 后续可扩展 action items、decisions、chapters。

Mind Map tab：

- 展示可展开/折叠的树状结构。
- 节点内容应紧凑，避免大卡片堆叠。
- 提供 copy markdown、export markdown、export JSON。
- 后续如果接入可视化 canvas，树状视图仍然保留为可访问 fallback。

Transcript tab：

- 展示 segment 列表。
- segment 应包含 speaker/timestamp 的预留位置，即使当前 provider 还没有 diarization。
- 提供 copy transcript、export txt、export markdown。

Metadata tab：

- 展示 recording id、workspace id、provider、model、duration、file info、workflow state。
- 面向开发和排障，信息密度可以高一些。

## Upload Flow

上传应作为 drawer 或 modal 进入，而不是离开当前工作台页面：

- 入口在 Top Bar 和 Library 空状态中都存在。
- 表单字段：
  - title
  - workflow type
  - language
  - audio file
- 上传成功后：
  - 自动把新 recording 插入列表顶部；
  - 自动选中新 recording；
  - detail 区进入 processing state。

## Processing State

处理中状态应展示一个 stepper：

1. Uploaded
2. Processing
3. Transcribing
4. Summarizing
5. Generating mind map
6. Completed

每一步显示状态：

- pending
- active
- completed
- failed

用户应能在 processing 期间看到 recording 基础信息，并且可以离开后回来继续查看。

## Failed State

失败状态显示在 detail 区，而不是只在列表里显示一个 badge：

- 失败步骤。
- 失败原因。
- Retry 按钮。
- Metadata tab 中展示更完整的错误上下文。

如果失败发生在 summary 或 mind map 阶段，未来可以保留已经成功生成的 transcript，并允许单步 retry。

## Export Flow

Export 使用下拉菜单：

- Summary Markdown
- Transcript Text
- Transcript Markdown
- Mind Map Markdown
- Mind Map JSON
- All as ZIP（后续）

导出成功后不需要跳转页面。失败应在当前上下文中显示错误。

## Regenerate Flow

Regenerate 使用下拉菜单或轻量 dialog：

- Regenerate summary
- Regenerate mind map
- Regenerate summary and mind map

重新生成期间：

- 保留旧结果可读。
- 在对应 tab 上显示 regenerating 状态。
- 新结果成功后替换旧结果。
- 失败时继续保留旧结果，并显示失败原因。

## Delete Flow

Delete 必须使用确认 dialog：

- 明确说明会删除 recording 和相关结果数据。
- 明确说明 local/S3 artifacts 会被删除。
- 明确说明 Temporal history 受 Temporal retention 控制，不会立刻物理删除。
- 用户确认后从列表移除 recording，并清空或切换 detail。

## 搜索、筛选、分页

Library 顶部提供：

- title search。
- status filter。
- workflow type filter。
- date filter（后续可加）。

分页采用稳定分页或 cursor pagination。滚动加载和显式分页都可以，但早期实现建议显式分页，便于测试和排障。

## 移动端

移动端使用两级页面：

- Library screen。
- Detail screen。

从列表进入详情后，顶部显示 back button。detail 内继续使用 tabs，但 tabs 要横向可滚动，避免挤压文字。

Upload 可以使用全屏 dialog 或 bottom sheet。

## 设计原则

- 工作台优先，不做 landing page。
- 信息密度适中，避免过大的装饰性卡片。
- 卡片只用于列表项、结果区块、dialog，不做卡片套卡片。
- 状态和操作要靠近对象本身。
- 操作按钮优先使用图标加 tooltip，关键 destructive action 使用文字和确认。
- 视觉风格应克制、专业、偏企业工具。
- 颜色不应只依赖单一蓝紫渐变；status、action、content 层次需要明确区分。
- 所有状态都必须有对应界面：empty、loading、processing、completed、failed、unauthorized。

## Stitch Prompt

下面的 prompt 可直接给 Stitch。建议先生成 desktop，再让 Stitch 基于同一方向补 mobile。

```text
Design a polished SaaS web app workspace for Soniq, a self-hostable audio intelligence product. The app is for uploading recordings, tracking processing, reading transcripts, summaries, and generated mind maps, then managing those recordings over time.

Do not design a marketing landing page. The first screen must be the actual usable product workspace.

Product tone:
- Professional, calm, enterprise-ready, work-focused.
- Dense enough for repeated daily use, but still clear and modern.
- Avoid oversized hero sections, decorative gradients, glassmorphism, and card-heavy marketing layouts.
- Avoid a one-color purple/blue theme. Use a restrained neutral base with distinct status colors and subtle accent colors.

Main desktop layout:
- Top navigation bar with workspace switcher on the left, global search, upload button, and user/account menu on the right.
- Two-column workspace below the top bar.
- Left column: Recording Library, about 320-420px wide.
- Right column: Recording Detail, filling the rest of the screen.

Recording Library:
- Header with "Recordings", search input, status filter, workflow type filter, and upload action.
- Recording list rows/cards showing title, status badge, workflow type, updated time, and a short preview.
- Include states for empty library, selected item, processing item, completed item, and failed item.
- Keep list items compact and scannable.

Recording Detail:
- Header with editable recording title, status badge, metadata line, and action toolbar.
- Action toolbar should include Export, Regenerate, Retry, More, and Delete.
- Use icons where appropriate and keep destructive action visually distinct.
- Below header, use tabs: Summary, Mind Map, Transcript, Metadata.

Summary tab:
- Show structured summary content with clear headings.
- Include copy and export actions near the content.

Mind Map tab:
- Show a readable expandable tree-style mind map.
- The mind map should feel like a knowledge structure, not a decorative illustration.
- Include actions for copy markdown, export markdown, and export JSON.

Transcript tab:
- Show transcript segments with timestamp and speaker placeholders.
- Include copy and export actions.

Metadata tab:
- Show technical metadata in a compact, readable layout: recording id, workspace id, provider, model, duration, file info, workflow state, created and updated time.

Upload flow:
- Design an upload drawer or modal opened from the workspace.
- Fields: title, workflow type, language, audio file.
- After upload, the new recording should appear selected and enter processing state.

Processing state:
- Show a stepper with these steps: Uploaded, Processing, Transcribing, Summarizing, Generating mind map, Completed.
- Include pending, active, completed, and failed step appearances.

Failed state:
- Show failed step, failure reason, Retry button, and a path to metadata details.

Delete confirmation:
- Design a confirmation dialog explaining that recording data and generated results will be deleted, artifacts will be deleted from storage, and workflow history may remain according to retention policy.

Mobile:
- Create responsive mobile screens where Library and Detail are separate screens.
- Detail screen has a back button and horizontally scrollable tabs.
- Upload can be a full-screen dialog or bottom sheet.

Screens to produce:
1. Desktop library with several recordings selected.
2. Desktop empty library state.
3. Desktop recording detail on Summary tab.
4. Desktop recording detail on Mind Map tab.
5. Desktop processing state with stepper.
6. Desktop failed state.
7. Upload drawer or modal.
8. Delete confirmation dialog.
9. Export and Regenerate menus.
10. Mobile library screen.
11. Mobile detail screen.

Use realistic but fake content. Do not include any real private transcript, real customer name, API key, token, or secret.
```
