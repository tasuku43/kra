const root = document.body;
const fileTree = document.getElementById("fileTree");
const mock = document.getElementById("fileViewMock");
const ws = KRA_WORKSPACES["PROJ-2314"];

const FILES = {
  "workspace.md": {
    title: "workspace.md",
    meta: "source of truth",
    body: [
      "# PROJ-2314 Serve dashboard spec",
      "",
      "## Tasks",
      "- [ ] TASK-003 Finalize the HTML shell",
      "- [ ] TASK-004 Update the spec status",
      "- [x] TASK-001 Align the product experience",
      "",
      "## Notes",
      "kra serve renders open workspaces as a read-only browser board."
    ]
  },
  "README.md": {
    title: "README.md",
    meta: "workspace notes",
    body: [
      "# Serve dashboard spec",
      "",
      "Design workspace for serving open workspace boards in the browser.",
      "",
      "- The MVP is read-only",
      "- Only open workspaces are shown",
      "- workspace.md remains the source of truth"
    ]
  },
  "docs/dev/spec/commands/serve.md": {
    title: "docs/dev/spec/commands/serve.md",
    meta: "command spec",
    body: [
      "# kra serve",
      "",
      "The command starts a local dashboard for browsing workspaces.",
      "",
      "The detail page should expose task state, files, and repo links without creating a heavy IDE surface."
    ]
  },
  "docs/dev/mock/kra-serve-dashboard.html": {
    title: "docs/dev/mock/kra-serve-dashboard.html",
    meta: "mock entry",
    body: [
      "<!doctype html>",
      "<html lang=\"ja\">",
      "<head>",
      "  <title>kra serve mock</title>",
      "</head>",
      "<body>",
      "  <main id=\"workspaceDetail\"></main>",
      "</body>",
      "</html>"
    ]
  },
  "internal/cli/serve.go": {
    title: "internal/cli/serve.go",
    meta: "handler",
    body: [
      "package cli",
      "",
      "func runServe() error {",
      "    // Starts the local read-only workspace dashboard.",
      "    return nil",
      "}"
    ]
  },
  "internal/cli/serve_test.go": {
    title: "internal/cli/serve_test.go",
    meta: "tests",
    body: [
      "package cli",
      "",
      "func TestServeDashboard(t *testing.T) {",
      "    t.Run(\"renders workspace detail\", func(t *testing.T) {})",
      "}"
    ]
  },
  "scripts/lint-ui-color.sh": {
    title: "scripts/lint-ui-color.sh",
    meta: "quality gate",
    body: [
      "#!/usr/bin/env bash",
      "set -euo pipefail",
      "",
      "rg 'lipgloss.Color|\\\\x1b\\\\[' internal/cli && exit 1",
      "exit 0"
    ]
  }
};

const TREE = [
  { type: "file", name: "workspace.md", path: "workspace.md" },
  { type: "file", name: "README.md", path: "README.md" },
  { type: "special", name: "repos", path: "repos/" },
  {
    type: "dir",
    name: "docs",
    path: "docs",
    children: [
      {
        type: "dir",
        name: "dev",
        path: "docs/dev",
        children: [
          {
            type: "dir",
            name: "spec",
            path: "docs/dev/spec",
            children: [
              {
                type: "dir",
                name: "commands",
                path: "docs/dev/spec/commands",
                children: [
                  { type: "file", name: "serve.md", path: "docs/dev/spec/commands/serve.md" }
                ]
              }
            ]
          },
          {
            type: "dir",
            name: "mock",
            path: "docs/dev/mock",
            children: [
              { type: "file", name: "kra-serve-dashboard.html", path: "docs/dev/mock/kra-serve-dashboard.html" }
            ]
          }
        ]
      }
    ]
  },
  {
    type: "dir",
    name: "internal",
    path: "internal",
    children: [
      {
        type: "dir",
        name: "cli",
        path: "internal/cli",
        children: [
          { type: "file", name: "serve.go", path: "internal/cli/serve.go" },
          { type: "file", name: "serve_test.go", path: "internal/cli/serve_test.go" }
        ]
      }
    ]
  },
  {
    type: "dir",
    name: "scripts",
    path: "scripts",
    children: [
      { type: "file", name: "lint-ui-color.sh", path: "scripts/lint-ui-color.sh" }
    ]
  }
];

let openFiles = ["workspace.md", "README.md", "docs/dev/spec/commands/serve.md"];
let selectedFile = "workspace.md";
let collapsedDirs = new Set(["docs/dev/mock", "scripts"]);

function progressCounts(workspace) {
  const done = (workspace.tasks.done || []).length;
  const total = Object.values(workspace.tasks).reduce((sum, tasks) => sum + tasks.length, 0);
  const percent = total === 0 ? 0 : Math.round((done / total) * 100);
  return { done, total, percent };
}

