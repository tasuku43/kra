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
  nav.innerHTML = Object.values(KRA_WORKSPACES).map((ws) => `
    <a class="workspace-link ${ws.id === currentWorkspaceID ? "active" : ""}" href="../${ws.id}/">
      <span class="${detailRiskDot(ws)}"></span>${ws.id}
    </a>
  `).join("");
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
  return `
    <section class="detail-board" aria-label="workspace board">
      <div class="status-grid">
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
    tab.classList.toggle("active", tab.dataset.tab === name);
  });
  document.querySelectorAll(".tab-panel").forEach((panel) => {
    panel.classList.toggle("active", panel.id === `${name}Panel`);
  });
}

function renderDetail(ws) {
  detail.innerHTML = `
    <div class="detail-header">
      <div>
        <div class="detail-title">${ws.id}</div>
        <p class="workspace-title">${ws.title}</p>
      </div>
      <span class="badge ${ws.riskClass}">${ws.risk}</span>
    </div>
    ${renderWorkspaceBoard(ws)}
    <div class="tabs">
      <button class="tab active" type="button" data-tab="readme">README</button>
      <button class="tab" type="button" data-tab="repositories">Repositories</button>
    </div>
    <div class="tab-panel active" id="readmePanel">${renderReadme(ws)}</div>
    <div class="tab-panel" id="repositoriesPanel">${renderRepos(ws)}</div>
  `;
  detail.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => activateTab(tab.dataset.tab));
  });
}

renderDetailNav();
renderDetail(currentWorkspace);
