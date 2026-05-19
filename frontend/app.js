// TeamBoard Frontend (Vanilla JS)
const API = "/api";
const TOKEN_KEY = "tb_token";

const state = {
  user: null,
  projects: [],
  currentProjectId: null,
  boards: [],
  documents: [],
  members: [],
  boardTaskCounts: {},
  currentBoardId: null,
  columns: [],
  tasks: [],
  currentTaskId: null,
  currentTask: null,
};

// ---------- HTTP helper ----------
async function api(path, { method = "GET", body = null, isForm = false } = {}) {
  const headers = {};
  const token = localStorage.getItem(TOKEN_KEY);
  if (token) headers["Authorization"] = `Bearer ${token}`;
  if (body && !isForm) headers["Content-Type"] = "application/json";

  const res = await fetch(`${API}${path}`, {
    method,
    headers,
    body: isForm ? body : body ? JSON.stringify(body) : null,
  });
  if (res.status === 204) return null;
  const text = await res.text();
  let data;
  try { data = text ? JSON.parse(text) : null; } catch { data = text; }
  if (!res.ok) {
    const detail = (data && data.detail) || res.statusText;
    throw new Error(typeof detail === "string" ? detail : JSON.stringify(detail));
  }
  return data;
}

// ---------- DOM helpers ----------
const $ = (sel) => document.querySelector(sel);
const $$ = (sel) => Array.from(document.querySelectorAll(sel));

function show(el) { el.classList.remove("hidden"); }
function hide(el) { el.classList.add("hidden"); }

function statusForColumnName(name) {
  return String(name || "Backlog");
}

function memberLabel(userId) {
  if (!userId) return "Unassigned";
  const member = state.members.find((m) => m.user_id === userId);
  return member?.display_name || member?.email || shortId(userId);
}

function columnForTask(task) {
  return state.columns.find((column) => column.id === task.column_id);
}

function taskStatusLabel(task) {
  return columnForTask(task)?.name || task.status || "Backlog";
}

// ---------- Auth ----------
function setupAuth() {
  $$(".tab").forEach((t) =>
    t.addEventListener("click", () => {
      $$(".tab").forEach((x) => x.classList.remove("active"));
      t.classList.add("active");
      if (t.dataset.tab === "login") {
        show($("#login-form")); hide($("#register-form"));
      } else {
        hide($("#login-form")); show($("#register-form"));
      }
    })
  );

  $("#login-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      const res = await api("/auth/login", {
        method: "POST",
        body: { email: fd.get("email"), password: fd.get("password") },
      });
      localStorage.setItem(TOKEN_KEY, res.access_token);
      await afterLogin();
    } catch (err) { $("#auth-error").textContent = err.message; }
  });

  $("#register-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    try {
      await api("/auth/register", {
        method: "POST",
        body: {
          email: fd.get("email"),
          password: fd.get("password"),
          display_name: fd.get("display_name"),
        },
      });
      const res = await api("/auth/login", {
        method: "POST",
        body: { email: fd.get("email"), password: fd.get("password") },
      });
      localStorage.setItem(TOKEN_KEY, res.access_token);
      await afterLogin();
    } catch (err) { $("#auth-error").textContent = err.message; }
  });
}

async function loadServiceHealth() {
  const el = $("#service-health");
  try {
    const result = await fetch("/health/services");
    const data = await result.json();
    el.classList.remove("ok", "degraded", "error");
    el.classList.add(data.status === "ok" ? "ok" : "degraded");
    const services = Object.entries(data.services || {})
      .map(([name, item]) => `${name}:${item.status}`)
      .join(" · ");
    el.textContent = `Services: ${data.status}`;
    el.title = services;
  } catch (err) {
    el.classList.remove("ok", "degraded");
    el.classList.add("error");
    el.textContent = "Services: error";
    el.title = err.message;
  }
}

async function afterLogin() {
  state.user = await api("/auth/me");
  $("#auth-error").textContent = "";
  hide($("#auth-view"));
  show($("#app-view"));
  $("#user-bar").innerHTML = `
    Hi, ${escapeHtml(state.user.display_name)}
    <button id="logout-btn">Logout</button>
  `;
  $("#logout-btn").addEventListener("click", () => {
    localStorage.removeItem(TOKEN_KEY);
    location.reload();
  });
  await loadProjects();
}

