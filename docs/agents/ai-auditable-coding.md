# AI-Auditable Coding Standard

Status: project generation policy

This standard controls how human and Agent-authored code is designed and generated in Threadline. Generation and manual review enforce this policy today. Existing static checks cover only the surfaces identified in section 6; a green build does not certify all numeric limits.

The keywords **MUST**, **MUST NOT**, **SHOULD**, and **MAY** are normative. Numeric limits apply to new or materially changed handwritten code. Existing code is ratcheted: a change MUST NOT worsen a metric, and touched code SHOULD move toward the limits.

## 1. Generation sequence

Before writing implementation code, the author or Agent MUST state:

1. The unit's single responsibility in one sentence.
2. Its invariants and authorization boundary.
3. Inputs, outputs, side effects, and failure modes.
4. The test seam and the observable behavior that proves correctness.
5. Whether the change can be independently reverted.

If the responsibility sentence needs unrelated clauses joined by “and,” or the invariants belong to different owners, the design MUST be split before code is generated.

## 2. Functions and methods

Every new or materially changed handwritten function MUST satisfy all of these limits:

| Metric | Project limit | Generation behavior |
| --- | ---: | --- |
| Non-comment lines | 60 | Extract named units before continuing; 80 is the absolute exception ceiling. |
| Executable statements | 40 | Split orchestration from policy and I/O. |
| Cyclomatic complexity | 10 | Split decision tables, policies, or state transitions. |
| Cognitive complexity | 15 | Flatten nesting and name intermediate decisions. |
| Nesting depth | 3 | Prefer guard clauses and explicit state dispatch. |
| Parameters | 5 | Introduce a cohesive request/value object when arguments represent one concept. |

Additional rules:

- A function MUST do one observable job at one abstraction level.
- Boolean flags that switch unrelated behavior SHOULD become separate functions or an explicit strategy/type.
- Error paths MUST be explicit. Ignored results, empty catches, broad exception swallowing, and panic/force-unwrap shortcuts are prohibited outside a documented process boundary.
- Complex conditions SHOULD be moved into a named predicate, but extraction MUST NOT merely hide complexity in an ambiguously named helper.
- Pure lookup tables, exhaustive protocol mappings, and generated bindings MAY exceed the line limit when they remain declarative and contain no hidden control flow.

## 3. Types, classes, and modules

Line count is only a review trigger; cohesion and responsibility decide whether a type is acceptable.

A type or module MUST:

- own one primary responsibility and one coherent set of invariants;
- expose the smallest capability-oriented public surface needed by callers;
- keep authorization, persistence, transport, orchestration, and domain policy in explicit layers;
- avoid state or method clusters that can change independently;
- depend through contracts instead of reaching into another component's storage or internals.

The author or Agent MUST propose a split when any trigger is reached:

| Trigger | Threshold |
| --- | ---: |
| Handwritten non-comment lines owned by one type | 250 |
| Handwritten non-comment lines in one source file | 300 |
| Methods/functions owned by one type/module | 11 |
| Direct external type dependencies | 10 |
| Disconnected method/state clusters | More than 1 |

The normal hard ceilings are 350 non-comment lines per type, 500 per handwritten source file, and 15 methods per type/module. A cohesive algorithm, parser, protocol mapping, or platform adapter MAY cross a trigger, but crossing a hard ceiling requires the exception record in section 8.

For cohesion reviews, build a simple method-to-state/invariant map. If two method groups share neither state nor an invariant, they belong in separate types even when the file is short. Data-only DTOs, configuration records, enums, and generated models are judged by schema purpose rather than shared mutable state.

## 4. Tests generated with the code

- A behavior change MUST include new or updated tests in the same Issue and change set.
- A bug fix MUST add a regression test that fails for the defect and passes for the fix.
- Unit tests MUST be deterministic, independent, repeatable, and hermetic: no real network, wall-clock dependency, production service, shared mutable database, or execution-order dependency.
- Tests MUST assert observable behavior and MUST fail when that behavior is deliberately broken. Asserting only that code ran or a mock was called is insufficient unless the interaction is the contract.
- Non-trivial decision logic MUST cover success, boundary, invalid-input, and failure cases.
- When coverage tooling is available, changed executable production lines MUST maintain at least 80% coverage. Changed decision logic in authorization, capability grants, crypto/recovery, audit, and idempotency MUST cover every branch with positive and negative cases.
- Coverage is evidence, not proof. A high number does not excuse weak assertions, missing boundary cases, nondeterminism, or tests coupled to private implementation details.
- Integration tests MUST identify the real boundary under test. End-to-end tests MUST NOT duplicate every lower-level branch case.

## 5. Reviewability and reversibility

- A change set SHOULD stay at or below 400 human-authored changed lines. Above 800 lines, it MUST be split unless the excess is generated or mechanical and isolated.
- Refactoring and behavior changes SHOULD be separate commits or change sets.
- Generated output MUST be isolated from handwritten logic so reviewers can inspect the generator input and semantic change.
- Each commit MUST build and preserve the agreed behavior. Schema, persisted state, and protocol changes require an explicit migration, compatibility, and rollback story.
- Comments and Issue context MUST explain *why* a non-obvious constraint exists; code and names SHOULD explain *what* happens.

