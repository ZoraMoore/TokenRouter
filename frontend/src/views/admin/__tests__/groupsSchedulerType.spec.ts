import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

const settingsSource = readFileSync(resolve(currentDir, "../../../components/admin/group/GroupSettingsForm.vue"), "utf8");

describe("groups scheduler type", () => {
  it("uses the shared Select control and preserves the scheduler mode in both forms", () => {
    expect(settingsSource).toContain('v-model="form.scheduler_type"');
    expect(groupsViewSource).toContain(':model-value="editForm"');
    expect(settingsSource).toContain(':options="schedulerOptions"');
    expect(groupsViewSource).toContain('scheduler_type: "basic" as GroupSchedulerType');
    expect(groupsViewSource).toContain('group.scheduler_type ?? "basic"');
    expect(groupsViewSource).toContain('advanced_scheduler_overrides');
    expect(groupsViewSource).toContain('GroupAdvancedSchedulerOverridesModal');
    expect(groupsViewSource).not.toMatch(/<select\b/);
  });
});