function escapeHtml(s) {
  return String(s ?? "").replace(/[&<>"']/g, (c) => ({
    "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
  })[c]);
}

// ---------- Projects ----------
async function loadProjects() {
  state.projects = await api("/projects/projects");
  renderProjects();
}

function renderProjects() {
  const ul = $("#project-list");
  ul.innerHTML = "";
  state.projects.forEach((p) => {
    const li = document.createElement("li");
    li.textContent = p.name;
    li.dataset.id = p.id;
    if (p.id === state.currentProjectId) li.classList.add("active");
    li.addEventListener("click", () => selectProject(p.id));
    ul.appendChild(li);
  });
}

async function selectProject(id) {
  state.currentProjectId = id;
  state.currentBoardId = null;
  state.boardTaskCounts = {};
  $("#ai-summary-result").textContent = "No summary generated yet.";
  $("#ai-summary-result").classList.add("muted");
  $("#ai-focus").value = "";
  $("#task-search").value = "";
  $("#task-assignee-filter").value = "";
  $("#task-status-filter").value = "";
  $("#task-sprint-filter").value = "";
  renderProjects();
  const p = state.projects.find((x) => x.id === id);
  show($("#project-view"));
  hide($("#empty-hint"));
  hide($("#board-view"));
  $("#project-title").textContent = p.name;
  $("#project-description").textContent = p.description || "";
  renderProjectStats({ loading: true });
  await Promise.all([loadBoards(id), loadDocuments(id), loadMembers(id)]);
  await refreshProjectStats();
}

// ---------- Boards ----------
async function loadBoards(projectId) {
  state.boards = await api(`/projects/projects/${projectId}/boards`);
  renderBoardList();
}

function renderBoardList() {
  const root = $("#board-list");
  root.innerHTML = "";
  if (!state.boards.length) {
    root.innerHTML = `<p class="empty-state">No boards yet. Create a Kanban board to start planning.</p>`;
    return;
  }
  state.boards.forEach((b) => {
    const card = document.createElement("button");
    card.type = "button";
    card.className = "board-card";
    if (b.id === state.currentBoardId) card.classList.add("active");
    card.innerHTML = `
      <span class="badge">${escapeHtml(b.type)}</span>
      <strong>${escapeHtml(b.name)}</strong>
      <small>${state.boardTaskCounts[b.id] ?? "—"} tasks</small>
    `;
    card.addEventListener("click", () => selectBoard(b.id, b.name));
    root.appendChild(card);
  });
}

async function selectBoard(boardId, boardName) {
  state.currentBoardId = boardId;
  $("#board-title").textContent = boardName;
  show($("#board-view"));
  renderBoardList();
  await renderBoard();
}

