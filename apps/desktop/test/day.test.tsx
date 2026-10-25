import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, test } from "vitest";
import { FixtureRepository, type Message } from "../src/repository.ts";
import { dayLabel, dayStarts } from "../src/shell/day.ts";
import { Shell } from "../src/shell/Shell.tsx";

afterEach(cleanup);

// Local-time constructors keep every case independent of the host time zone.
const at = (month: number, day: number, hours = 12, minutes = 0, year = 2026) => new Date(year, month - 1, day, hours, minutes).getTime();
const now = at(10, 25, 9, 30);

describe("dayLabel", () => {
  test("labels today from midnight to the last minute", () => {
    expect(dayLabel(at(10, 25, 0, 0), now)).toBe("今天");
    expect(dayLabel(at(10, 25, 23, 59), now)).toBe("今天");
  });

  test("labels the previous local day across midnight", () => {
    expect(dayLabel(at(10, 24, 23, 59), at(10, 25, 0, 1))).toBe("昨天");
  });

  test("labels older days by date, adding the year only when it differs", () => {
    expect(dayLabel(at(3, 5), now)).toBe("3月5日");
    expect(dayLabel(at(12, 31, 12, 0, 2025), now)).toBe("2025年12月31日");
    expect(dayLabel(at(10, 26), now)).toBe("10月26日");
  });
});

const message = (eventId: string, sentAtMs?: number): Message => ({
  eventId, channelId: "c", authorName: "a", sentAt: "", body: "", state: "committed",
  ...(sentAtMs === undefined ? {} : { sentAtMs }),
});

describe("dayStarts", () => {
  test("opens a day only at the first timestamped message of each local day", () => {
    const list = [message("1", at(10, 24, 22)), message("2", at(10, 24, 23)), message("3"), message("4", at(10, 25, 8)), message("5", at(10, 25, 9))];
    expect([...dayStarts(list)]).toEqual(["1", "4"]);
  });

  test("never invents a day for messages without timestamps", () => {
    expect(dayStarts([message("1"), message("2")]).size).toBe(0);
    expect(dayStarts([]).size).toBe(0);
  });
});

describe("timeline day dividers", () => {
  test("shows one 今天 divider before today's fixture history", async () => {
    const clock = () => now;
    render(<Shell repository={new FixtureRepository({ autoCommit: false, now: clock })} clock={clock} />);
    await screen.findAllByTestId("message");
    const dividers = screen.getAllByTestId("day-divider");
    expect(dividers.map((divider) => divider.textContent)).toEqual(["今天"]);
    expect(dividers[0]?.nextElementSibling?.getAttribute("data-testid")).toBe("message");
  });

  test("relabels the same history as 昨天 on the next day", async () => {
    render(<Shell repository={new FixtureRepository({ autoCommit: false, now: () => now })} clock={() => at(10, 26, 8)} />);
    await screen.findAllByTestId("message");
    expect(screen.getAllByTestId("day-divider").map((divider) => divider.textContent)).toEqual(["昨天"]);
  });
});
