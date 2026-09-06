# V20 折叠范围交接

## Identity

- Issue: #183
- Workstream: 产品 HTML 原型
- Branch: `codex/183-ui-v11`
- Base commit: `ab73958`
- Head commit: 本文所在提交（`git log -1 -- docs/prototype/handoff-v20.md`）

## Outcome

- Completed: 折叠只收起第一栏文字，第二栏列表及当前位置始终保留；保留企业、个人头像、角标、悬停和读屏名称。
- Not completed: 原生客户端接入及真实会话后端（沿用原型边界）。
- Acceptance status: 通过，无截图。

## Changed Surfaces

- Owned paths: `docs/prototype/`、`docs/product-requirements.md`。
- Contract / migration / generated / lockfile changes: 无。

## Verification

```text
node --check docs/prototype/app.js — PASS
git diff --check — PASS
make verify — PASS (51 required surfaces)
浏览器：收起→第二栏仍可见（宽度约 228px）、第一栏文字隐藏 — PASS
浏览器：收起时切换消息→展开→收起，频道列表及栏目保持 — PASS
```

## Security And Data

- 无权限、数据、日志或遥测变化；切换仅改变 CSS 布局，不清空消息或草稿。

## Risks And Decisions

- V19 隐藏第二栏的设计被用户否定；V20 将折叠契约限定为第一栏文字。
- 原生窗口控件验证仍未完成，本次不改变该边界。

## Next

- Issues unblocked: 无新增。
- Blocking condition: 无。
- Recommended next task: 用户确认第一栏紧凑状态。

## V21 follow-up

- Identity: #183，同一原型工作流及分支；Base `383bf23`，Head 为此增补所在提交。
- Outcome: 展开态改为 144px 横排图标文字导航，收起为 48px；企业名称随之隐藏。第二栏始终 228px。
- Changed surfaces: HTML、CSS、PRD；无协议、迁移或依赖变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify` 均通过。
- Browser: 展开三栏 `144 / 228 / 390`，收起 `48 / 228 / 486`；主内容增宽 96px，频道列表保持可见。
- Security/data: 无变化。
- Risks: V20 仅缩短 10px，不足以表达有意义的折叠；原生窗口接入仍未完成。
- Next: 用户确认展开／收起两态；无阻塞。

## V22 follow-up

- Identity: #183，同一原型分支；Base `335bf5c`，Head 为此增补所在提交。
- Outcome: 第二栏右边界可拖拽；默认 228px、最小 180px、最大 360px（受主内容 320px 保留空间约束）；支持键盘、双击复位和拖拽取消。
- Changed surfaces: HTML、CSS、JS、PRD；无协议、迁移或依赖变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify` 通过。
- Browser: 真实拖拽 228→278；Right→288；折叠第一栏并切换消息仍为288；End→360、Home→180、双击→228，全部通过。未截图。
- Security/data: 宽度仅存内存，无新增数据访问或持久化。
- Risks: Escape/系统取消处理已实现但未自动化触发验证；窗口极窄时仍依赖现有移动端路由，未完成跨平台原生验证。
- Next: 用户试用拖拽手感；无阻塞。

## V23 follow-up

- Identity: #183，同一原型分支；Base `c78e21d`，Head 为此增补所在提交。
- Outcome: 第一栏默认 104px，仅显示企业 Logo；文字应用导航保留。第一栏展开时可独立拖拽 88–160px，折叠仍为 48px。
- Changed surfaces: HTML、CSS、JS、PRD；两栏共用拖拽行为，无协议/迁移/依赖变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify` 通过。
- Browser: 第一栏拖拽104→130、第二栏228→258；折叠再展开两者保留；分别双击恢复104/228，全部通过。未截图。
- Security/data: 内存偏好，无新增访问或持久化。
- Risks: 原生窗口集成、极窄桌面视口仍待验证。
- Next: 用户确认默认比例；无阻塞。
