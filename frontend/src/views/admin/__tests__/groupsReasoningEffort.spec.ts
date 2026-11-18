import { describe, expect, it } from "vitest";

import {
  createReasoningEffortMappingPair,
  createReasoningEffortMappingRow,
  normalizeReasoningEffortForPlatform,
  normalizeReasoningEffortMappingForPlatform,
  normalizeReasoningEffortMatchType,
  normalizeReasoningEffortOverLimit,
  reasoningEffortMappingOptionsForPlatform,
  reasoningEffortMappingsToAPI,
  reasoningEffortMappingsToRows,
  reasoningEffortOptionsForPlatform,
  validateReasoningEffortMappings,
} from "../groupsReasoningEffort";

describe("groupsReasoningEffort", () => {
  it("keeps none out of the max-effort choices while allowing it in mappings", () => {
    expect(
      reasoningEffortOptionsForPlatform().map((option) => option.value),
    ).toEqual([
      "minimal",
      "low",
      "medium",
      "high",
      "xhigh",
      "max",
    ]);
    expect(
      reasoningEffortMappingOptionsForPlatform().map(
        (option) => option.value,
      ),
    ).toEqual(["none", "minimal", "low", "medium", "high", "xhigh", "max"]);
  });

  // 混合分组保存完整强度集合，具体模型能力由执行器检查。
  it("round-trips model-scoped mappings across providers", () => {
    expect(reasoningEffortOptionsForPlatform().map((option) => option.value))
      .toEqual(["minimal", "low", "medium", "high", "xhigh", "max"]);
    const rows = reasoningEffortMappingsToRows([
      { from: "max", to: "xhigh", match_type: "prefix", model: "claude-" },
      { from: "minimal", to: "low" },
      { from: "none", to: "low" },
    ]);
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "xhigh", match_type: "prefix", model: "claude-" },
      { from: "minimal", to: "low" },
      { from: "none", to: "low" },
    ]);
    expect(validateReasoningEffortMappings(rows)).toEqual({});
  });

  it("hydrates supported rows and drops stale custom values", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        { from: " max ", to: " xhigh " },
        { from: "ultra", to: "high" },
      ],
    );

    expect(rows).toHaveLength(1);
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "xhigh" },
    ]);
  });

  it("hydrates model scoped mappings", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        {
          from: "max",
          to: "low",
          match_type: "prefix",
          model: " gpt ",
        },
        {
          from: "max",
          to: "medium",
          match_type: "exact",
          model: "gpt-5.4",
        },
      ],
    );

    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "low", match_type: "prefix", model: "gpt" },
      { from: "max", to: "medium", match_type: "exact", model: "gpt-5.4" },
    ]);
  });

  it("groups multiple request mappings under the same type and model", () => {
    const rows = reasoningEffortMappingsToRows(
      [
        { from: "high", to: "medium", match_type: "prefix", model: "gpt" },
        { from: "xhigh", to: "medium", match_type: "prefix", model: "GPT" },
      ],
    );

    expect(rows).toHaveLength(1);
    expect(rows[0].match_type).toBe("prefix");
    expect(rows[0].model).toBe("gpt");
    expect(rows[0].pairs.map((pair) => pair.from)).toEqual(["high", "xhigh"]);
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "high", to: "medium", match_type: "prefix", model: "gpt" },
      { from: "xhigh", to: "medium", match_type: "prefix", model: "gpt" },
    ]);
  });

  it("omits empty model scopes from the API payload", () => {
    const row = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
    });
    expect(reasoningEffortMappingsToAPI([row])).toEqual([
      { from: "max", to: "low" },
    ]);
  });

  it("hydrates suffix mappings", () => {
    const rows = reasoningEffortMappingsToRows(
      [{ from: "max", to: "low", match_type: "suffix", model: " mini " }],
    );
    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "max", to: "low", match_type: "suffix", model: "mini" },
    ]);
  });

  it("keeps none limited to mappings and clears unsupported values", () => {
    expect(normalizeReasoningEffortForPlatform(" MAX ")).toBe("max");
    expect(normalizeReasoningEffortForPlatform("unsupported")).toBe("");
    expect(normalizeReasoningEffortForPlatform("none")).toBe("");
    expect(normalizeReasoningEffortMappingForPlatform(" none ")).toBe("none");
    expect(normalizeReasoningEffortMappingForPlatform("unsupported")).toBe("");
  });

  it("normalizes the over-limit action with downgrade as the default", () => {
    expect(normalizeReasoningEffortOverLimit(" DENY ")).toBe("deny");
    expect(normalizeReasoningEffortOverLimit("block")).toBe("downgrade");
    expect(normalizeReasoningEffortOverLimit(" ")).toBe("downgrade");
  });

  it("normalizes match type to exact, prefix, suffix, or empty", () => {
    expect(normalizeReasoningEffortMatchType("PREFIX")).toBe("prefix");
    expect(normalizeReasoningEffortMatchType("suffix")).toBe("suffix");
    expect(normalizeReasoningEffortMatchType("exact")).toBe("exact");
    expect(normalizeReasoningEffortMatchType("")).toBe("");
    expect(normalizeReasoningEffortMatchType("wildcard")).toBe("");
  });

  it("requires both sides of every mapping", () => {
    const first = createReasoningEffortMappingRow({ to: "low" });
    const second = createReasoningEffortMappingRow({ from: "max" });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { duplicateScope: "duplicateScope" },
      [second.id]: { duplicateScope: "duplicateScope" },
      [first.pairs[0].id]: { from: "fromRequired" },
      [second.pairs[0].id]: { to: "toRequired" },
    });
  });

  it("rejects duplicate source values case insensitively", () => {
    const first = createReasoningEffortMappingRow({ from: "MAX", to: "xhigh" });
    const second = createReasoningEffortMappingRow({ from: " max ", to: "high" });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { duplicateScope: "duplicateScope" },
      [second.id]: { duplicateScope: "duplicateScope" },
      [first.pairs[0].id]: { from: "duplicateFrom" },
      [second.pairs[0].id]: { from: "duplicateFrom" },
    });
  });

  it("allows the same source across different model scopes", () => {
    const prefix = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
      model: "gpt",
    });
    const exact = createReasoningEffortMappingRow({
      from: "max",
      to: "medium",
      match_type: "exact",
      model: "gpt-5.4",
    });
    const global = createReasoningEffortMappingRow({ from: "max", to: "high" });

    expect(validateReasoningEffortMappings([prefix, exact, global])).toEqual({});
  });

  it("allows multiple request values in one model scope", () => {
    const row = createReasoningEffortMappingRow({
      from: "high",
      to: "medium",
      match_type: "prefix",
      model: "gpt",
    });
    row.pairs.push(
      createReasoningEffortMappingPair({ from: "xhigh", to: "medium" }),
    );

    expect(validateReasoningEffortMappings([row])).toEqual({});
    expect(reasoningEffortMappingsToAPI([row])).toEqual([
      { from: "high", to: "medium", match_type: "prefix", model: "gpt" },
      { from: "xhigh", to: "medium", match_type: "prefix", model: "gpt" },
    ]);
  });

  it("rejects duplicate source values within the same model scope", () => {
    const first = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
      model: "GPT",
    });
    const second = createReasoningEffortMappingRow({
      from: "MAX",
      to: "high",
      match_type: "prefix",
      model: " gpt ",
    });

    expect(validateReasoningEffortMappings([first, second])).toEqual({
      [first.id]: { duplicateScope: "duplicateScope" },
      [second.id]: { duplicateScope: "duplicateScope" },
      [first.pairs[0].id]: { from: "duplicateFrom" },
      [second.pairs[0].id]: { from: "duplicateFrom" },
    });
  });

  it("allows empty type and model as a global mapping", () => {
    const row = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
    });
    expect(row.match_type).toBe("");
    expect(row.model).toBe("");
    expect(validateReasoningEffortMappings([row])).toEqual({});
  });

  it("treats prefix without a model as a global mapping", () => {
    const row = createReasoningEffortMappingRow({
      from: "max",
      to: "low",
      match_type: "prefix",
    });
    expect(validateReasoningEffortMappings([row])).toEqual({});
    expect(reasoningEffortMappingsToAPI([row])).toEqual([
      { from: "max", to: "low" },
    ]);
  });

  it("rejects custom mappings", () => {
    const row = createReasoningEffortMappingRow({ from: "ultra", to: "high" });
    expect(validateReasoningEffortMappings([row])).toEqual({
      [row.pairs[0].id]: { from: "unsupportedFrom" },
    });
  });

  it("hydrates none mapping values", () => {
    const rows = reasoningEffortMappingsToRows(
      [{ from: "none", to: "low", match_type: "exact", model: "gpt-6-astra" }],
    );

    expect(reasoningEffortMappingsToAPI(rows)).toEqual([
      { from: "none", to: "low", match_type: "exact", model: "gpt-6-astra" },
    ]);
  });
});
