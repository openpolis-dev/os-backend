# 提案系统 Metaforo 数据迁移规划

> **文档版本**：v1.0  
> **编写日期**：2026-06-09  
> **关联分支**：`feature/proposal-remove-metaforo`  
> **目标**：在去除 Metaforo 依赖前，将 Metaforo 侧独有数据迁入 OS DB，保证历史提案可继续展示投票与评论  
> **代码基线**：`internal_inject/proposal`（当前线上提案 API 实现路径）

---

## 1. 背景

Metaforo 论坛服务已停止维护。当前 Proposal V2 采用 **OS Backend + Metaforo** 双系统：

- **OS DB**：提案元数据、状态机、正文块、组件、投票结构（poll/option）、部分用户票记录、部分评论
- **Metaforo**：投票明细（wallet + weight）、评论树展示、poll 实时计票、活动流

Metaforo 下线后，以下接口将不可用：

| 接口 | Metaforo 依赖 |
|------|----------------|
| `GET /proposals/show/:id` | `GetProposal` → votes、comments |
| `GET /proposals/vote_detail/:vote_option_id` | `GetVoterList` |
| `POST /proposals/vote/:id` 等写操作 | CastVote / AddComment 等 |
| `GET /user/metaforo_activities` | UserActivities |
| 定时任务 `RefreshVotingProposalInfoJob` | 轮询 GetProposal 同步状态 |

**迁移目标不是「整包重拉提案」**，而是把 Metaforo 作为真源、OS 缺失或不完整的数据补进本地 DB。

---

## 2. 存量数据盘点（OS 已有 vs 需迁移）

### 2.1 已在 OS DB、无需从 Metaforo 重拉

| 数据 | 表 / 字段 | 说明 |
|------|-----------|------|
| 提案主记录 | `proposals` | 含 `state`、`sip`、`title`、`vote_type`、版本（`proposal_record_id` + `version`） |
| 正文 / 组件 | `proposal_content_blocks`、`proposal_component_records` | 详情静态内容 |
| 投票轮次 / 选项结构 | `proposal_vote_records`、`proposal_vote_option_records` | 含 `metaforo_id` 映射 |
| 选项聚合票数 | `proposal_vote_option_records.voter_count` | 曾由 Metaforo 同步，可作迁移校验基准 |
| 驳回评论正文 | `proposal_comments`（`is_reject_comment=true`） | 正文在 OS，展示 ID 曾绑 Metaforo |
| 编辑历史 | 同 `proposal_record_id` 多版本 `proposals` | 本地 SQL，不依赖 Metaforo |
| 列表页 | `GET /proposals/list` | 纯 OS SQL，**当前仍可用** |

### 2.2 必须从 Metaforo（或备份）迁入

| 数据 | 优先级 | 目标表 | 现状问题 |
|------|--------|--------|----------|
| 用户投票明细（wallet + weight + option + ts） | **P0** | `proposal_user_vote_records`（扩展 `weight`） | 仅部分提案有记录；无 `weight` 字段 |
| 评论全文 + 树结构 | **P0** | `proposal_comments` | 展示以 Metaforo posts 为准，OS 记录不完整 |
| 选项 `voter_count` / poll 状态校验 | **P0** | 已有字段，迁移后重算或校对 | 与明细汇总应一致 |
| 用户活动流 | P2 | 新建 `user_proposal_activities` | 可选，近 N 月即可 |
| Metaforo 用户映射 | P1 | `metaforo_users` | 已有部分数据，迁移期保留只读 |

### 2.3 标识与映射关系

```
proposals.proposal_record_id  = "metaforo:{thread_id}"
proposals.GetMetaforoThreadId() → thread_id

proposal_vote_records.metaforo_id     → Metaforo poll id
proposal_vote_option_records.metaforo_id → Metaforo poll option id（vote_detail 路径参数）

proposal_comments.metaforo_comment_id → Metaforo post id
proposal_comments.parent_id           → OS 父评论 id（迁移时需建树）
```

**`vote_detail` 的 `:vote_option_id` 是 Metaforo option id**（`proposal_vote_option_records.metaforo_id`），不是 OS 自增 id。

---

## 3. 迁移原则

