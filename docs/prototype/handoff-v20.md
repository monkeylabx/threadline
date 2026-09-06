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