async function renderBoard() {
  const [columns, tasks] = await Promise.all([
    api(`/projects/boards/${state.currentBoardId}/columns`),
    api(`/tasks/boards/${state.currentBoardId}/tasks`),
  ]);
  state.columns = columns;
  state.tasks = tasks;
  state.boardTaskCounts[state.currentBoardId] = tasks.length;
  renderBoardList();
  renderBoardFilters();
  await refreshProjectStats(false);

  const root = $("#kanban");
  root.innerHTML = "";
  const query = $("#task-search").value.trim().toLowerCase();
  const assigneeFilter = $("#task-assignee-filter").value;
  const statusFilter = $("#task-status-filter").value;
  const sprintFilter = $("#task-sprint-filter").value;
  columns.forEach((col) => {
    const colEl = document.createElement("div");
    colEl.className = "column";
    const columnTasks = tasks.filter((t) => {
      if (t.column_id !== col.id) return false;
      if (assigneeFilter === "__unassigned" && t.assignee_id) return false;
      if (assigneeFilter && assigneeFilter !== "__unassigned" && t.assignee_id !== assigneeFilter) return false;
      if (statusFilter && t.column_id !== statusFilter) return false;
      if (sprintFilter === "__none" && t.sprint) return false;
      if (sprintFilter && sprintFilter !== "__none" && t.sprint !== sprintFilter) return false;
      if (!query) return true;
      return [t.title, t.description, taskStatusLabel(t), memberLabel(t.assignee_id), t.sprint]
        .filter(Boolean)
        .some((value) => String(value).toLowerCase().includes(query));
    });
    colEl.innerHTML = `
      <div class="column-header">
        <h4>${escapeHtml(col.name)}</h4>
        <span>${columnTasks.length}</span>
      </div>
    `;

    columnTasks.forEach((t) => colEl.appendChild(renderTask(t)));

    const form = document.createElement("form");
    form.className = "add-task-form";
    form.innerHTML = `
      <input type="text" placeholder="New task title" required />
      <textarea placeholder="Description"></textarea>
      <input type="text" maxlength="64" placeholder="Sprint" />
      <input type="date" />
      <button type="submit">+ Task</button>
    `;
    form.addEventListener("submit", async (e) => {
      e.preventDefault();
      const title = form.querySelector("input").value.trim();
      const description = form.querySelector("textarea").value.trim();
      const sprint = form.querySelector('input[placeholder="Sprint"]').value.trim();
      const dueDate = form.querySelector('input[type="date"]').value;
      if (!title) return;
      await api(`/tasks/boards/${state.currentBoardId}/tasks`, {
        method: "POST",
        body: {
          title,
          description: description || null,
          sprint: sprint || null,
          due_date: dueDate || null,
          column_id: col.id,
          status: statusForColumnName(col.name),
        },
      });
      form.reset();
      await renderBoard();
    });
    colEl.appendChild(form);
    root.appendChild(colEl);
  });
}

function renderBoardFilters() {
  const assigneeSelect = $("#task-assignee-filter");
  const statusSelect = $("#task-status-filter");
  const sprintSelect = $("#task-sprint-filter");
  const selectedAssignee = assigneeSelect.value;
  const selectedStatus = statusSelect.value;
  const selectedSprint = sprintSelect.value;

  assigneeSelect.innerHTML = `<option value="">All assignees</option><option value="__unassigned">Unassigned</option>`;
  state.members.forEach((member) => {
    const opt = document.createElement("option");
    opt.value = member.user_id;
    opt.textContent = memberLabel(member.user_id);
    if (member.user_id === selectedAssignee) opt.selected = true;
    assigneeSelect.appendChild(opt);
  });
  if (selectedAssignee === "__unassigned") assigneeSelect.value = "__unassigned";

  statusSelect.innerHTML = `<option value="">All statuses</option>`;
  state.columns.forEach((column) => {
    const opt = document.createElement("option");
    opt.value = column.id;
    opt.textContent = column.name;
    if (column.id === selectedStatus) opt.selected = true;
    statusSelect.appendChild(opt);
  });

  const sprints = [...new Set(state.tasks.map((task) => task.sprint).filter(Boolean))].sort();
  sprintSelect.innerHTML = `<option value="">All sprints</option><option value="__none">No sprint</option>`;
  sprints.forEach((sprint) => {
    const opt = document.createElement("option");
    opt.value = sprint;
    opt.textContent = sprint;
    if (sprint === selectedSprint) opt.selected = true;
    sprintSelect.appendChild(opt);
  });
  if (selectedSprint === "__none") sprintSelect.value = "__none";
}

function renderTask(t) {
  const el = document.createElement("div");
  el.className = "task";
  const statusLabel = taskStatusLabel(t);
  const isOverdue = t.due_date && new Date(`${t.due_date}T23:59:59`) < new Date() && statusLabel.toLowerCase() !== "done";
  if (isOverdue) el.classList.add("overdue");
  el.innerHTML = `
    <div class="task-title">${escapeHtml(t.title)}</div>
    ${t.description ? `<p>${escapeHtml(t.description)}</p>` : ""}
    <div class="task-badges">
      <span class="badge">${escapeHtml(statusLabel)}</span>
      ${t.due_date ? `<span class="badge ${isOverdue ? "danger" : ""}">Due ${t.due_date}</span>` : ""}
      ${t.sprint ? `<span class="badge">${escapeHtml(t.sprint)}</span>` : ""}
      <span class="badge">${escapeHtml(memberLabel(t.assignee_id))}</span>
    </div>
  `;
  const select = document.createElement("select");
  state.columns.forEach((c) => {
    const opt = document.createElement("option");
    opt.value = c.id; opt.textContent = "→ " + c.name;
    if (c.id === t.column_id) opt.selected = true;
    select.appendChild(opt);
  });
  select.addEventListener("click", (e) => e.stopPropagation());
  select.addEventListener("change", async (e) => {
    const targetColumn = state.columns.find((c) => c.id === e.target.value);
    await api(`/tasks/tasks/${t.id}`, {
      method: "PATCH",
      body: { column_id: e.target.value, status: statusForColumnName(targetColumn?.name) },
    });
    await renderBoard();
  });
  el.appendChild(select);
  el.addEventListener("click", () => openTaskModal(t.id));
  return el;
}