## 6. Language verification map

The generation limits are language-independent. At integration baseline
`b045d402ebf45537b1ac24bfbe4413465114d235`, the inventory below distinguishes
executed checks from planned tooling. This documentation change installs no analyzer.

| Current entry | Executed scope | What it does not establish |
| --- | --- | --- |
| [Workspace lint](../../scripts/workspace.mjs), `lint()` | Text/pin checks, Node syntax, Rust formatting, Go formatting, Swift package parsing and Android lint | Function/type size, statement budgets, cognitive/cyclomatic limits and cohesion are not measured to this policy. |
| [Desktop lint](../../apps/desktop/package.json) | Node TypeScript syntax check | This is not ESLint and enforces none of the proposed ESLint metrics. |
| [Build workflow](../../.github/workflows/build.yml), `workspace-linux` | Cargo Clippy with warnings denied, workspace builds/tests and NATS fixture checks | Default enabled Clippy diagnostics are enforced; this is not an explicit configuration of this document's 60-line, 5-argument, depth-3 profile. |
| Build workflow platform jobs and [secret scan](../../.github/workflows/secret-scan.yml) | Platform compilation/tests and secret scanning | No cross-language complexity, changed-line coverage percentage, cohesion or change-size gate is configured here. |

All six function metrics, the type/module triggers and ceilings, the 80% changed-line
coverage policy, security decision branch completeness, and the 400/800 change-size
budgets therefore require author evidence and manual review today. Reviewers record
measurements and exceptions in the Issue/handoff; CI success is not a substitute.
Legacy ratcheting and method-to-invariant maps also remain manual review obligations.

The following is a **planned tooling map**, not a list of installed checks. A separate
Integration task must pin versions, define counting/exclusion semantics, add failing
fixtures for each enabled rule, and update this inventory before claiming enforcement:


| Area | Preferred checks |
| --- | --- |
| Go | `golangci-lint`: `funlen`, `gocyclo` or `cyclop`, nesting/maintainability checks |
| Rust | Clippy: `too_many_lines`, `too_many_arguments`, `excessive_nesting`; treat line metrics around macros cautiously |
| TypeScript/JavaScript | ESLint: `max-lines-per-function`, `complexity`, `max-depth`, `max-params`, `max-lines` |
| Kotlin | detekt: `LongMethod`, `CognitiveComplexMethod`, `CyclomaticComplexMethod`, `NestedBlockDepth`, `TooManyFunctions`, `LargeClass` |
| Swift | SwiftLint: `function_body_length`, `cyclomatic_complexity`, `function_parameter_count`, `type_body_length`, `file_length` |
| Java | PMD and Checkstyle size, complexity, coupling, nesting, and test rules; Alibaba p3c where applicable |

CI configuration MUST pin tool versions. A rule suppression MUST be narrow and follow section 8.

## 7. Generated, declarative, and legacy code

The numeric function/type limits do not directly apply to:

- generated SDKs and bindings;
- vendored third-party source;
- database migrations and declarative schemas;
- golden fixtures and test vectors;
- exhaustive constant tables and protocol enumerations.

These files MUST be identifiable by path or generated header and MUST NOT contain disguised handwritten business logic. Their source generator or input remains subject to this standard. Tests and handwritten adapters remain fully subject to complexity and quality rules.

Legacy violations do not justify new violations. When touching a violating unit, the change MUST NOT increase its line count, complexity, nesting, parameter count, or coupling unless the Issue records a time-bounded exception. Prefer extracting a tested seam before changing behavior.

## 8. Exceptions

An exception is allowed only when splitting would reduce correctness, clarity, or auditability. The Issue and agent handoff MUST record:

- the exact rule and unit;
- the measured value;
- why the straightforward split is worse;
- the reviewer/owner;
- the removal condition or why the exception is permanent;
- the tests and review evidence that compensate for the exception.

Blanket linter disables, file-wide suppressions without explanation, and “the AI generated it” are not valid exceptions.

## 9. Basis for the thresholds

These numbers are project policy, not universal laws. They intentionally combine conservative, mechanically checkable defaults from open primary sources:

- Alibaba's official [`p3c`](https://github.com/alibaba/p3c) repository and [Huangshan edition](https://github.com/alibaba/p3c/blob/master/Java%E5%BC%80%E5%8F%91%E6%89%8B%E5%86%8C%28%E9%BB%84%E5%B1%B1%E7%89%88%29.pdf) cover programming, unit tests, engineering, security, and MySQL. Its implemented [`MethodTooLongRule`](https://github.com/alibaba/p3c/blob/master/p3c-pmd/src/main/java/com/alibaba/p3c/pmd/lang/java/rule/other/MethodTooLongRule.java#L42-L52) recommends no more than 80 non-comment lines and the manual limits `if/else` nesting to three levels.
- NASA/JPL's [Power of Ten](https://spinroot.com/gerard/pdf/Power_of_Ten.pdf) uses roughly 60 lines per function for safety-critical C and emphasizes rules that can be checked mechanically.
- [PMD's Java design rules](https://docs.pmd-code.org/latest/pmd_rules_java_design.html) report cyclomatic complexity at 10, cognitive complexity at 15, nesting at 3, and object coupling at 20 by default.
- ESLint documents defaults of [50 lines per function](https://eslint.org/docs/latest/rules/max-lines-per-function), [20 cyclomatic complexity](https://eslint.org/docs/latest/rules/complexity), [4 nesting levels](https://eslint.org/docs/latest/rules/max-depth), [3 parameters](https://eslint.org/docs/latest/rules/max-params), and [300 lines per file](https://eslint.org/docs/latest/rules/max-lines). These independent metrics catch different failure modes.
- [`golangci-lint`](https://golangci-lint.run/docs/linters/configuration/) documents `funlen` defaults of 60 lines and 40 statements and recommends cyclomatic complexity in the 10–20 range.
- Rust Clippy documents configurable defaults of [100 lines and 7 arguments](https://doc.rust-lang.org/clippy/lint_configuration.html). Its lint documentation also cautions that a single calculated “cognitive complexity” score is not a complete model of human understanding.
- detekt documents [60-line methods, 600-line classes, cognitive complexity 15, and 11 functions per class](https://detekt.dev/docs/rules/complexity/).
- SwiftLint documents [50-line warning/100-line error function bodies](https://realm.github.io/SwiftLint/function_body_length.html) and related type/file/complexity rules.
- Google's [small-change guidance](https://google.github.io/eng-practices/review/developer/small-cls.html) says 100 changed lines is usually reasonable, 1,000 is usually too large, and related tests should accompany the change. Its [review guidance](https://google.github.io/eng-practices/review/reviewer/looking-for.html) expects reviewers to understand every human-written line and inspect whether tests actually fail when behavior breaks.
- Bazel's [test encyclopedia](https://bazel.build/reference/test-encyclopedia) explains why hermetic, deterministic, and reentrant tests matter for reproducibility and auditability.

### Project adjustments and provenance

The source defaults above are references, not a claim of universal optimal values.
Every local numeric choice is accounted for below; a project-selected value is a
review budget, not an empirically validated correctness or security boundary.

| Local metric/value | Basis and reason for this choice |
| --- | --- |
| Function 60 lines; exception ceiling 80 | 60 follows the cited JPL/funlen guidance; 80 uses p3c's outer bound to allow a documented cohesive unit without making the exception the default. |
| 40 executable statements | Uses the cited funlen default as a second budget because several statements can occupy one line. |
| Cyclomatic 10; cognitive 15 | Uses the cited PMD defaults, favoring its cyclomatic threshold over ESLint's looser 20. These are distinct review signals. |
| Nesting 3 | Uses the cited p3c/PMD guidance; stricter than ESLint's 4 to expose failure paths early. |
| Parameters 5 | Project compromise between ESLint's 3 and Clippy's 7; allows small explicit boundaries without forcing unrelated values into a wrapper. |
| Type 250 trigger / 350 ceiling | Project-selected, not an upstream default. Leaves a 100-line review margin for cohesive state machines; deliberately below the cited detekt 600-line class budget. |
| File 300 trigger / 500 ceiling | 300 follows the cited ESLint default; 500 is a project-selected 200-line exception margin for cohesive adapters and parsers. |
| Methods 11 trigger / 15 ceiling | 11 follows the cited detekt default; 15 is a project-selected allowance of four methods, conditional on a single invariant cluster. |
| External type dependencies 10 | Project-selected, half the cited PMD coupling default of 20, to expose cross-boundary coordination earlier. |
| More than 1 disconnected cluster | Project interpretation of single responsibility: independent invariant owners should not share a module merely to meet size limits. It is a semantic review, not a sourced numeric complexity score. |
| 80% changed executable line coverage | Project-selected minimum where measurement exists; no cited source establishes 80% as sufficient. It leaves room for integration-only paths while requiring reviewers to explain uncovered behavior. |
| Every security/idempotency decision branch | Project risk policy, not an upstream coverage default. Positive and negative behavior evidence is required because aggregate line coverage can conceal a missing denial path. |
| 400 changed-line target / 800 split threshold | Project-selected within Google's cited small-change guidance (100 usually reasonable, 1,000 usually too large). The target allows code and tests together; the upper bound intervenes before the cited too-large example. |

Before automation, measurements must state the language/tool or manual counting
method and exclude comments/blanks for the non-comment line budgets. Differences
around macros, multiline expressions, nested functions or generated sections must
be recorded, not silently treated as equivalent scores. These initial budgets have
not been calibrated against a project defect dataset. Adjustments require a reviewed
Issue with measured examples and an explanation of readability and testing impact;
changing tooling defaults alone does not change policy.

When evidence or language tooling changes, update this document and any affected CI configuration together. Do not silently change generation behavior by upgrading a linter profile.