1. **先扩展 schema，再导数据，再切换读路径** — 不与代码改造完全串行等待  
2. **保留 legacy 字段** — `metaforo_id`、`metaforo_comment_id` 迁移期只读保留，便于对账  
3. **幂等导入** — 脚本可重复执行（`ON CONFLICT DO NOTHING` 或按唯一键 upsert）  
4. **可降级** — 无 Metaforo 备份时，旧提案票/评论允许只读缺失，新提案全走 OS  
5. **校验驱动** — 按 SIP 抽样对比 `voter_count`、评论数、选民数  

---

## 4. Schema 变更（迁移前 / 与改造同期）

### 4.1 `proposal_user_vote_records` 扩展

当前结构（无 weight）：

```go
// internal/model/proposal.go
type ProposalUserVoteRecord struct {
    UserWallet                 string
    ProposalID                 uint
    ProposalVoteOptionRecordId uint
    VoteTs                     int64
}
```

**建议新增**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `weight` | `decimal` / `int` | 投票权重，与 Metaforo `UserPollRecord.Weight` 对齐 |
| `metaforo_poll_option_id` | `int` nullable | 迁移对账用（可选，也可用 option 表 join） |

唯一约束保持不变：`UNIQUE(user_wallet, proposal_id, proposal_vote_option_record_id)`（多选场景）。

### 4.2 `proposal_comments` 补全

已有字段基本够用，迁移时确保：

- `parent_id` 正确（替代 `reply_metaforo_post_id` 业务语义）
- `author_wallet`、`content`、`create_ts`、`is_deleted`（软删）
- 保留 `metaforo_comment_id` 作 legacy 映射

### 4.3 新建 `user_proposal_activities`（P2，可与主改造并行）

| 字段 | 说明 |
|------|------|
| `wallet` | 活动所属用户 |
| `action_type` | create / comment / vote / share |
| `proposal_id` | |
| `target_title` | 标题快照 |
| `reply_to_wallet` | nullable |
| `action_ts` | Unix 时间戳 |

---

## 5. 数据来源与导出

### 5.1 优先顺序

| 来源 | 适用 | 说明 |
|------|------|------|
| Metaforo DB 备份 / 运维导出 | **最佳** | 完整 posts、poll votes、users |
| Metaforo API 批量脚本（若仍可达） | 次选 | `GetProposal`、`GetVoterList` 分页拉取 |
| OS DB 已有数据 | 兜底 | `proposal_user_vote_records`、`proposal_comments` 不完整 |

### 5.2 API 导出参考（历史实现）

与线上一致的 Metaforo 调用：

| 用途 | API | 代码位置 |
|------|-----|----------|
| 提案 thread + polls + posts | `GET /api/get_thread/{threadId}` | `metaforo.GetProposal` |
| 某选项选民列表 | `POST /api/poll/list`（`option_id`, `page`） | `metaforo.GetVoterList` |
| 用户 profile | `GET /api/profile/{userId}` | `metaforo.UserDetail` |

导出脚本建议：

1. 从 OS 查出所有 `proposal_record_id LIKE 'metaforo:%'` 的 thread id  
2. 对每个 thread 调 `GetProposal`（或读备份）  
3. 对每个 `proposal_vote_option_records.metaforo_id` 分页调 `GetVoterList` 直至 `< 10` 条/页  
4. 将 Metaforo `user_id` → wallet 写入/更新 `metaforo_users`  

### 5.3 待运维确认清单

- [ ] Metaforo 是否有最终 DB dump？格式？存放位置？  
- [ ] `group_name` / `group_id` 与生产配置是否一致（`system_variables` 中 Metaforo 配置）  
- [ ] 历史提案是否要求投票记录 100% 可溯？  
- [ ] 权重是否与 Metaforo 时期必须一致？（影响 `weight` 字段写入规则）

---

## 6. 字段映射表

### 6.1 投票明细 → `proposal_user_vote_records`

| Metaforo / 导出 | OS 字段 | 转换规则 |
|-----------------|---------|----------|
| `poll_option_id` | `proposal_vote_option_record_id` | 通过 `proposal_vote_option_records.metaforo_id = poll_option_id` 查 OS id |
| `user.web3_public_key` / profile | `user_wallet` | `common.FormatUserWallet` |
| `weight` | `weight`（新增） | 直接写入 |
| `created_at` | `vote_ts` | Unix 时间戳 |
| thread → proposal | `proposal_id` | `proposals.id`（取该 thread 对应 **max version** 记录，与列表逻辑一致） |