// ---------- Task modal ----------
async function openTaskModal(taskId) {
  state.currentTaskId = taskId;
  const task = await api(`/tasks/tasks/${taskId}`);
  state.currentTask = task;
  $("#task-modal-title").textContent = task.title;
  $("#task-modal-meta").textContent =
    `Status: ${taskStatusLabel(task)}` +
    (task.due_date ? ` · Due ${task.due_date}` : "") +
    (task.sprint ? ` · ${task.sprint}` : "") +
    ` · Assignee: ${memberLabel(task.assignee_id)}`;
  const form = $("#task-edit-form");
  form.title.value = task.title;
  form.description.value = task.description || "";
  form.due_date.value = task.due_date || "";
  form.sprint.value = task.sprint || "";
  form.assignee_id.innerHTML = `<option value="">Unassigned</option>`;
  state.members.forEach((m) => {
    const opt = document.createElement("option");
    opt.value = m.user_id;
    opt.textContent = `${memberLabel(m.user_id)} (${m.role})`;
    if (m.user_id === task.assignee_id) opt.selected = true;
    form.assignee_id.appendChild(opt);
  });
  form.status_column_id.innerHTML = "";
  state.columns.forEach((c) => {
    const opt = document.createElement("option");
    opt.value = c.id;
    opt.textContent = c.name;
    if (c.id === task.column_id) opt.selected = true;
    form.status_column_id.appendChild(opt);
  });
  await loadComments(taskId);
  show($("#task-modal"));
}

async function loadComments(taskId) {
  const comments = await api(`/tasks/tasks/${taskId}/comments`);
  const ul = $("#comment-list");
  ul.innerHTML = "";
  comments.forEach((c) => {
    const li = document.createElement("li");
    li.innerHTML = `<strong>${new Date(c.created_at).toLocaleString()}</strong><p>${escapeHtml(c.body)}</p>`;
    ul.appendChild(li);
  });
}

function setupModal() {
  $("#task-modal").addEventListener("click", (e) => {
    if (e.target.matches("[data-close]") || e.target === $("#task-modal")) {
      hide($("#task-modal"));
    }
  });
  $("#task-edit-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.currentTaskId) return;
    const fd = new FormData(e.target);
    const columnId = fd.get("status_column_id");
    const column = state.columns.find((c) => c.id === columnId);
    await api(`/tasks/tasks/${state.currentTaskId}`, {
      method: "PATCH",
      body: {
        title: fd.get("title"),
        description: fd.get("description") || null,
        status: statusForColumnName(column?.name),
        due_date: fd.get("due_date") || null,
        sprint: fd.get("sprint") || null,
        assignee_id: fd.get("assignee_id") || null,
        column_id: columnId,
      },
    });
    hide($("#task-modal"));
    await renderBoard();
  });
  $("#delete-task-btn").addEventListener("click", async () => {
    if (!state.currentTaskId || !confirm("Delete this task?")) return;
    await api(`/tasks/tasks/${state.currentTaskId}`, { method: "DELETE" });
    hide($("#task-modal"));
    await renderBoard();
  });
  $("#comment-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const body = e.target.body.value.trim();
    if (!body || !state.currentTaskId) return;
    await api(`/tasks/tasks/${state.currentTaskId}/comments`, {
      method: "POST",
      body: { body },
    });
    e.target.reset();
    await loadComments(state.currentTaskId);
  });
}

