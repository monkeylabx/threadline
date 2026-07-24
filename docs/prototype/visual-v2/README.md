# Threadline 移动端渲染器

这是统一产品原型在窄视口下的移动端实现，由 `../index.html` 自动进入。产品结构与文案以 `docs/prototype/` 为准，不单独形成另一套设计源。

打开 `index.html` 后可通过导航或页面内按钮切换七个可深链接场景：

- `?view=messages`：移动端消息根页
- `?view=search`：移动端聚焦搜索
- `?view=activity`：移动端活动与待处理事项
- `?view=profile`：移动端身份、组织与 Runtime
- `?view=channel`：频道工作台
- `?view=task`：Agent 任务 Activity Sheet
- `?view=approval`：一次性权限审批 Sheet

视觉方向：macOS 原生工作台、轻玻璃侧栏、清晰内容层、蓝色主交互、薄荷绿与琥珀色状态，以及嵌入对话流的 Agent 执行现场。

移动端采用明确的推进层级：`消息 -> 频道 -> 任务 -> 审批`。底部主导航只出现在消息、活动和我的三个根页面；进入频道及其详情后使用顶部返回，不再叠加抽屉或第二套导航。
