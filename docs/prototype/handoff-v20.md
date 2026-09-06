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

## V24 follow-up

- Identity: #183，同一原型分支；Base `79be77e`，Head 为此增补所在提交。
- Outcome: 企业与个人头像水平居中；消除个人按钮默认内边距造成的 2px 偏移。
- Changed surfaces: HTML 缓存版本、CSS、PRD；无协议/迁移/依赖变化。
- Verification: `git diff --check`、`make verify` 通过；浏览器 DOM 实测两个头像展开态中心均约61.58px、折叠态均约33.66px。
- Security/data: 无变化。
- Risks/Next: 原生宿主仍待验证；当前原型无阻塞。

## V25 follow-up

- Identity: #183，同一产品原型分支；Base `c79ab98`，Head 为此增补所在提交。
- Outcome: 应用图标不再叠加微型 `@1`／数字；消息显示提醒点、普通未读加粗、频道行末保留提及数、处理文字旁显示中性色数量。企业和个人入口统一为32px圆角图形、38px点击区及同一中轴。
- Changed surfaces: HTML/CSS/PRD；无协议、迁移或依赖变化，JS行为未变。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify` 通过。浏览器确认两个身份图形尺寸/颜色/圆角/中心一致；88px导航文字与数字无重叠；折叠后数量隐藏、小点和第二栏仍显示。
- Security/data: 无变化；读屏名称保留未读、提及及数量语义。
- Risks: 标记仍为静态示例，未实现真实未读清除；不将本轮样式确认视为此前整栏视觉对齐争议已解决。
- Next: 用户评估统一样式；无技术阻塞。未截图。

## V26 follow-up

- Identity: #183，同一产品原型分支；Base `167c550`，Head 为此增补所在提交。
- Outcome: 三项导航与频道未读共用一个 template 渲染器及 `.nav-signal`，统一行尾圆点；数量留在行标题/无障碍名称。移除旧数字徽标、图标顶部点、私聊行尾在线点；企业及个人入口改左对齐。
- Changed surfaces: HTML/CSS/JS/PRD；无协议/迁移/依赖变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify` 通过。浏览器5个提示均单实例、同色同6px直径、行内垂直居中；折叠后三项提示的垂直偏移均0，距行右侧中心均6px；两个身份入口左边界相同。
- Security/data: 在线与未读语义未混淆；在线信息保留在成员行说明中，不凭空添加私聊未读。状态仍为静态示例。
- Risks/Next: 未实现真实通知清除，待用户评估组件一致性；无阻塞。未截图。

## V27 follow-up

- Identity: #183，同一原型分支；Base `6ee446c`，Head 为此增补所在提交。
- Outcome: 共用组件由绿色点改为中性灰数字，正整数才渲染，超过9显示9+；折叠态图标与数字并排。身份入口左对齐不变。
- Surfaces: HTML/CSS/JS/PRD；无契约、迁移或依赖变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify` 通过；浏览器五个标记均18px、同灰色，折叠后三项图标与标记无重叠。
- Security/data: 无权限变化；示例数字只用于原型，语义由标题说明。
- Risks/Next: 真实已读状态尚未接入；等待用户评估显示方案，无技术阻塞。未截图。

## V28 follow-up

- Identity: #183，同一原型分支；Base `6a13b4f`，Head 为此增补所在提交。
- Outcome: 同一个通知组件支持两层显示：外层三项仅在图标右上圆角显示灰色状态点，第二栏保留行尾数量；企业及个人入口保持左对齐。
- Surfaces: HTML/CSS/JS/PRD；无契约、迁移或依赖变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify` 通过。浏览器确认三项图标框均26px，状态标记均8px、top/right均-2px；展开和折叠规则一致，第二栏仍为228px且显示1、4两个数字徽标。未截图。
- Security/data: 无权限变化；通知状态仍为静态示例，完整语义保留在入口无障碍名称和提示中。
- Risks/Next: 未接入真实未读状态；等待用户评估，无技术阻塞。

## V29 follow-up

- Identity: #183，codex/183-ui-v11；Base `861da28`，Head 为本提交。
- Outcome: 第二栏下限180→112px，默认228px不变；容器查询按栏宽收紧留白、隐藏辅助说明，处理类型移到下一行；保留数量、新建入口及选中项，截断名称提供全文提示。顶部工具区设置220px下限，避免搜索重叠。
- Surfaces: docs/prototype HTML/CSS/JS、产品需求；无协议、迁移、依赖或权限变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify`通过；实际拖拽228→112，三种列表无横向溢出；折叠第一栏后第二栏仍112且待处理数量可见；End达到当前视口动态上限338，双击恢复228，Home恢复112。拉宽后辅助说明恢复；顶部工具与搜索相隔10px。未截图。
- Risks/Next: 原型未完成原生跨平台验证；112px下名称会明显截断，供用户评估最小宽度。无阻塞。

## V30 follow-up

- Identity: #183，codex/183-ui-v11；Base `18ce235`，Head 为本提交。
- Outcome: 外层三项通知改为橙色小点，第二栏去掉数字底色。用户提出数字可否叠在频道标识上；本版保持独立行尾数字，避免覆盖频道类型标识，并使频道与私聊的计数位置一致。
- Surfaces: HTML/CSS、产品需求；无协议、迁移、依赖或权限变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify`通过。浏览器确认三项状态为同色8px橙点、位置不变；两个数字背景透明，频道#保留，计数仍为1和4。未截图。
- Risks/Next: 仅静态原型样式，等待用户评估计数位置；无阻塞。

## V31 follow-up

- Identity: #183，codex/183-ui-v11；Base `4f06ca4`，Head 为本提交。
- Outcome: 第二栏通知数字实际改为深橙色 `#a95113`，无底色；位置、尺寸及外层状态点不变。
- Surfaces: HTML/CSS、产品需求；无契约、迁移、依赖或权限变化。
- Verification: `git diff --check`、`make verify`通过；浏览器两个计数均为rgb(169,81,19)，背景透明。未截图。
- Risks/Next: 静态原型；无阻塞。

## V32 follow-up

- Identity: #183，codex/183-ui-v11；Base `36782c9`，Head 为本提交。
- Outcome: 根据用户对计数距离的反馈，会话列表新增共用图标容器；频道为圆角#图标、私聊沿用头像，数字移至图标右上方。保留深橙文字、无底色，移除行尾计数槽。
- Surfaces: HTML/CSS/JS、产品需求；无契约、迁移、依赖或权限变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify`通过。浏览器确认六个会话图标均23px，两个数字均在图标容器内，行尾数字为0；112px最窄栏下六行无溢出，名称仍有59px空间。未截图。
- Risks/Next: 静态示例，未凭空给私聊添加未读；待用户评估角标位置，无阻塞。

## V33 follow-up

- Identity: #183，codex/183-ui-v11；Base `96eeccd`，Head 为本提交。
- Outcome: 数字恢复行尾最右侧，保留深橙色无底色及会话图标；第二栏默认和双击复位宽度228→180px，112px下限不变。
- Surfaces: HTML/CSS/JS、产品需求；无契约、迁移、依赖或权限变化。
- Verification: `git diff --check`、`node --check docs/prototype/app.js`、`make verify`通过；浏览器初始宽180，两个计数距行右边缘均8px，图标角标为0；112px时无行溢出，双击恢复180。未截图。
- Risks/Next: 静态原型，等待用户评估默认密度；无阻塞。
