import { describe, expect, it } from "vitest";
import { isNearBottom } from "./autoScroll";

describe("atendimentos auto scroll", () => {
  it("detecta quando usuario esta perto do fim da lista", () => {
    expect(isNearBottom(920, 80, 1000)).toBe(true);
    expect(isNearBottom(830, 80, 1000)).toBe(false);
  });
});