// ---------- Documents ----------
async function loadDocuments(projectId) {
  state.documents = await api(`/documents/projects/${projectId}/documents`);
  const root = $("#document-list");
  root.innerHTML = "";
  if (!state.documents.length) {
    root.innerHTML = `<p class="empty-state">No documents uploaded yet.</p>`;
    return;
  }
  state.documents.forEach((d) => {
    const item = document.createElement("button");
    item.type = "button";
    item.className = "document-card";
    item.innerHTML = `
      <span class="document-icon">${documentIcon(d.content_type)}</span>
      <span>
        <strong>${escapeHtml(d.filename)}</strong>
        <small>${formatBytes(d.size_bytes)} · ${new Date(d.created_at).toLocaleDateString()}</small>
      </span>
    `;
    item.addEventListener("click", () => downloadDocument(d.id, d.filename));
    root.appendChild(item);
  });
}

async function loadMembers(projectId) {
  state.members = await api(`/projects/projects/${projectId}/members`);
  const root = $("#member-list");
  root.innerHTML = "";
  state.members.forEach((m) => {
    const item = document.createElement("div");
    item.className = "member-card";
    item.innerHTML = `
      <span class="avatar">${escapeHtml(m.role.slice(0, 1).toUpperCase())}</span>
      <span>
        <strong>${escapeHtml(memberLabel(m.user_id))}</strong>
        <small>${escapeHtml(m.role)} · ${escapeHtml(m.email || shortId(m.user_id))}</small>
      </span>
    `;
    root.appendChild(item);
  });
}

async function refreshProjectStats(updateBoardCards = true) {
  if (!state.currentProjectId) return;
  const boardTasks = await Promise.all(
    state.boards.map(async (board) => {
      if (board.id === state.currentBoardId) return [board.id, state.tasks];
      const tasks = await api(`/tasks/boards/${board.id}/tasks`);
      return [board.id, tasks];
    })
  );
  const allTasks = boardTasks.flatMap(([, tasks]) => tasks);
  state.boardTaskCounts = Object.fromEntries(boardTasks.map(([id, tasks]) => [id, tasks.length]));
  if (updateBoardCards) renderBoardList();
  renderActivity(allTasks);
  renderProjectStats({
    boards: state.boards.length,
    tasks: allTasks.length,
    done: allTasks.filter((t) => taskStatusLabel(t).toLowerCase() === "done").length,
    documents: state.documents.length,
    members: state.members.length,
  });
}

function renderProjectStats(stats = {}) {
  const root = $("#project-stats");
  if (stats.loading) {
    root.innerHTML = `<div class="stat-card"><strong>Loading</strong><span>project overview</span></div>`;
    return;
  }
  const done = stats.done || 0;
  const tasks = stats.tasks || 0;
  const progress = tasks ? Math.round((done / tasks) * 100) : 0;
  root.innerHTML = `
    <div class="stat-card"><strong>${stats.boards || 0}</strong><span>Boards</span></div>
    <div class="stat-card"><strong>${tasks}</strong><span>Tasks</span></div>
    <div class="stat-card"><strong>${progress}%</strong><span>Done</span></div>
    <div class="stat-card"><strong>${stats.documents || 0}</strong><span>Docs</span></div>
    <div class="stat-card"><strong>${stats.members || 0}</strong><span>Members</span></div>
  `;
}

function renderActivity(tasks = state.tasks) {
  const root = $("#activity-list");
  if (!root) return;
  const events = [
    ...tasks.map((task) => ({
      type: "Task",
      title: task.title,
      detail: `${taskStatusLabel(task)} · ${memberLabel(task.assignee_id)}${task.sprint ? ` · ${task.sprint}` : ""}${task.due_date ? ` · due ${task.due_date}` : ""}`,
      at: task.updated_at || task.created_at,
    })),
    ...state.documents.map((doc) => ({
      type: "Document",
      title: doc.filename,
      detail: formatBytes(doc.size_bytes),
      at: doc.created_at,
    })),
  ]
    .sort((a, b) => new Date(b.at) - new Date(a.at))
    .slice(0, 8);
  if (!events.length) {
    root.innerHTML = `<p class="empty-state">No activity yet. Create tasks or upload documents to start collaborating.</p>`;
    return;
  }
  root.innerHTML = events.map((event) => `
    <div class="activity-item">
      <span>${escapeHtml(event.type)}</span>
      <strong>${escapeHtml(event.title)}</strong>
      <small>${escapeHtml(event.detail)} · ${new Date(event.at).toLocaleString()}</small>
    </div>
  `).join("");
}

