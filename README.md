# Threadline

Threadline 是一个 Agent 原生的企业 IM：团队在正常、可靠的企业聊天系统中协作，任何一段对话都可以直接转化为有权限、可观察、可审批的 Agent 任务。

## 产品定位

```text
企业 IM = 组织 + 身份 + 消息 + 文件 + 权限 + 审计
Agent Runtime = Task + Run + Workspace + 工具 + 审批 + 产物
```

Agent 是组织内受治理的正式协作者，不是需要 `@` 才能工作的机器人插件。IM 与 Runtime 相互独立：模型或本地执行器离线时，成员仍可正常沟通；Agent 执行时，只能读取被明确引用和授权的上下文。

## 当前资产

- [产品需求文档](./docs/product-requirements.md)
- [系统架构与通信协议](./docs/architecture/system-architecture.md)
- [P0 服务目录](./docs/architecture/service-catalog.md)
- [Private Enterprise v1.0 交付计划](./docs/delivery-plan.md)
- [Agent 并行工作流](./docs/agent-workstreams.md)
- [产品架构图](./docs/architecture/assets/threadline-product-architecture.svg)
- [P0 部署服务总图](./docs/architecture/assets/threadline-p0-deployed-services.svg)
- [逻辑服务架构图](./docs/architecture/assets/threadline-service-architecture.svg)
- [私有化部署架构图](./docs/architecture/assets/threadline-private-deployment-architecture.svg)
- [统一产品原型](./docs/prototype/index.html)

当前处于产品与架构定义阶段，品牌和视觉系统仍会继续迭代。架构基线采用中心化 IM
协调层与本地 Agent Runtime 分离的部署方式；消息、执行和模型出网分别受独立权限控制。

`docs/prototype/index.html` 是唯一设计入口。它根据视口进入桌面端或移动端，所有产品与交互修改都直接落在 HTML 原型中，不再维护 Figma 版本或独立产品画板。

## 核心产品流

1. 团队成员在 Channel 或私聊中正常沟通。
2. 人或 Agent 将明确的一段讨论转成 Task。
3. Runtime 使用短期 Capability Grant 获取必要消息、文件和工具。
4. 团队实时观察 Run，批准高影响动作，并接收可验证的产物。
5. 消息、任务、授权和执行结果进入同一个组织审计边界。

## 仓库状态

此仓库目前包含 PRD、系统架构与交互原型。下一步需要通过架构评审确认协议契约、威胁模型和
第一阶段部署拓扑，再进入实现。

## License

Licensed under the [Apache License 2.0](./LICENSE).
