# AI 生成代码的可审计性与编码规则调研

状态：工程质量政策输入，尚未实施 CI 门禁

调研快照：2026-08-30

范围：类/文件大小、函数/方法长度、复杂度、参数和嵌套、单元测试，以及 AI 生成代码的审计约束。事实部分只引用官方项目、官方文档、规范或源码。

## 结论

用户记得的“阿里、数据库和类定义规则”几乎可以确定是阿里官方 [alibaba/p3c](https://github.com/alibaba/p3c)，即《阿里巴巴 Java 开发手册》。官方仓库同时包含手册、PMD 实现和 IDEA/Eclipse 插件；当前仓库标注的最新版是 2022-02-03 发布的[黄山版 PDF](https://github.com/alibaba/p3c/blob/master/Java%E5%BC%80%E5%8F%91%E6%89%8B%E5%86%8C%28%E9%BB%84%E5%B1%B1%E7%89%88%29.pdf)。

它确实覆盖 OOP、单元测试、工程结构，以及 MySQL 建表、索引、SQL 和 ORM。它还明确建议单个方法不超过 80 行，并有真实 PMD 实现。但它没有给 class 总行数、class 方法数或圈复杂度的数字上限。因此，p3c 很适合当 Java/MySQL 基础线，却不能单独解决 AI 写出“自己也无法审计”的大类和复杂方法。

更稳妥的门禁不是只数物理行，而是同时限制：

- 函数有效行数、认知/圈复杂度、嵌套深度和参数数；
- 类的职责、内聚性、公开接口数量和总复杂度；
- 新代码测试覆盖、关键分支、错误路径和重复率；
- lint 例外、测试跳过和生成代码排除必须可追溯；
- 高风险代码必须有人类审查，模型审查不能替代责任人。

## 一方资料中的明确规则

### 阿里巴巴 Java 开发手册 / p3c

- [p3c README](https://github.com/alibaba/p3c/blob/master/README.md) 说明该项目源于阿里技术团队的 Java 编码实践，关注数据库结构/索引缺陷、混乱代码结构和安全问题。
- 官方[旧 GitBook 目录](https://github.com/alibaba/p3c/blob/master/p3c-gitbook/SUMMARY.md)可用于导航，能看到 OOP、单元测试、MySQL 和工程结构等完整范围；但[旧 GitBook README](https://github.com/alibaba/p3c/blob/master/p3c-gitbook/README.md)明确警告其内容已与最新版手册不一致，所以最终应以黄山版 PDF 和规则源码为准。
- 黄山版建议单个方法总行数不超过 80 行。官方 [MethodTooLongRule 源码](https://github.com/alibaba/p3c/blob/master/p3c-pmd/src/main/java/com/alibaba/p3c/pmd/lang/java/rule/other/MethodTooLongRule.java#L42-L52)将最大值设为 80；[判断实现](https://github.com/alibaba/p3c/blob/master/p3c-pmd/src/main/java/com/alibaba/p3c/pmd/lang/java/rule/other/MethodTooLongRule.java#L82-L96)扣除注释行。旧英文 p3c-pmd README 对注释计数的文字与最新版 PDF/源码不一致，不应采用旧描述。
- 手册建议 if/else 分支不要超过三层，超过时考虑卫语句、策略模式或状态模式；复杂条件应先赋给有意义的布尔变量。它没有 McCabe/圈复杂度数字阈值。
- OOP 规则涉及类名、Override、访问控制、POJO/DO 类型、构造器和 getter/setter 的职责等，但没有 class 最大行数或最大方法数。不能把“80 行”误说成类限制。
- 单测要求 AIR：Automatic、Independent、Repeatable；必须有断言、不可依赖人工查看输出、测试之间不依赖顺序，并隔离网络/服务/中间件。手册推荐普通代码语句覆盖率 70%，核心模块语句和分支覆盖率 100%，并使用 BCDE（边界、正确、设计、错误）设计用例。
- 数据库规则包括小写下划线命名、索引命名、禁止 float/double 表示精确小数、禁止超过三表 JOIN、禁止存储过程和外键级联、禁止 SELECT 星号、参数绑定防注入，以及数据订正前先 SELECT 等。可从官方旧导航查看[建表](https://github.com/alibaba/p3c/blob/master/p3c-gitbook/MySQL%E6%95%B0%E6%8D%AE%E5%BA%93/%E5%BB%BA%E8%A1%A8%E8%A7%84%E7%BA%A6.md)、[索引](https://github.com/alibaba/p3c/blob/master/p3c-gitbook/MySQL%E6%95%B0%E6%8D%AE%E5%BA%93/%E7%B4%A2%E5%BC%95%E8%A7%84%E7%BA%A6.md)、[SQL](https://github.com/alibaba/p3c/blob/master/p3c-gitbook/MySQL%E6%95%B0%E6%8D%AE%E5%BA%93/SQL%E8%AF%AD%E5%8F%A5.md)和[ORM](https://github.com/alibaba/p3c/blob/master/p3c-gitbook/MySQL%E6%95%B0%E6%8D%AE%E5%BA%93/ORM%E6%98%A0%E5%B0%84.md)，但具体执行仍以最新版 PDF 为准。

### 通用规则与数值阈值

| 官方来源 | 明确阈值或规则 | 正确解读 |
| --- | --- | --- |
| [Google C++ Style Guide](https://google.github.io/styleguide/cppguide.html#Write_Short_Functions) | 函数超过约 40 行时考虑拆分，但明确不设硬上限；公开声明处的函数体通常只放约 10 行以内 | 40 是审查触发线，不是普世硬限制 |
| [ESLint max-lines-per-function](https://eslint.org/docs/latest/rules/max-lines-per-function) | 默认 50 行 | 应与复杂度、语句数同时使用 |
| [ESLint max-lines](https://eslint.org/docs/latest/rules/max-lines) | 文件默认 300 行；文档称常见建议为 100–500 行 | 文件大小不等于类的职责是否合理 |
| [ESLint complexity](https://eslint.org/docs/latest/rules/complexity) | 圈复杂度默认上限 20 | 默认偏宽；规则需显式启用 |
| [ESLint max-depth](https://eslint.org/docs/latest/rules/max-depth) | 嵌套深度默认 4 | 可阻止通过压缩行数规避复杂度 |
| [ESLint max-params](https://eslint.org/docs/latest/rules/max-params) | 参数默认 3 | DTO、构造器和边界适配器需按语义例外 |
| [PMD Java Design Rules](https://docs.pmd-code.org/latest/pmd_rules_java_design.html) | 认知复杂度 15；方法圈复杂度 10；类总圈复杂度 80；if 嵌套 3；NCSS 方法 60、类 1500 | 都是可配置默认值；类 1500 NCSS 只是极大类报警，不是理想类大小 |
| [SonarJava S3776 源码](https://github.com/SonarSource/sonar-java/blob/master/java-checks/src/main/java/org/sonar/java/checks/CognitiveComplexityMethodCheck.java) | 方法认知复杂度默认最大 15 | 与 PMD/Detekt 的 15 形成跨工具一致性 |

PMD 官方还提供了重要反例：它已淘汰按物理行数判断 Java 类/方法的旧规则，因为物理行数受格式影响、可以通过一行塞多条语句规避；官方建议使用 NCSS。见 [PMD #2127](https://github.com/pmd/pmd/issues/2127) 和当前 [NcssCount](https://docs.pmd-code.org/latest/pmd_rules_java_design.html#ncsscount)。

## Threadline 五种语言的可落地工具

| 语言 | 官方工具中的相关能力 |
| --- | --- |
| TypeScript / JavaScript | ESLint 可配置函数 50 行、文件 300 行、复杂度 20、嵌套 4、参数 3；这些规则并非全部由推荐配置自动开启，项目必须显式配置 |
| Rust | [Clippy too_many_lines](https://rust-lang.github.io/rust-clippy/master/#too_many_lines) 默认 100 行且属于 opt-in pedantic；[too_many_arguments](https://rust-lang.github.io/rust-clippy/master/#too_many_arguments) 默认 7；[excessive_nesting](https://rust-lang.github.io/rust-clippy/master/#excessive_nesting) 必须配置非零阈值才会报告。Clippy 官方不建议把它的旧 cognitive_complexity lint 当成真正的认知复杂度，推荐优先用长度和嵌套 |
| Go | [golangci-lint 官方配置](https://golangci-lint.run/docs/linters/configuration/) 中 funlen 默认 60 行/40 语句，cyclop 默认最大复杂度 10；需要显式启用相关 linter |
| Swift | SwiftLint 默认启用：[function_body_length](https://realm.github.io/SwiftLint/function_body_length.html) 警告 50/错误 100；[type_body_length](https://realm.github.io/SwiftLint/type_body_length.html) 250/350；[file_length](https://realm.github.io/SwiftLint/file_length.html) 400/1000；[cyclomatic_complexity](https://realm.github.io/SwiftLint/cyclomatic_complexity.html) 10/20；[function_parameter_count](https://realm.github.io/SwiftLint/function_parameter_count.html) 5/8 |
| Kotlin | [Detekt Complexity Rules](https://detekt.dev/docs/rules/complexity/) 提供认知复杂度 15、嵌套 4、每类/文件等 11 个函数的默认值；认知复杂度规则默认不激活，需显式开启 |

## 单元测试与 AI 代码质量门禁

- [SonarQube AI Code Assurance](https://docs.sonarsource.com/sonarqube-server/quality-standards-administration/ai-code-assurance/quality-gates-for-ai-code) 是找到的最直接、专门面向 AI 代码的官方门禁：新代码无新 issue、100% 复核新 Security Hotspot、测试覆盖率至少 80%、重复率最多 3%；同时对整体代码要求安全评级 A、Security Hotspot 全部复核和可靠性评级至少 C。
- [Google Testing Blog](https://testing.googleblog.com/2020/08/code-coverage-best-practices.html) 明确说没有“理想覆盖率”；其通用指导是 60% 可接受、75% 值得肯定、90% 优秀，并反对脱离业务风险的全局数字崇拜。因此 80% 可作新代码门禁，但不是正确性的充分证明。
- [GoogleTest Primer](https://google.github.io/googletest/primer.html) 要求测试独立、可重复、结构与被测代码相符、失败信息充分且运行快。
- [OpenJDK HotSpot 单测指南](https://github.com/openjdk/jdk/blob/master/doc/hotspot-unit-tests.md)还要求隔离、原子且自包含、可重复，并强调测试必须有断言，不能只“访问”代码。
- OpenAI 的[Codex 介绍](https://openai.com/index/introducing-codex/)明确要求人类审查和验证 agent 生成代码；[Codex 安全实践](https://openai.com/index/running-codex-safely/)强调受管配置、受限执行、高风险审批和可解释 agent 行为的日志。

## 建议的 AI 生成代码政策

以下是基于上述证据的 Threadline 综合建议，不是外部规范原文。

### 函数和控制流

- 有效代码行超过 40 行触发警告；新建或修改的函数超过 60 行阻断。p3c Java 规则可保留其官方 80 行上限，Threadline 以 60 行作为更严格的跨语言新代码政策。
- 认知复杂度最大 15；圈复杂度 10 警告、超过 15 阻断。
- 嵌套深度最大 4；超过 3 层时，审查人必须确认是否可用早返回、状态机、策略或拆分职责降低。
- 普通函数最多 5 个参数；构造器、序列化和协议边界最多 7 个。超过时优先使用有名字、表达业务概念的 request/options 类型。
- 禁止通过拆成大量仅调用一次、无语义名称的小函数来“满足行数”。拆出的函数必须表达不变式、业务步骤或明确输入/输出边界。

### 类的内聚与责任

- class 行数只作预警，不作“设计正确”的证明：类型体 250 行警告、350 行阻断；每类超过 11 个行为方法触发设计审查。
- 类必须有一句可陈述的单一职责。若描述需要“并且/同时/顺便”，或同一类包含两个不同变化原因，应拆为更内聚的模块。
- 审查类时必须同时检查：公开接口数量、字段是否服务同一不变式、方法是否主要操作本类状态、依赖是否来自同一领域边界、类总复杂度、测试是否能独立构造。行数合格但低内聚的类仍不合格。
- 数据载体、协议类型和不可变值对象可以方法很少；适配器、解析器和状态机可以相对较长，但必须有明确边界、表驱动结构或状态转换测试。禁止为了降低 class 行数制造无领域意义的碎片类。
- 文件 300 个有效行警告、500 阻断；生成 SDK、fixture、静态表和 migration 只能做路径级排除，不能在普通源文件内随意关闭。

### 测试、审查和例外

- PR 新代码覆盖率至少 80%，新代码重复率不高于 3%。Threadline 的权限、审批、租户隔离、幂等、并发/恢复和密码学状态转换不能因达到 80% 而省略关键分支测试。
- 所有新/修改行为至少有成功、拒绝/错误和关键边界证据；修 bug 必须先有可失败的回归测试。高风险状态机和授权决策应优先看分支覆盖，并补属性、契约或变异测试，而不是只看行覆盖。
- 新增 lint 抑制、覆盖率排除或测试跳过必须写理由，绑定 issue 和 owner，由审查人批准；“AI 生成”不是例外理由。
- 合并证据至少包含格式化、编译/类型检查、静态分析、单元/契约测试，以及实际改动路径的最小集成验证。只接受命令、结果和制品，不接受 agent 自述。
- 权限、租户隔离、本地文件能力、高影响审批、密钥/恢复和持久化契约的改动必须有人类审查，不得仅由另一个模型审查后自动合并。
- 采用“只阻断新违规”的过渡方式：现有违规先建立基线；PR 不得增加或恶化违规；改到旧的大函数/类时至少不能让其更长、更复杂或职责更多。

## 实施顺序

1. 将上述 Threadline policy 固化为工程质量文档，清楚区分警告、阻断和需批准例外。
2. 分语言启用 ESLint、Clippy、golangci-lint、SwiftLint 和 Detekt 对应规则，先生成现状报告，不批量重写。
3. CI 先阻断新增违规、低于 80% 的新代码覆盖和未解释的 suppress/skip；旧债建立基线。
4. 对超大或超复杂现有单元按风险拆 issue；先用特征/回归测试锁定行为，再重构。
5. 运行 4–6 周后，用真实告警率、例外率、缺陷逃逸和审查时间校准阈值。
