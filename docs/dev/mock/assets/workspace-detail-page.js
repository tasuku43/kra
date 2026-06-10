const currentWorkspaceID = document.body.dataset.workspace;
const currentWorkspace = KRA_WORKSPACES[currentWorkspaceID];
const nav = document.getElementById("workspaceNav");
const detail = document.getElementById("workspaceDetail");

function detailRiskDot(ws) {
  if (ws.riskClass === "danger") return "dot danger";
  if (ws.riskClass === "warn") return "dot warn";
  return "dot";
}

function renderDetailNav() {
  nav.innerHTML = Object.values(KRA_WORKSPACES).map((ws) => {
    const progress = taskProgress(ws);
    return `
    <a class="workspace-link ${ws.id === currentWorkspaceID ? "active" : ""}" href="../${ws.id}/">
      <span class="${detailRiskDot(ws)}"></span>
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

function renderReadme(ws) {
  return `
    <article class="readme">
      <h2>${ws.readme.heading}</h2>
      <p>${ws.readme.body}</p>
      <ul>${ws.readme.bullets.map((item) => `<li>${item}</li>`).join("")}</ul>
    </article>
  `;
}

function renderRepos(ws) {
  return `
    <div class="repo-table">
      <div class="repo-row header">
        <span>Repository</span>
        <span>Branch</span>
        <span>Pull request</span>
      </div>
      ${ws.repos.map((repo) => `
        <div class="repo-row">
          <strong>${repo.name}</strong>
          <span>${repo.branch}</span>
          <span>${repo.url ? `<a href="${repo.url}" target="_blank" rel="noreferrer">${repo.pr}</a>` : repo.pr}</span>
        </div>
      `).join("")}
    </div>
  `;
}

function renderTaskCards(ws, status) {
  const tasks = ws.tasks[status] || [];
  if (tasks.length === 0) {
    return `<span class="small">No cards</span>`;
  }
  return tasks.map(([id, title]) => `
    <div class="task-card">
      ${title}
      <span class="small">${id}</span>
    </div>
  `).join("");
}

function renderWorkspaceBoard(ws) {
  const statuses = ["todo", "doing", "blocked", "done"];
  const progress = taskProgress(ws);
  return `
    <section class="detail-board" aria-label="workspace board">
      <header class="detail-board-head">
        <div class="detail-board-title">
          <button class="collapse-btn" type="button" aria-expanded="true" aria-controls="detail-board-grid">
            <span class="collapse-icon" aria-hidden="true"></span>
          </button>
          <div>
            <h2>Board</h2>
            <p class="meta">current workspace task flow</p>
          </div>
        </div>
        <span class="detail-board-summary">${progress.done} / ${progress.total} done</span>
      </header>
      <div class="status-grid" id="detail-board-grid">
        ${statuses.map((status) => `
          <section class="status-column">
            <div class="column-title">
              <span>${KRA_STATUS_LABELS[status]}</span>
              <span>${(ws.tasks[status] || []).length}</span>
            </div>
            ${renderTaskCards(ws, status)}
          </section>
        `).join("")}
      </div>
    </section>
  `;
}

function activateTab(name) {
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.classList.toggle("active", tab.getAttribute("href") === `#${name}`);
  });
  document.querySelectorAll(".tab-panel").forEach((panel) => {
    panel.classList.toggle("active", panel.id === name);
  });
}

function renderDetail(ws) {
  const progress = taskProgress(ws);
  detail.innerHTML = `
    <div class="detail-header">
      <div>
        <strong class="workspace-id">${ws.id}</strong>
        <p class="workspace-title">${ws.title}</p>
      </div>
      <div class="progress" aria-label="task progress">
        <div class="progress-meta">
          <span>Progress</span>
          <span>${progress.done} / ${progress.total} done</span>
        </div>
        <div class="progress-track">
          <div class="progress-fill" style="width: ${progress.percent}%"></div>
        </div>
      </div>
    </div>
    ${renderWorkspaceBoard(ws)}
    <div class="tabs">
      <a class="tab active" href="#readme">README</a>
      <a class="tab" href="#repositories">Repositories</a>
    </div>
    <div class="tab-panel active default" id="readme">${renderReadme(ws)}</div>
    <div class="tab-panel" id="repositories">${renderRepos(ws)}</div>
  `;
  detail.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => activateTab(tab.getAttribute("href").slice(1)));
  });
  const boardToggle = detail.querySelector(".detail-board .collapse-btn");
  const boardSection = detail.querySelector(".detail-board");
  boardToggle.addEventListener("click", () => {
    const collapsed = boardSection.classList.toggle("collapsed");
    boardToggle.setAttribute("aria-expanded", String(!collapsed));
  });
  activateTab(window.location.hash === "#repositories" ? "repositories" : "readme");
}

renderDetailNav();
renderDetail(currentWorkspace);