function renderTreeNode(node, depth = 0) {
  const indent = `style="--depth: ${depth}"`;
  if (node.type === "special") {
    return `
      <button type="button" class="tree-row special ${selectedFile === node.path ? "active" : ""}" ${indent} data-open="${node.path}">
        <span class="special-folder-icon" aria-hidden="true"></span>
        <span>${node.name}/</span>
        <span class="open-marker">view</span>
      </button>
    `;
  }
  if (node.type === "file") {
    return `
      <button type="button" class="tree-row file ${selectedFile === node.path ? "active" : ""}" ${indent} data-file="${node.path}">
        <span class="file-dot" aria-hidden="true"></span>
        <span>${node.name}</span>
      </button>
    `;
  }
  const collapsed = collapsedDirs.has(node.path);
  return `
    <div class="tree-dir">
      <button type="button" class="tree-row folder ${collapsed ? "collapsed" : ""}" ${indent} data-folder="${node.path}" aria-expanded="${String(!collapsed)}">
        <span class="folder-chevron" aria-hidden="true"></span>
        <span class="folder-icon" aria-hidden="true"></span>
        <span>${node.name}</span>
      </button>
      <div class="tree-children ${collapsed ? "hidden" : ""}">
        ${node.children.map((child) => renderTreeNode(child, depth + 1)).join("")}
      </div>
    </div>
  `;
}

function renderFileTree() {
  fileTree.innerHTML = `
    <div class="tree-head">
      <span class="tree-root">workspace root</span>
      <span class="small">${Object.keys(FILES).length} files</span>
    </div>
    <div class="tree-list">
      ${TREE.map((node) => renderTreeNode(node)).join("")}
    </div>
  `;
  fileTree.querySelectorAll("[data-file]").forEach((control) => {
    control.addEventListener("click", () => selectFile(control.dataset.file));
  });
  fileTree.querySelectorAll("[data-open]").forEach((control) => {
    control.addEventListener("click", () => selectFile(control.dataset.open));
  });
  fileTree.querySelectorAll("[data-folder]").forEach((control) => {
    control.addEventListener("click", () => toggleFolder(control.dataset.folder));
  });
}

function renderFileTabs() {
  return `
    <div class="file-tabs" role="tablist" aria-label="open files">
      ${openFiles.map((name) => `
        <button type="button" class="file-tab ${name === selectedFile ? "active" : ""}" data-file="${name}">
          <span>${name}</span>
          <span class="tab-close" aria-hidden="true" data-close-file="${name}"></span>
        </button>
      `).join("")}
    </div>
  `;
}

function escapeHTML(value) {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;");
}

function renderMarkdown(lines) {
  const blocks = [];
  let listItems = [];
  const flushList = () => {
    if (listItems.length === 0) return;
    blocks.push(`<ul>${listItems.map((item) => `<li>${escapeHTML(item)}</li>`).join("")}</ul>`);
    listItems = [];
  };
  lines.forEach((line) => {
    if (line.startsWith("# ")) {
      flushList();
      blocks.push(`<h1>${escapeHTML(line.slice(2))}</h1>`);
    } else if (line.startsWith("## ")) {
      flushList();
      blocks.push(`<h2>${escapeHTML(line.slice(3))}</h2>`);
    } else if (line.startsWith("- ")) {
      listItems.push(line.slice(2));
    } else if (line.trim() === "") {
      flushList();
    } else {
      flushList();
      blocks.push(`<p>${escapeHTML(line)}</p>`);
    }
  });
  flushList();
  return `<article class="rendered-markdown">${blocks.join("")}</article>`;
}

function renderWorkspaceBoardView() {
  const statuses = ["todo", "doing", "blocked", "done"];
  return `
    <section class="workspace-board-view" aria-label="workspace.md board view">
      <div class="view-intro">
        <div>
          <h2>Board from workspace.md</h2>
          <p class="meta">source-backed task state rendered as a workspace board</p>
        </div>
        <button type="button" class="secondary-btn">View source</button>
      </div>
      <div class="status-grid">
        ${statuses.map((status) => `
          <section class="status-column">
            <div class="column-title">
              <span>${KRA_STATUS_LABELS[status]}</span>
              <span>${(ws.tasks[status] || []).length}</span>
            </div>
            ${(ws.tasks[status] || []).length === 0 ? `<span class="small">No cards</span>` : ws.tasks[status].map(([id, title]) => `
              <div class="task-card">
                ${title}
                <span class="small">${id}</span>
              </div>
            `).join("")}
          </section>
        `).join("")}
      </div>
    </section>
  `;
}

function renderHTMLPreview(file) {
  return `
    <section class="html-preview">
      <div class="preview-toolbar">
        <span></span>
        <span></span>
        <span></span>
        <strong>${file.title}</strong>
      </div>
      <div class="preview-frame">
        <div class="preview-page">
          <h1>kra serve mock</h1>
          <p>The workspace detail shell renders here as a browser preview.</p>
          <div class="preview-card">
            <strong>PROJ-2314</strong>
            <span>Serve dashboard spec</span>
          </div>
        </div>
      </div>
      <details class="source-details">
        <summary>Source</summary>
        <pre>${escapeHTML(file.body.join("\n"))}</pre>
      </details>
    </section>
  `;
}

