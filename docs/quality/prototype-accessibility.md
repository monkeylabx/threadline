# HTML Prototype Accessibility Baseline

Status: **FAIL — remediation is tracked in #59 and #60**  
Audit date: 2026-08-22  
Base commit: `1563315252e075a0dfd749000437e903eaebeacb`  
Scope: the single HTML prototype at `docs/prototype/index.html` and its internal Mobile renderer

## Decision

The prototype is keyboard-navigable at its nominal Desktop and Mobile viewports, inactive Mobile surfaces are removed from navigation with `inert`, and neither nominal viewport has horizontal overflow. It does not yet meet the accessibility baseline because the Mobile Task Sheet has a Critical ARIA failure and every audited state has Serious contrast failures.

T020 is an audit, not a remediation task. The exceptions below have accountable owners, rationale, and ready follow-ups:

| Gap | Impact | Owner | Rationale | Follow-up |
| --- | --- | --- | --- | --- |
| A11Y-01 Mobile Task tab semantics | Critical | Design-system / Product Design | A `tablist` contains plain buttons without the required `tab` children or selected/panel relationships. | #59 |
| A11Y-02 Color contrast | Serious | Design-system / Product Design | Muted text and several action states fall below WCAG AA across Channel, Dialog, Task, and Approval states. | #60 |
| A11Y-03 Modal/Sheet focus and semantics | High manual | Design-system / Product Design | The Desktop modal does not move/trap/restore focus or make the background inert; Mobile sheets lack complete sheet/dialog focus semantics. | #59 |
| A11Y-04 Heading and form-name structure | Moderate/manual | Design-system / Product Design | Channel views have no `h1`; composer and approval textareas depend on placeholders instead of persistent labels. | #59 |
| A11Y-05 Mobile 200% reflow | High manual | Design-system / Product Design | The 200% layout-equivalent Mobile check produces horizontal overflow. | #60 |
| A11Y-06 Mobile textarea focus | High manual | Design-system / Product Design | Later component rules override the shared focus rule, leaving composer and approval textareas without a visible keyboard focus indicator. | #60 |

## Fixed Environment And Command

The audit used:

- `@axe-core/webdriverjs` and `axe-core` 4.13.0
- `selenium-webdriver` 4.44.0
- Google Chrome 151.0.7922.172
- official Chrome for Testing ChromeDriver 151.0.7922.174 for macOS arm64
- exact Desktop 1440×900 and Mobile 390×844 CSS viewports, set through Selenium and the Chrome DevTools Protocol

The checked-in runner avoids changes to the integration-owned workspace manifest and lockfile. Install its pinned dependencies into a temporary prefix, start the prototype, and run:

```sh
mkdir -p /private/tmp/threadline-a11y-modules
npm install --prefix /private/tmp/threadline-a11y-modules --no-save \
  @axe-core/webdriverjs@4.13.0 selenium-webdriver@4.44.0
ruby -run -e httpd docs/prototype -p 4173

NODE_PATH=/private/tmp/threadline-a11y-modules/node_modules \
CHROME_PATH="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
CHROMEDRIVER_PATH=/absolute/path/to/chromedriver \
node docs/prototype/tools/accessibility-audit.cjs
```

The JSON output contains the engine/browser versions, requested and actual viewports, complete rule/node evidence, 25 keyboard stops per state, focus styles, reduced-motion/forced-colors state, and 200% layout-equivalent overflow measurements.

## Automated Results

Tags: `wcag2a`, `wcag2aa`, `wcag21aa`, `wcag22aa`, and `best-practice`.

| State | Viewport | Critical | Serious | Moderate | Result |
| --- | --- | ---: | ---: | ---: | --- |
| Desktop Channel | 1440×900 | 0 | `color-contrast` — 22 nodes | `page-has-heading-one` — 1 node | FAIL |
| Mobile Channel | 390×844 | 0 | `color-contrast` — 8 nodes | `page-has-heading-one` — 1 node | FAIL |
| Desktop Task Dialog | 1440×900 | 0 | `color-contrast` — 23 nodes | 0 | FAIL |
| Mobile Task Sheet | 390×844 | `aria-required-children` — 1 node | `color-contrast` — 14 nodes | 0 | FAIL |
| Mobile Approval Sheet | 390×844 | 0 | `color-contrast` — 11 nodes | 0 | FAIL |

Representative contrast failures include Desktop sidebar labels, timestamps, keyboard-hint text, and quiet metadata; Mobile day markers, timestamps, task metadata, approval metadata, and one action state. Counts are affected DOM nodes, not separate axe rules.

## Keyboard And Accessibility Tree

### Passing evidence

- At the nominal Desktop and Mobile Channel viewports, Tab order follows the visual/document order and every visited control stays in the viewport.
- Inactive Mobile home/search/task/approval surfaces have `aria-hidden="true"` and `inert`; keyboard traversal does not enter them.
- Browser accessibility inspection exposes accessible names for icon buttons through their `title` attributes.
- The Desktop Task surface has `role="dialog"`, `aria-modal="true"`, and an `aria-labelledby` relationship.
- Native Desktop focus outlines remain visible on sampled controls. Mobile buttons retain the prototype's 2px focus outline.

### Failing evidence

- When the Desktop Task Dialog is initially open, the first Tab stop remains in the background rail. The background is not inert, focus is not moved into the dialog, focus is not contained, Escape is not handled, and focus is not restored to the opener.
- Mobile Task and Approval sheets are labeled `aside` elements, but opening them does not place focus on their heading/first action or expose complete modal/sheet semantics.
- Mobile `role="tablist"` children have no `role="tab"`, `aria-selected`, `aria-controls`, matching tab panels, or arrow-key behavior.
- Mobile composer and approval textareas have computed `outline: none` while keyboard-focused.
- Channel composer and approval-comment textareas rely on placeholder text as their accessible name and visible instruction; there is no persistent label.
- Channel views contain no page-level `h1`.

## Responsive, Motion, And High Contrast

| Check | Desktop | Mobile |
| --- | --- | --- |
| Nominal horizontal overflow | PASS: 1440 client / 1440 scroll | PASS: 390 client / 390 scroll |
| 200% layout-equivalent reflow | PASS: 720 client / 720 scroll; the single entry routes to its internal narrow renderer | FAIL: 195 client / 320 scroll after the viewport minimum is applied |
| Reduced motion | PASS for the audited static state; no active Desktop animation, and the Mobile sheet/home/search transitions are disabled by the existing media query | PASS for the audited state |
| Forced colors | Emulation activates successfully and retains the same document width and focusable structure | Emulation activates successfully and retains the same document width and focusable structure |

The forced-colors result is a structural smoke check, not proof of visual parity. #60 owns screenshot-level verification and any required `forced-colors: active` safeguards.

## Boundaries And Residual Risk

- Real-device and assistive-technology validation is intentionally **NOT RUN** for T020. This follows the task boundary and does not change the separate Native Mobile Gate.
- The 200% check uses the layout-equivalent half-size CSS viewport (720×450 and 195×422). It is deterministic in headless Chrome and exercises reflow, but the remediation task should also capture an interactive browser-zoom screenshot.
- The audit runner creates no second visual source and does not modify the prototype.
- Passing this audit after #59 and #60 still does not replace VoiceOver, TalkBack, keyboard/switch-device, or real-device release evidence.

## Gate

T020 audit deliverables are complete. The prototype accessibility baseline remains **FAIL/HOLD** until #59 and #60 close with zero Critical/Serious automated findings and the manual keyboard/reflow checks pass.
