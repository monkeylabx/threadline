# V33 产品价值与文档一致性审查

日期：2026-09-06。范围：用户确认V33后要求提交PR，并扫描工程文档，而非只写PR说明。
审查输入：V33 `8d9b8d4`、主分支 `e2f06d6`，合并保留#185成果审查页；产品价值首轮补充 `6159663`。

## 方法与边界

- 清点仓库自有72份Markdown文档（审查新增文件之前），覆盖根目录、docs、apps、packages、crates、deploy、db、test、proto和spikes。
- 全文检索产品定位、Channel/Task可见性、私人工作、Artifact、导航、Scope和证据措辞；重点逐段核对PRD、四份既有ADR、验收、安全边界、架构和设计文档。
- 对照Task Protobuf的父Channel/DM与Task Thread关系，以及原型HTML/JS；不把UI标签当作后端事实。
- evidence文件检查其目标SHA/用途和引用关系，不改生成物、证据状态、哈希或签字。没有重跑密码、FFI、codegen、真机或完整A11Y验证。
- 排除第三方/vendor/node_modules及Agent技能本体；这不是逐行代码审计、竞品重新调研或对全部历史实现证据的复验。

## 结论与落点

| 问题 | 处理 | 文档 |
| --- | --- | --- |
| 首页偏向治理/委派任务，缺少IM+个人AI工作及团队接手的价值 | 改为人际沟通、私人工作、选定成果共享；商业采用仍为待验证假设 | README、PRD |
| 私人工作容易被理解为共享Task Thread中的隐藏分组 | 新增Private Work/Artifact Publication词汇及Proposed边界ADR；保留父会话密码受众规则 | CONTEXT、ADR-0005 |
| PRD把历次导航描述和现行规范混在一起，旧收件箱与独立目录被读成常驻入口 | 新增V33现行摘要、标记历史探索，更新消息/工作/处理及顶部搜索层级 | PRD、prototype/README、prototype-state-matrix |
| 既有AC-006被误认为覆盖私人工作 | 保留冻结共享Task基线，新增PW-01–PW-08提案及实施前置项，生产验收NOT RUN | acceptance、quality/test-plan、delivery-plan |
| “团队可观察”缺少共享/私人限定 | 明确来源引用不是公开授权，受众/来源分享策略/审计最小化须单独验证 | PRD、security/data-classification、security/trust-boundaries |
| 早期架构草案仍是六工作负载、Agent Device入群、Managed Encryption双模式 | 以既有ADR纠正文句和数量，明确旧图未重绘且不是现行实施依据 | system-architecture、service-catalog |
| 调研建议、旧截图和handoff可能被误读为当前规范 | 保留历史结果，增加适用版本说明与现行入口 | research通知/导航记录、handoff-v19/v20、PRD |
| 旧A11Y FAIL/HOLD可能因界面获认可而被隐式清除 | 标明绑定旧SHA，V33局部交互检查不替代重新审计 | quality/prototype-accessibility |

## 已检查但不因V33改写

- ADR-0001客户端平台、ADR-0002服务/协议、ADR-0003密码受众、ADR-0004密码库候选：保留各自状态；未借UI确认批准密码或持久化设计。
- architecture中的Capability签名、Audit/Retention、Outbox契约/策略：不改已冻结字段与事务规则。私人发布的新增契约仍未定义。
- contracts、integration/t014、proto/README、测试fixture/evidence和spikes技术报告：它们证明特定提交的技术结果，不是产品采用或私人工作实现证据，原记录不变。
- build、development、deploy、db、工具链/SQLCipher研究、各应用/包/FFI说明：本次布局和产品边界不要求更换技术栈、依赖、存储或部署配置。
- agents流程、CONTRIBUTING、handoff模板：仍适用；没有把计划冻结或原型批准改成代码验收。
- security/threat-model及risk-register：既有受众越权/Prompt泄露风险仍有效，未将风险改为关闭；私人发布的新测试要求另列提案。
- design/tokens：V33仍是一次性HTML原型，不把探索用的局部CSS宣称为跨平台Token已实现。
- 两篇商业/Slack调研：保留带日期的来源与假设，不把国内产品描述成“只有文字机器人”，不声称市场和付费已验证。

## 尚需后续工程，不在本PR伪造完成

Private Work持久化/恢复与多设备、Publication版本/受众/幂等契约、来源撤权、私人状态与团队投影隔离、
原生窗口/移动端同等体验、完整A11Y重验，以及老架构图重绘。Owner和前置条件见ADR-0005与PW场景；
这些项不阻止静态原型和文档评审，但阻止对应功能宣称生产可用。

## 本PR验证

- V33浏览器导航、双栏缩放、数字/状态层级局部检查，见版本handoff。
- `git diff --check`、`node --check docs/prototype/app.js`、`node --check docs/prototype/mobile/app.js`、`make verify`。
- 合并冲突保留#185的默认Artifact审查、移动端入口，以及V33融合原型；未修改既有证据JSON/Golden/SDK/数据库契约。
- 本文不是新的性能、安全、无障碍或商业验证证据包。
