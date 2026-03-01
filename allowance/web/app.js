let agents = [];
let transactions = [];
let logsData = [];
let activeLogFilter = "all";
let currentReview = null;

async function api(path, opts = {}) {
  const res = await fetch(`/api/v1${path}`, {
    headers: { "Content-Type": "application/json" },
    ...opts,
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(body?.error?.message || `Request failed: ${res.status}`);
  }
  return body;
}

function showView(viewId) {
  document.querySelectorAll(".view").forEach(v => v.classList.add("hidden"));
  const selected = document.getElementById(`view-${viewId}`);
  if (selected) selected.classList.remove("hidden");

  document.getElementById("breadcrumb-view").innerText = viewId.charAt(0).toUpperCase() + viewId.slice(1);

  document.querySelectorAll(".nav-item").forEach(item => {
    item.classList.remove("active", "bg-zinc-100", "text-zinc-950");
    item.classList.add("text-zinc-500");
  });
  const navItem = document.getElementById(`nav-${viewId}`);
  if (navItem) {
    navItem.classList.add("active", "bg-zinc-100", "text-zinc-950");
    navItem.classList.remove("text-zinc-500");
  }
}

function closeAllDropdowns() {
  document.querySelectorAll(".dropdown-content").forEach(d => d.classList.remove("show"));
}

async function loadAgents() {
  const data = await api("/agents");
  agents = data.agents || [];
  populatePolicyFilters();
  renderAgents();
}

async function loadTransactions() {
  const status = document.getElementById("filter-type")?.value || "all";
  const agentName = document.getElementById("filter-policy")?.value || "all";
  const query = new URLSearchParams();
  if (status !== "all") query.set("status", status);
  if (agentName !== "all") {
    const agent = agents.find(a => a.name === agentName);
    if (agent) query.set("agent_id", agent.id);
  }
  const suffix = query.toString() ? `?${query}` : "";
  const data = await api(`/transactions${suffix}`);
  transactions = data.transactions || [];
  renderTransactions();
}

async function loadSettings() {
  const data = await api("/settings");
  document.getElementById("privacy-key-display").value = data.privacy_api_key_masked || "(not set)";
  document.getElementById("db-path-display").value = data.db_path || "";
  document.getElementById("notification-webhook-url").value = data.notification_webhook_url || "";
  document.getElementById("approval-timeout").value = data.approval_timeout_minutes || 30;
  document.getElementById("privacy-status").innerText = data.privacy_connected
    ? "Privacy.com API Connected"
    : "Privacy.com API Missing (degraded mode)";
}

async function saveSettings() {
  const payload = {
    notification_webhook_url: document.getElementById("notification-webhook-url").value.trim(),
    approval_timeout_minutes: parseInt(document.getElementById("approval-timeout").value || "30", 10),
  };
  try {
    await api("/settings", { method: "PUT", body: JSON.stringify(payload) });
    document.getElementById("settings-message").innerText = "Settings saved.";
  } catch (e) {
    document.getElementById("settings-message").innerText = e.message;
  }
}

function renderTransactions() {
  const container = document.getElementById("transactionList");
  const countLabel = document.getElementById("transaction-count");
  if (!container) return;

  countLabel.innerText = `Showing ${transactions.length} transaction${transactions.length === 1 ? "" : "s"}`;
  if (transactions.length === 0) {
    container.innerHTML = `<div class="p-12 text-center"><p class="text-sm text-zinc-400">No transactions found.</p></div>`;
    return;
  }

  container.innerHTML = transactions.map(t => {
    const agent = agents.find(a => a.id === t.agent_id);
    const icon = agent?.icon || "💸";
    const agentName = agent?.name || t.agent_id;
    const status = (t.status || "").toLowerCase();
    let action = `<span class="px-2 py-0.5 rounded-full text-[10px] font-bold uppercase tracking-tight bg-zinc-100 text-zinc-600">${status}</span>`;
    if (status === "review") {
      action = `<button onclick="openReview('${t.id}','approve')" class="btn-primary h-8 px-3 rounded-md text-xs font-semibold">Review</button>`;
    } else if (status === "declined") {
      action = `<button onclick="openReview('${t.id}','decline')" class="bg-red-50 text-red-600 border border-red-100 h-8 px-3 rounded-md text-[10px] font-bold uppercase tracking-tight">Declined</button>`;
    }

    return `
      <div class="px-6 py-4 flex items-center justify-between hover:bg-[#fafafa] transition-colors group">
        <div class="flex items-center gap-4">
          <div class="w-8 h-8 rounded border border-zinc-200 flex items-center justify-center text-sm bg-white">${icon}</div>
          <div>
            <div class="text-sm font-semibold text-zinc-900">${escapeHtml(t.merchant)}</div>
            <div class="text-[11px] text-zinc-500 flex items-center gap-1.5">${escapeHtml(agentName)}</div>
          </div>
        </div>
        <div class="flex items-center gap-6">
          <span class="text-sm font-mono font-medium">$${Number(t.amount).toFixed(2)}</span>
          ${action}
        </div>
      </div>`;
  }).join("");
}

function populatePolicyFilters() {
  const select = document.getElementById("filter-policy");
  if (!select) return;
  const current = select.value;
  select.innerHTML = '<option value="all">All Policies</option>' +
    agents.map(a => `<option value="${escapeHtml(a.name)}">${escapeHtml(a.name)}</option>`).join("");
  if ([...select.options].some(o => o.value === current)) select.value = current;
}

async function renderAgents() {
  const container = document.getElementById("agentGrid");
  if (!container) return;

  const cards = await Promise.all(agents.map(async a => {
    let policy = null;
    let allowance = { remaining: 0, limit: 0, spent: 0 };
    try {
      const p = await api(`/agents/${a.id}/policy`);
      policy = p.policy;
      const al = await api(`/allowance/${a.id}`);
      allowance = al;
    } catch (_e) {}

    const usedPct = policy?.spend_limit ? Math.min(100, (allowance.spent / policy.spend_limit) * 100) : 0;
    return `
      <div class="bg-white border border-zinc-200 p-6 rounded-lg hover:ring-1 hover:ring-zinc-400 transition-all flex flex-col justify-between h-56 relative group">
        <div>
          <div class="flex justify-between items-start mb-4">
            <span class="text-xl">${escapeHtml(a.icon)}</span>
            <span class="px-2 py-0.5 rounded-md border border-zinc-100 text-[10px] font-medium ${a.status === 'active' ? 'text-zinc-950' : 'text-zinc-400'}">${a.status}</span>
          </div>
          <h3 class="text-sm font-bold tracking-tight mb-0.5">${escapeHtml(a.name)}</h3>
          <p class="text-[11px] font-mono text-zinc-400 uppercase tracking-widest mb-6">${policy?.privacy_card_token ? 'card linked' : 'no card'}</p>
        </div>
        <div class="space-y-2">
          <div class="flex justify-between text-[10px] font-semibold text-zinc-400 uppercase tracking-wider">
            <span>Used</span>
            <span>$${Number(policy?.spend_limit || 0).toFixed(2)} Limit</span>
          </div>
          <div class="w-full bg-zinc-100 h-1 rounded-full overflow-hidden">
            <div class="bg-zinc-950 h-full transition-all duration-700" style="width:${usedPct}%"></div>
          </div>
          <div class="flex justify-between items-baseline pt-1">
            <span class="text-sm font-bold">$${Number(allowance.spent || 0).toFixed(2)}</span>
            <span class="text-[10px] text-zinc-400">$${Number(allowance.remaining || 0).toFixed(2)} remaining</span>
          </div>
          <div class="flex gap-2 pt-2">
            <button onclick="toggleAgentStatus('${a.id}','${a.status}')" class="btn-outline h-8 px-2 rounded-md text-[11px]">${a.status === 'active' ? 'Pause' : 'Activate'}</button>
            <button onclick="provisionCard('${a.id}')" class="btn-primary h-8 px-2 rounded-md text-[11px]">Provision Card</button>
          </div>
        </div>
      </div>`;
  }));

  container.innerHTML = cards.join("");
}

async function toggleAgentStatus(agentID, current) {
  const next = current === "active" ? "paused" : "active";
  await api(`/agents/${agentID}`, { method: "PUT", body: JSON.stringify({ status: next }) });
  await refreshAll();
}

async function provisionCard(agentID) {
  try {
    await api(`/agents/${agentID}/card`, { method: "POST" });
    alert("Card provisioned.");
  } catch (e) {
    alert(e.message);
  }
  await refreshAll();
}

function openPolicyModal() {
  document.getElementById("policy-modal-overlay").classList.remove("hidden");
}

function closePolicyModal() {
  document.getElementById("policy-modal-overlay").classList.add("hidden");
}

document.getElementById("save-policy-btn")?.addEventListener("click", async () => {
  const name = document.getElementById("new-policy-name").value.trim();
  const limit = parseFloat(document.getElementById("new-policy-limit").value || "50");
  const category = document.getElementById("new-policy-category").value;
  if (!name) return;

  try {
    const created = await api("/agents", { method: "POST", body: JSON.stringify({ name }) });
    await api(`/agents/${created.agent.id}/policy`, {
      method: "PUT",
      body: JSON.stringify({ spend_limit: limit, limit_period: "monthly", category_lock: category === "all" ? [] : [category] }),
    });
    closePolicyModal();
    await refreshAll();
  } catch (e) {
    alert(e.message);
  }
});

function openReview(txID, action) {
  currentReview = { txID, action };
  const tx = transactions.find(t => t.id === txID);
  if (!tx) return;
  const modal = document.getElementById("modal-overlay");
  document.getElementById("modal-title").innerText = action === "decline" ? "Override Decline" : "Review Transaction";
  document.getElementById("modal-desc").innerText = `${tx.merchant} requested $${Number(tx.amount).toFixed(2)}.`;
  const btn = document.getElementById("modal-action-btn");
  btn.innerText = action === "decline" ? "Approve Anyway" : "Approve";
  btn.onclick = submitReview;
  modal.classList.remove("hidden");
}

async function submitReview() {
  if (!currentReview) return;
  await api(`/transactions/${currentReview.txID}/approve`, { method: "POST" });
  closeModal();
  await refreshAll();
}

function closeModal() {
  document.getElementById("modal-overlay").classList.add("hidden");
  currentReview = null;
}

function setLogFilter(filter) {
  activeLogFilter = filter;
  document.querySelectorAll(".log-filter-btn").forEach(btn => btn.classList.remove("active"));
  document.getElementById(`filter-${filter}`)?.classList.add("active");
  renderLogs();
}

function clearLogs() {
  logsData = [];
  renderLogs();
}

function renderLogs() {
  const terminal = document.getElementById("logTerminal");
  if (!terminal) return;
  const filtered = logsData.filter(l => activeLogFilter === "all" || l.type === activeLogFilter);
  terminal.innerHTML = filtered.map(log => {
    const t = new Date(log.created_at).toTimeString().slice(0, 8);
    return `<div class="mb-1"><span class="text-zinc-600">[${t}]</span> <span class="text-zinc-500">${log.type}:</span> <span>${escapeHtml(log.message)}</span></div>`;
  }).join("");
  terminal.scrollTop = terminal.scrollHeight;
}

async function loadLogs() {
  const data = await api("/logs?limit=200");
  logsData = data.logs || [];
  renderLogs();
}

function escapeHtml(s) {
  return String(s || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

async function refreshAll() {
  await loadAgents();
  await loadTransactions();
  await loadSettings();
  await loadLogs();
}

window.showView = showView;
window.openPolicyModal = openPolicyModal;
window.closePolicyModal = closePolicyModal;
window.openReview = openReview;
window.closeModal = closeModal;
window.setLogFilter = setLogFilter;
window.clearLogs = clearLogs;
window.closeAllDropdowns = closeAllDropdowns;
window.saveSettings = saveSettings;
window.renderTransactions = loadTransactions;
window.toggleAgentStatus = toggleAgentStatus;
window.provisionCard = provisionCard;

window.onload = async () => {
  await refreshAll();
  setInterval(loadLogs, 5000);
};
