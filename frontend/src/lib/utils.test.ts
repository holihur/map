import { describe, expect, it } from "vitest";

import { cn } from "@/lib/utils";

describe("cn", () => {
  it("合并类名", () => {
    expect(cn("a", "b")).toBe("a b");
  });

  it("去重冲突的 tailwind 类", () => {
    expect(cn("p-2", "p-4")).toBe("p-4");
    expect(cn("text-sm", false && "hidden", undefined, "font-bold")).toBe(
      "text-sm font-bold"
    );
  });

  it("支持条件对象", () => {
    expect(cn("base", { active: true, disabled: false })).toBe("base active");
  });
});