### 6.2 评论 → `proposal_comments`

| Metaforo post | OS 字段 | 转换规则 |
|---------------|---------|----------|
| `id` | `metaforo_comment_id` | 保留 |
| `content` | `content` | Quill JSON 字符串 |
| `reply_pid` / parent | `parent_id` | 先导入顶级，再第二遍填 parent（post id → OS comment id 映射表） |
| `user.web3_public_keys[0]` | `author_wallet` | |
| `created_at` | `create_ts` | |
| `deleted` | 软删标记 | 按业务增 `is_deleted` 或 `is_hidden` |
| thread | `proposal_id`、`proposal_record_id` | 关联提案 |

### 6.3 Legacy 映射表（建议迁移脚本维护）

```sql
-- 可选：独立对账表，迁移完成后归档
CREATE TABLE IF NOT EXISTS legacy_metaforo_mappings (
    id              BIGSERIAL PRIMARY KEY,
    entity_type     VARCHAR(32) NOT NULL,  -- 'comment' | 'vote_option' | 'poll'
    metaforo_id     BIGINT NOT NULL,
    os_id           BIGINT NOT NULL,
    proposal_id     BIGINT,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(entity_type, metaforo_id)
);
```

---

## 7. 迁移阶段与排期

与代码改造 **并行**，不建议「全部迁完再改代码」。

| 阶段 | 内容 | 产出 | 预估 |
|------|------|------|------|
| **M0 摸底** | 跑 §8 SQL、确认备份、定 weight 规则 | 摸底报告 | 1–2 天 |
| **M1 Schema** | `weight` 等字段 migration | DB migration 脚本 | 1 天 |
| **M2 导出** | 从备份/API 导出 JSON/CSV | `export/` 原始数据 | 2–5 天（依赖备份） |
| **M3 导入** | 投票明细 + 评论 + 用户映射 | import 脚本 + 执行日志 | 3–5 天 |
| **M4 校验** | 抽样 + 全量计数对比 | 校验报告 | 1–2 天 |
| **M5 切换** | 读路径改 DB；Metaforo 只读 fallback 可选 | 与代码改造 PR 合并 | 与 P2/P3 同步 |

**代码改造分支**：`feature/proposal-remove-metaforo`（写路径 → 读路径 → 清理 Metaforo SDK）。

---

## 8. 迁移前摸底 SQL

在**生产库只读副本**或 staging 执行：

```sql
-- 绑定了 Metaforo thread 的提案版本数
SELECT COUNT(*) AS metaforo_linked_rows
FROM proposals
WHERE proposal_record_id LIKE 'metaforo:%';

-- 逻辑提案数（按 record_id 去重）
SELECT COUNT(DISTINCT proposal_record_id)
FROM proposals
WHERE proposal_record_id LIKE 'metaforo:%';

-- 已有用户投票记录的提案数
SELECT COUNT(DISTINCT proposal_id) FROM proposal_user_vote_records;

-- 投票已结束但无用户票记录的提案（迁移缺口候选）
SELECT p.id, p.sip, p.state
FROM proposals p
JOIN (
    SELECT proposal_record_id, MAX(version) AS max_version
    FROM proposals GROUP BY proposal_record_id
) t ON p.proposal_record_id = t.proposal_record_id AND p.version = t.max_version
WHERE p.proposal_record_id LIKE 'metaforo:%'
  AND p.state IN (6, 7, 8, 9)  -- vote_passed, vote_failed, pending_execution, executed 等，按实际调整
  AND p.id NOT IN (SELECT DISTINCT proposal_id FROM proposal_user_vote_records);

-- 本地评论条数
SELECT COUNT(*) FROM proposal_comments;

-- 有 Metaforo option id 的投票选项数（vote_detail 依赖）
SELECT COUNT(*) FROM proposal_vote_option_records WHERE metaforo_id != 0;

-- 选项票数总和 vs 用户票记录数（抽样诊断）
SELECT pvo.proposal_id,
       SUM(pvo.voter_count) AS sum_voter_count,
       (SELECT COUNT(*) FROM proposal_user_vote_records u WHERE u.proposal_id = pvo.proposal_id) AS user_record_count
FROM proposal_vote_option_records pvo
GROUP BY pvo.proposal_id
HAVING SUM(pvo.voter_count) > 0
ORDER BY sum_voter_count DESC
LIMIT 20;
```

