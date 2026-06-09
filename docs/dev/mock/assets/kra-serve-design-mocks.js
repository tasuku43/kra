const root = document.body;
const nav = document.getElementById("workspaceNav");
const board = document.getElementById("kanbanBoard");
const themeToggle = document.getElementById("themeToggle");
const themeLabel = document.getElementById("themeLabel");
const designButtons = Array.from(document.querySelectorAll("[data-design-option]"));

function progressCounts(ws) {
  const done = (ws.tasks.done || []).length;
  const total = Object.values(ws.tasks).reduce((sum, tasks) => sum + tasks.length, 0);
  const percent = total === 0 ? 0 : Math.round((done / total) * 100);
  return { done, total, percent };
}

function riskDot(ws) {
  if (ws.riskClass === "danger") return "dot danger";
  if (ws.riskClass === "warn") return "dot warn";
  return "dot";
}

function renderNav() {
  nav.innerHTML = Object.values(KRA_WORKSPACES).map((ws) => {
    const progress = progressCounts(ws);
    return `
      <a class="workspace-link" href="${workspaceHref(ws.id, true)}">
        <span class="${riskDot(ws)}"></span>
        <span class="workspace-link-text">
          <span class="workspace-link-id">${ws.id}</span>
          <span class="workspace-link-title">${ws.title}</span>
          <span class="progress-mini" aria-label="task progress">
            <span class="progress-mini-fill" style="width: ${progress.percent}%"></span>
          </span>
        </span>
      </a>
    `;
  }).join("");
}

function renderTaskCards(ws, status) {
  const tasks = ws.tasks[status] || [];
  if (tasks.length === 0) {
    return `<span class="small empty-column">No cards</span>`;
  }
  return tasks.map(([id, title]) => `
    <a class="task-card" href="${workspaceHref(ws.id, true)}">
      ${title}
      <span class="small">${id}</span>
    </a>
  `).join("");
}

function renderSwimlane(ws) {
  const statuses = ["todo", "doing", "blocked", "done"];
  const progress = progressCounts(ws);
  return `
    <article class="swimlane">
      <header class="swimlane-head">
        <div>
          <a class="workspace-id" href="${workspaceHref(ws.id, true)}">${ws.id}</a>
          <p class="workspace-title">${ws.title}</p>
        </div>
        <div class="progress" aria-label="task progress">
          <div class="progress-meta">
            <span>Progress</span>
            <span>${progress.done} / ${progress.total} done</span>
          </div>
          <div class="progress-track">
            <span class="progress-fill" style="width: ${progress.percent}%"></span>
          </div>
        </div>
      </header>
      <div class="status-grid">
        ${statuses.map((status) => `
          <section class="status-column" data-status="${status}">
            <div class="column-title">
              <span>${KRA_STATUS_LABELS[status]}</span>
              <span>${(ws.tasks[status] || []).length}</span>
            </div>
            ${renderTaskCards(ws, status)}
          </section>
        `).join("")}
      </div>
    </article>
  `;
}

function setDesign(design) {
  root.dataset.design = design;
  designButtons.forEach((button) => {
    button.classList.toggle("active", button.dataset.designOption === design);
  });
}

function setTheme(theme) {
  root.dataset.theme = theme;
  const dark = theme === "dark";
  themeToggle.setAttribute("aria-pressed", String(dark));
  themeLabel.textContent = dark ? "Dark" : "Light";
}

designButtons.forEach((button) => {
  button.addEventListener("click", () => setDesign(button.dataset.designOption));
});

themeToggle.addEventListener("click", () => {
  setTheme(root.dataset.theme === "dark" ? "light" : "dark");
});

renderNav();
board.innerHTML = Object.values(KRA_WORKSPACES).map(renderSwimlane).join("");