function formatBytes(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

function shortId(id) {
  return `${id.slice(0, 8)}…${id.slice(-4)}`;
}

function documentIcon(contentType) {
  if (contentType?.includes("image")) return "IMG";
  if (contentType?.includes("pdf")) return "PDF";
  if (contentType?.includes("text")) return "TXT";
  return "DOC";
}

async function downloadDocument(id, filename) {
  const token = localStorage.getItem(TOKEN_KEY);
  const res = await fetch(`${API}/documents/documents/${id}/content`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok) { alert("Download failed"); return; }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url; a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

// ---------- AI assistant ----------
async function generateAiSummary() {
  if (!state.currentProjectId) return;
  const resultEl = $("#ai-summary-result");
  const button = $("#generate-summary-btn");
  button.disabled = true;
  button.textContent = "Generating...";
  resultEl.classList.add("muted");
  resultEl.textContent = "Collecting project context and calling Bedrock...";
  try {
    const focus = $("#ai-focus").value.trim();
    const result = await api(`/ai/projects/${state.currentProjectId}/summary`, {
      method: "POST",
      body: {
        include_comments: true,
        include_documents: true,
        focus: focus || null,
      },
    });
    resultEl.classList.remove("muted");
    resultEl.textContent = [
      result.summary,
      "",
      "Risks:",
      ...(result.risks.length ? result.risks.map((r) => `- ${r}`) : ["- None reported"]),
      "",
      "Next actions:",
      ...(result.next_actions.length ? result.next_actions.map((a) => `- ${a}`) : ["- None reported"]),
      "",
      `Provider: ${result.provider} · Model: ${result.model_id}`,
    ].join("\n");
  } catch (err) {
    resultEl.textContent = `AI summary failed: ${err.message}`;
  } finally {
    button.disabled = false;
    button.textContent = "Generate summary";
  }
}

// ---------- Forms (project / board / upload) ----------
function setupForms() {
  $("#new-project-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    const fd = new FormData(e.target);
    await api("/projects/projects", {
      method: "POST",
      body: { name: fd.get("name"), description: fd.get("description") || null },
    });
    e.target.reset();
    await loadProjects();
  });

  $("#new-board-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.currentProjectId) return;
    const fd = new FormData(e.target);
    await api(`/projects/projects/${state.currentProjectId}/boards`, {
      method: "POST",
      body: { name: fd.get("name"), type: "kanban" },
    });
    e.target.reset();
    await loadBoards(state.currentProjectId);
  });

  $("#upload-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.currentProjectId) return;
    const fd = new FormData(e.target);
    await api(`/documents/projects/${state.currentProjectId}/documents`, {
      method: "POST",
      body: fd,
      isForm: true,
    });
    e.target.reset();
    await loadDocuments(state.currentProjectId);
    await refreshProjectStats(false);
  });

  $("#member-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    if (!state.currentProjectId) return;
    const fd = new FormData(e.target);
    await api(`/projects/projects/${state.currentProjectId}/members`, {
      method: "POST",
      body: { user_id: fd.get("user_id"), role: fd.get("role") },
    });
    e.target.reset();
    await loadMembers(state.currentProjectId);
    if (state.currentBoardId) renderBoardFilters();
    await refreshProjectStats(false);
  });

  $("#generate-summary-btn").addEventListener("click", generateAiSummary);
  $("#task-search").addEventListener("input", renderBoard);
  $("#task-assignee-filter").addEventListener("change", renderBoard);
  $("#task-status-filter").addEventListener("change", renderBoard);
  $("#task-sprint-filter").addEventListener("change", renderBoard);
}

// ---------- Boot ----------
async function boot() {
  setupAuth();
  setupForms();
  setupModal();
  await loadServiceHealth();
  setInterval(loadServiceHealth, 30000);

  if (localStorage.getItem(TOKEN_KEY)) {
    try { await afterLogin(); }
    catch { localStorage.removeItem(TOKEN_KEY); }
  }
}

boot();
