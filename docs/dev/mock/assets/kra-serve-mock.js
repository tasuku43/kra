const KRA_WORKSPACES = {
  "PROJ-2314": {
    id: "PROJ-2314",
    title: "Serve dashboard spec",
    risk: "unpushed",
    riskClass: "warn",
    tasks: {
      todo: [
        ["TASK-003", "Finalize the HTML shell"],
        ["TASK-004", "Update the spec status"]
      ],
      doing: [
        ["TASK-002", "Define the API contract"]
      ],
      blocked: [],
      done: [
        ["TASK-001", "Align the product experience"]
      ]
    },
    readme: {
      heading: "Serve dashboard spec",
      body: "Design workspace for serving open workspace boards in the browser with kra serve.",
      bullets: [
        "The MVP is read-only",
        "Only open workspaces are shown",
        "workspace.md remains the source of truth managed by coding agents"
      ]
    },
    repos: [
      { name: "kra", branch: "feature/PROJ-2314/serve-dashboard", pr: "#128", url: "https://github.com/tasuku43/kra/pull/128" },
      { name: "docs-site", branch: "docs/serve-dashboard", pr: "none", url: "" }
    ]
  },
  "PROJ-2315": {
    id: "PROJ-2315",
    title: "Workspace task polish",
    risk: "clean",
    riskClass: "ok",
    tasks: {
      todo: [
        ["TASK-004", "Update the docs"]
      ],
      doing: [
        ["TASK-003", "Wait for review"]
      ],
      blocked: [],
      done: [
        ["TASK-001", "Integrate task summary"],
        ["TASK-002", "Polish status rendering"]
      ]
    },
    readme: {
      heading: "Workspace task polish",
      body: "Workspace for polishing task rendering and documentation language around workspace.md.",
      bullets: [
        "Task summary is already integrated into the existing dashboard",
        "Only review follow-up remains",
        "Archive candidate after completion"
      ]
    },
    repos: [
      { name: "kra", branch: "feature/PROJ-2315/task-polish", pr: "#126", url: "https://github.com/tasuku43/kra/pull/126" }
    ]
  },
  "OPS-044": {
    id: "OPS-044",
    title: "Close safety regression",
    risk: "dirty",
    riskClass: "danger",
    tasks: {
      todo: [
        ["TASK-004", "Fix the guard test"]
      ],
      doing: [
        ["TASK-003", "Investigate the CI failure"]
      ],
      blocked: [
        ["TASK-002", "Confirm the staging allowlist spec"]
      ],
      done: [
        ["TASK-001", "Create a reproduction case"]
      ]
    },
    readme: {
      heading: "Close safety regression",
      body: "Workspace for checking whether close-command user-data commit scope regressed.",
      bullets: [
        "Re-check the staging allowlist",
        "Inspect the CI failure first",
        "Review the dirty repo before closing"
      ]
    },
    repos: [
      { name: "kra", branch: "fix/OPS-044-close-safety", pr: "#129", url: "https://github.com/tasuku43/kra/pull/129" },
      { name: "automation", branch: "main", pr: "none", url: "" }
    ]
  },
  "DOC-018": {
    id: "DOC-018",
    title: "Public guide refresh",
    risk: "clean",
    riskClass: "ok",
    tasks: {
      todo: [],
      doing: [],
      blocked: [],
      done: [
        ["TASK-001", "Update README"],
        ["TASK-002", "Review the guide"],
        ["TASK-003", "Organize artifacts"]
      ]
    },
    readme: {
      heading: "Public guide refresh",
      body: "Workspace for completed public guide copy updates.",
      bullets: [
        "risk clean",
        "tasks done",
        "Ready to archive after checking README and artifacts"
      ]
    },
    repos: [
      { name: "kra", branch: "docs/DOC-018-public-guide", pr: "#124", url: "https://github.com/tasuku43/kra/pull/124" }
    ]
  }
};

const KRA_STATUS_LABELS = {
  todo: "Todo",
  doing: "Doing",
  blocked: "Blocked",
  done: "Done"
};

function workspaceHref(id, fromRoot) {
  return fromRoot ? `${id}/` : `/workspaces/${id}/`;
}
