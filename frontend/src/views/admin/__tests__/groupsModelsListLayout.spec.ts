import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, resolve } from "node:path";

import { describe, expect, it } from "vitest";

const currentDir = dirname(fileURLToPath(import.meta.url));
const groupsViewSource = readFileSync(
  resolve(currentDir, "../GroupsView.vue"),
  "utf8",
);

const listSource = readFileSync(resolve(currentDir, "../../../components/admin/group/GroupModelsListFields.vue"), "utf8");

describe("groups models list layout", () => {
  it("keeps the toolbar outside of the scrolling list content", () => {
    expect(listSource).toContain("overflow-hidden rounded-surface border");
    expect(listSource).toContain("max-h-64 divide-y divide-gray-200 overflow-y-auto");
    expect(groupsViewSource).not.toContain("sticky top-0");
  });

  it("uses a wide dialog with a single form-owned scroll area", () => {
    expect(groupsViewSource).toContain('width="wide"');
    expect(groupsViewSource).toContain(
      ':body-scroll="false"',
    );
  });

  it("uses the unified models endpoint in group configuration", () => {
    expect(listSource).toContain("endpoint: '/v1/models'");
    expect(groupsViewSource).not.toContain('modelsListEndpoint(createForm.platform)');
  });
});
