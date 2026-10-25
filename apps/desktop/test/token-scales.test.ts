import { expect, test } from "vitest";
import { scaleCustomProperties } from "../src/tokens.ts";

test("composer scales use the same canonical CSS names as the prototype", () => {
  const properties = scaleCustomProperties();
  expect(properties["--tl-space-composer-outer-inset"]).toBe("1.25rem");
  expect(properties["--tl-radius-composer-send"]).toBe("0.5625rem");
  expect(properties["--tl-space-2xl"]).toBe("1.5rem");
  expect(properties["--tl-radius-full"]).toBe("9999px");
  expect(properties).not.toHaveProperty("--tl-space-composerOuterInset");
});