function renderCodeSource(file) {
  return `<pre class="code-source">${escapeHTML(file.body.join("\n"))}</pre>`;
}

function renderReposView() {
  return `
    <section class="repo-overview">
      <div class="view-intro">
        <div>
          <h2>Repositories</h2>
          <p class="meta">repository context opened from repos/</p>
        </div>
        <span class="badge">${ws.repos.length} linked</span>
      </div>
      <div class="repo-grid">
        ${ws.repos.map((repo) => `
          <article class="repo-card large">
            <strong>${repo.name}</strong>
            <span class="repo-branch">branch ${repo.branch}</span>
            <div class="repo-pr-list">
              ${(repo.pullRequests || [{ title: repo.pr, url: repo.url, state: repo.url ? "open" : "unknown", meta: repo.url ? "current branch" : "" }]).filter((pr) => pr.title && pr.title !== "none").length === 0 ? `<span class="empty">No matching pull requests.</span>` : (repo.pullRequests || [{ title: repo.pr, url: repo.url, state: repo.url ? "open" : "unknown", meta: repo.url ? "current branch" : "" }]).filter((pr) => pr.title && pr.title !== "none").map((pr) => `
                <a class="repo-pr" href="${pr.url || "#"}">
                  <span class="repo-pr-main">
                    <span class="repo-pr-title">${pr.title}</span>
                    <span class="repo-pr-meta">${pr.meta || "title match"}</span>
                  </span>
                  <span class="pr-state pr-state-${pr.state || "unknown"}">${pr.state || "unknown"}</span>
                </a>
              `).join("")}
            </div>
          </article>
        `).join("")}
      </div>
    </section>
  `;
}

function renderViewerBody() {
  if (selectedFile === "workspace.md") return renderWorkspaceBoardView();
  if (selectedFile === "repos/") return renderReposView();
  const file = FILES[selectedFile];
  if (selectedFile.endsWith(".md")) return renderMarkdown(file.body);
  if (selectedFile.endsWith(".html")) return renderHTMLPreview(file);
  return renderCodeSource(file);
}

function viewerMeta() {
  if (selectedFile === "workspace.md") return { title: "workspace.md", meta: "board view", badge: "workspace" };
  if (selectedFile === "repos/") return { title: "repos/", meta: "repository overview", badge: "special view" };
  const file = FILES[selectedFile];
  if (selectedFile.endsWith(".md")) return { title: file.title, meta: "rendered markdown", badge: "markdown" };
  if (selectedFile.endsWith(".html")) return { title: file.title, meta: "rendered preview", badge: "html" };
  return { title: file.title, meta: file.meta, badge: "source" };
}

function renderFileViewer() {
  const view = viewerMeta();
  return `
    <section class="file-viewer" aria-label="selected file">
      ${renderFileTabs()}
      <header class="viewer-head">
        <div>
          <h2>${view.title}</h2>
          <p class="meta">${view.meta}</p>
        </div>
        <span class="badge">${view.badge}</span>
      </header>
      <div class="viewer-body">
        ${renderViewerBody()}
      </div>
    </section>
  `;
}

function renderHeader() {
  const progress = progressCounts(ws);
  return `
    <div class="detail-header">
      <div>
        <strong class="workspace-id">${ws.id}</strong>
        <p class="workspace-title">${ws.title}</p>
      </div>
      <div class="header-meta">
        <span>${progress.done} / ${progress.total} done</span>
        <span>${ws.risk}</span>
      </div>
      <p class="layout-note">Open files and workspace views as tabs.</p>
    </div>
  `;
}

function renderTabsColumn() {
  return `
    ${renderHeader()}
    <div class="editor-main">
      ${renderFileViewer()}
    </div>
  `;
}

function renderMock() {
  root.dataset.layout = "tabs";
  mock.innerHTML = renderTabsColumn();
  bindFileControls();
  renderFileTree();
}

function selectFile(name) {
  selectedFile = name;
  if (!openFiles.includes(name)) {
    openFiles = [...openFiles, name];
  }
  renderMock();
}

function closeFile(name) {
  if (openFiles.length === 1) return;
  const index = openFiles.indexOf(name);
  openFiles = openFiles.filter((file) => file !== name);
  if (selectedFile === name) {
    selectedFile = openFiles[Math.max(0, index - 1)];
  }
  renderMock();
}

function toggleFolder(path) {
  if (collapsedDirs.has(path)) {
    collapsedDirs.delete(path);
  } else {
    collapsedDirs.add(path);
  }
  renderFileTree();
}

function bindFileControls() {
  mock.querySelectorAll("[data-file]").forEach((control) => {
    control.addEventListener("click", () => selectFile(control.dataset.file));
  });
  mock.querySelectorAll("[data-close-file]").forEach((control) => {
    control.addEventListener("click", (event) => {
      event.stopPropagation();
      closeFile(control.dataset.closeFile);
    });
  });
}

renderMock();
