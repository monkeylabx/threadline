/*
 * Repeatable local accessibility evidence for the HTML prototype.
 *
 * Dependencies are intentionally installed outside the repository so this audit
 * does not mutate the integration-owned workspace manifest or lockfile. See
 * docs/quality/prototype-accessibility.md for the pinned setup and command.
 */

const { Builder, Key } = require("selenium-webdriver");
const chrome = require("selenium-webdriver/chrome");
const { AxeBuilder } = require("@axe-core/webdriverjs");

const chromePath = process.env.CHROME_PATH;
const driverPath = process.env.CHROMEDRIVER_PATH;
const origin = process.env.PROTOTYPE_ORIGIN || "http://127.0.0.1:4173";

if (!chromePath || !driverPath) {
  throw new Error("Set CHROME_PATH and CHROMEDRIVER_PATH to pinned local binaries");
}

const states = [
  { name: "desktop-channel", width: 1440, height: 900, mobile: false, path: "/?screen=channel" },
  { name: "mobile-channel", width: 390, height: 844, mobile: true, path: "/mobile/index.html?view=channel" },
  { name: "desktop-task-dialog", width: 1440, height: 900, mobile: false, path: "/?screen=channel&modal=task" },
  { name: "mobile-task-sheet", width: 390, height: 844, mobile: true, path: "/mobile/index.html?view=task" },
  { name: "mobile-approval-sheet", width: 390, height: 844, mobile: true, path: "/mobile/index.html?view=approval" },
];

const tags = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa", "best-practice"];

async function setViewport(driver, width, height, mobile) {
  if (mobile) {
    await driver.sendDevToolsCommand("Emulation.setDeviceMetricsOverride", {
      width,
      height,
      deviceScaleFactor: 1,
      mobile: true,
    });
    return driver.executeScript("return { width: innerWidth, height: innerHeight }");
  }

  await driver.sendDevToolsCommand("Emulation.clearDeviceMetricsOverride");
  await driver.manage().window().setRect({ width, height, x: 0, y: 0 });
  for (let attempt = 0; attempt < 3; attempt += 1) {
    const viewport = await driver.executeScript("return { width: innerWidth, height: innerHeight }");
    if (viewport.width === width && viewport.height === height) return viewport;
    const rect = await driver.manage().window().getRect();
    await driver.manage().window().setRect({
      width: rect.width + width - viewport.width,
      height: rect.height + height - viewport.height,
      x: 0,
      y: 0,
    });
  }
  return driver.executeScript("return { width: innerWidth, height: innerHeight }");
}

async function pageSnapshot(driver) {
  return driver.executeScript(`
    const root = document.documentElement;
    return {
      viewport: {
        width: innerWidth,
        height: innerHeight,
        devicePixelRatio,
      },
      document: {
        clientWidth: root.clientWidth,
        scrollWidth: root.scrollWidth,
        horizontalOverflow: root.scrollWidth > root.clientWidth + 1,
      },
      media: {
        reducedMotion: matchMedia("(prefers-reduced-motion: reduce)").matches,
        forcedColors: matchMedia("(forced-colors: active)").matches,
      },
    };
  `);
}

async function keyboardEvidence(driver) {
  await driver.executeScript("if (document.activeElement) document.activeElement.blur()");
  const stops = [];
  for (let index = 0; index < 25; index += 1) {
    await driver.actions().sendKeys(Key.TAB).perform();
    stops.push(await driver.executeScript(`
      const element = document.activeElement;
      const rect = element.getBoundingClientRect();
      const style = getComputedStyle(element);
      const label = element.getAttribute("aria-label")
        || element.getAttribute("title")
        || element.getAttribute("placeholder")
        || (element.textContent || "").trim().replace(/\\s+/g, " ");
      return {
        index: arguments[0],
        tag: element.tagName.toLowerCase(),
        id: element.id || null,
        className: typeof element.className === "string" ? element.className : null,
        label: label.slice(0, 120),
        inViewport: rect.right > 0 && rect.bottom > 0 && rect.left < innerWidth && rect.top < innerHeight,
        outline: style.outline,
        boxShadow: style.boxShadow,
      };
    `, index + 1));
  }
  return stops;
}

async function responsiveEvidence(driver, state) {
  await setViewport(driver, state.width, state.height, state.mobile);
  await driver.sendDevToolsCommand("Emulation.setEmulatedMedia", { media: "", features: [] });
  await driver.get(`${origin}${state.path}`);
  await driver.sleep(250);
  const baseline = await pageSnapshot(driver);

  await driver.sendDevToolsCommand("Emulation.setEmulatedMedia", {
    media: "",
    features: [
      { name: "prefers-reduced-motion", value: "reduce" },
      { name: "forced-colors", value: "active" },
    ],
  });
  await driver.navigate().refresh();
  await driver.sleep(250);
  const reducedMotionAndForcedColors = await pageSnapshot(driver);

  await driver.sendDevToolsCommand("Emulation.setEmulatedMedia", { media: "", features: [] });
  await setViewport(driver, Math.floor(state.width / 2), Math.floor(state.height / 2), state.mobile);
  await driver.get(`${origin}${state.path}`);
  await driver.sleep(250);
  const zoom200LayoutEquivalent = await pageSnapshot(driver);

  return { baseline, reducedMotionAndForcedColors, zoom200LayoutEquivalent };
}

async function main() {
  const options = new chrome.Options();
  options.setChromeBinaryPath(chromePath);
  options.addArguments("headless=new", "no-sandbox", "disable-dev-shm-usage");
  const driver = await new Builder()
    .forBrowser("chrome")
    .setChromeOptions(options)
    .setChromeService(new chrome.ServiceBuilder(driverPath))
    .build();

  const evidence = {
    generatedAt: new Date().toISOString(),
    axeCore: require("axe-core/package.json").version,
    seleniumWebDriver: require("selenium-webdriver/package.json").version,
    states: [],
    responsive: {},
  };

  try {
    for (const state of states) {
      await setViewport(driver, state.width, state.height, state.mobile);
      await driver.get(`${origin}${state.path}`);
      await driver.sleep(250);
      const viewport = await pageSnapshot(driver);
      const axe = await new AxeBuilder(driver).withTags(tags).analyze();
      const keyboard = await keyboardEvidence(driver);
      evidence.states.push({
        name: state.name,
        requestedViewport: { width: state.width, height: state.height },
        actualViewport: viewport.viewport,
        url: await driver.getCurrentUrl(),
        testEngine: axe.testEngine,
        testEnvironment: axe.testEnvironment,
        violations: axe.violations.map((violation) => ({
          id: violation.id,
          impact: violation.impact,
          help: violation.help,
          helpUrl: violation.helpUrl,
          nodes: violation.nodes.map((node) => ({
            target: node.target,
            html: node.html,
            failureSummary: node.failureSummary,
          })),
        })),
        keyboard,
      });
    }

    evidence.responsive.desktop = await responsiveEvidence(driver, states[0]);
    evidence.responsive.mobile = await responsiveEvidence(driver, states[1]);
    process.stdout.write(`${JSON.stringify(evidence, null, 2)}\n`);
  } finally {
    await driver.quit();
  }
}

main().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