---

## 9. 导入流程（建议脚本顺序）

```
1. 冻结 Metaforo 写入（已完成）
2. 导出原始数据 → data/export/{threads,votes,comments,users}/
3. 导入 metaforo_users + users（wallet 映射）
4. 导入 proposal_comments（两遍：顶级 → 回复）
5. 导入 proposal_user_vote_records + weight
6. 重算 proposal_vote_option_records.voter_count（SUM by option）并与 Metaforo 导出对比
7. 写入 legacy_metaforo_mappings
8. 抽样人工验收（见 §10）
9. 切换 API 读 DB（feature 分支代码）
10. 观察期后下线 Metaforo 调用
```

**幂等性**：导入脚本以 `(proposal_id, user_wallet, proposal_vote_option_record_id)` 及 `(metaforo_comment_id)` 为唯一键 upsert。

---

## 10. 验收标准

| # | 场景 | 预期 |
|---|------|------|
| 1 | 随机抽 10 个已结束 SIP 提案 | `SUM(voter_count)` 与导入明细 COUNT 误差在约定范围内（如 ±0） |
| 2 | 同一提案 `vote_detail`（按 metaforo option id） | 与迁移后 DB 查询结果一致 |
| 3 | 评论数 | `comment_count` ≥ OS `proposal_comments` 非删计数 |
| 4 | 评论树 | 随机 5 条回复链 parent 正确 |
| 5 | 无备份提案 | 列表/正文可看；票/评论缺失有明确降级（不 500） |
| 6 | 重复执行 import | 不产生 duplicate key，数据不变 |

---

## 11. 无 Metaforo 备份时的降级策略

| 数据 | 策略 |
|------|------|
| 提案正文 / 状态 / SIP | 继续用 OS DB（列表、详情静态部分可用） |
| 投票明细 | 仅保留 OS 已有 `proposal_user_vote_records` + `voter_count` 快照 |
| 评论 | 仅 OS `proposal_comments` 已有记录 |
| 用户操作 | 旧提案标记 **archived**，禁止新投票/评论；新提案走 OS 全链路 |

---

## 12. 与代码改造的衔接

| 迁移完成项 | 解锁的代码改造 |
|------------|----------------|
| M1 Schema（weight） | `CastVote` 写 OS 时可落 weight |
| M3 投票 + 评论导入 | `show` / `vote_detail` 改读 DB |
| M4 校验通过 | 生产切换读路径 |
| — | 新提案 `PublishProposal` 不再调 `SaveProposalToMetaforo` |

**勿依赖 Phase A「忽略 metaforo_access_token」**：Metaforo 已不可用时，必须先有 DB 数据 + 读路径改造。

---

## 13. 风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| 无 Metaforo 备份 | 历史票/评论永久缺失 | 尽早确认备份；降级 + 沟通 |
| weight 规则未定义 | 计票/积分不一致 | M0 产品确认；写入时落库 |
| option id 映射错误 | vote_detail 查错人 | 严格用 `metaforo_id` 映射表校验 |
| 多 version 提案 | 票/评论挂错 proposal_id | 统一挂 **max version** 的 `proposals.id` |
| 导入与线上写并发 | 数据冲突 | 维护窗口冻结提案写操作 |

---

## 14. 后续待办（Issue 可拆）

- [ ] 确认 Metaforo 备份可用性与格式  
- [ ] 编写 `cmd/migrate_metaforo/` 或 `_scripts/metaforo_import/`  
- [ ] GORM / SQL migration：`proposal_user_vote_records.weight`  
- [ ] 迁移执行与校验报告模板  
- [ ] 与前端对齐：`PollRecord` JSON 兼容 spec（读 DB 组装）  

---

## 15. 修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| v1.0 | 2026-06-09 | 初版；基于 os-backend 代码梳理与迁移讨论 |
