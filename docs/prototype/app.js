const routeMeta = {
  channel: ["产品研发", "18 位成员 · 2 个 Agent · 1 个工作区", "#"],
  tasks: ["任务执行", "共享运行现场 · 3 个进行中", ""],
  approvals: ["审批中心", "1 个动作等待你的决定", ""],
  agents: ["Agent 目录", "组织成员、职责与运行时", ""],
  inbox: ["动态", "提及、回复与需要确认的 Agent 结果", ""],
  search: ["全局搜索", "消息、文件、任务与成员", ""],
  files: ["文件与产物", "1,284 个可访问文件", ""],
  "task-result": ["任务交付", "DES-088 · 等待人工确认", ""],
  runtime: ["Runtime 设备", "2 台获权 Desktop Runtime", ""],
  sync: ["同步与恢复", "本地数据库和设备游标", ""],
  organization: ["工作空间", "北辰科技 · 产品与工程", ""],
  admin: ["管理后台", "北辰科技 · 组织治理", ""],
};

function switchRoute(route, { moveFocus = false } = {}) {
  if (!routeMeta[route]) return;
  document.querySelectorAll("[data-screen]").forEach((screen) => {
    screen.classList.toggle("is-visible", screen.dataset.screen === route);
  });
  document.querySelectorAll(".rail-button[data-route]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.route === route);
  });
  const [title, subtitle, hash] = routeMeta[route];
  const viewTitle = document.querySelector("#view-title");
  viewTitle.textContent = title;
  document.querySelector("#view-subtitle").textContent = subtitle;
  document.querySelector("#route-announcement").textContent = `${title}。${subtitle}`;
  document.querySelector(".channel-hash").textContent = hash;
  document.querySelectorAll(".sidebar-item").forEach((item) => {
    item.classList.toggle("is-selected", route === "channel" && item.dataset.channel === "产品研发");
  });
  const nextUrl = new URL(window.location.href);
  nextUrl.searchParams.set("screen", route);
  window.history.replaceState({}, "", nextUrl);
  if (moveFocus) window.requestAnimationFrame(() => viewTitle.focus());
}

document.querySelectorAll("[data-route]").forEach((button) => {
  button.addEventListener("click", () => switchRoute(button.dataset.route, { moveFocus: true }));
});

const modal = document.querySelector("#task-modal");
const appShell = document.querySelector("#app-shell");
const createTaskButton = document.querySelector("#create-task");
let modalOpener = null;

const modalFocusableSelector = [
  "button:not([disabled])",
  "input:not([disabled])",
  "textarea:not([disabled])",
  "select:not([disabled])",
  "a[href]",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

function modalFocusableElements() {
  return [...modal.querySelectorAll(modalFocusableSelector)].filter((element) => !element.hidden);
}

const setModal = (open, { restoreFocus = true } = {}) => {
  if (open) {
    modalOpener = document.activeElement instanceof HTMLElement
      && document.activeElement !== document.body
      && appShell.contains(document.activeElement)
      ? document.activeElement
      : createTaskButton;
  }
  modal.classList.toggle("is-open", open);
  modal.setAttribute("aria-hidden", String(!open));
  modal.inert = !open;
  appShell.inert = open;
  appShell.setAttribute("aria-hidden", String(open));

  if (open) {
    window.requestAnimationFrame(() => modalFocusableElements()[0]?.focus());
  } else if (restoreFocus && modalOpener?.isConnected && !modalOpener.disabled) {
    window.requestAnimationFrame(() => modalOpener.focus());
  }
};

createTaskButton.addEventListener("click", () => setModal(true));
document.querySelectorAll(".modal-close").forEach((button) => button.addEventListener("click", () => setModal(false)));
modal.addEventListener("click", (event) => {
  if (event.target === modal) setModal(false);
});
document.addEventListener("keydown", (event) => {
  if (!modal.classList.contains("is-open")) return;
  if (event.key === "Escape") {
    event.preventDefault();
    setModal(false);
    return;
  }
  if (event.key !== "Tab") return;

  const focusable = modalFocusableElements();
  if (focusable.length === 0) {
    event.preventDefault();
    return;
  }
  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (!modal.contains(document.activeElement)) {
    event.preventDefault();
    (event.shiftKey ? last : first).focus();
  } else if (event.shiftKey && document.activeElement === first) {
    event.preventDefault();
    last.focus();
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault();
    first.focus();
  }
});
document.querySelector("#confirm-task").addEventListener("click", () => {
  setModal(false, { restoreFocus: false });
  switchRoute("tasks", { moveFocus: true });
});

document.querySelector("#approve-action").addEventListener("click", (event) => {
  event.currentTarget.innerHTML = "<span>✓</span> 已批准";
  event.currentTarget.disabled = true;
  event.currentTarget.style.background = "#74817b";
  document.querySelector(".approval-detail-head .eyebrow").textContent = "已批准 · 权限 10 分钟后过期";
});

document.querySelector("#send-message").addEventListener("click", () => {
  const input = document.querySelector("#composer-input");
  const value = input.value.trim();
  if (!value) return;
  const article = document.createElement("article");
  article.className = "message-row";
  article.innerHTML = `
    <span class="avatar avatar-user">林</span>
    <div class="message-body">
      <div class="message-meta"><strong>林嘉</strong><time>刚刚</time></div>
      <p></p>
    </div>`;
  article.querySelector("p").textContent = value;
  document.querySelector("#timeline").appendChild(article);
  input.value = "";
  article.scrollIntoView({ behavior: "smooth", block: "end" });
});

document.querySelector("#composer-input").addEventListener("keydown", (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
    document.querySelector("#send-message").click();
  }
});

document.addEventListener("keydown", (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
    event.preventDefault();
    switchRoute("search");
  }
});

document.querySelectorAll(".inbox-tabs, .search-chips, .result-tabs, .admin-nav, .mode-control").forEach((group) => {
  group.querySelectorAll("button").forEach((button) => {
    button.addEventListener("click", () => {
      group.querySelectorAll("button").forEach((item) => item.classList.remove("is-active"));
      button.classList.add("is-active");
    });
  });
});

document.querySelectorAll(".file-row").forEach((row) => {
  row.addEventListener("click", () => {
    if (row.dataset.route) return;
    document.querySelectorAll(".file-row").forEach((item) => item.classList.remove("is-selected"));
    row.classList.add("is-selected");
  });
});

document.querySelectorAll(".agent-card").forEach((card) => {
  card.addEventListener("click", () => {
    document.querySelectorAll(".agent-card").forEach((item) => item.classList.remove("is-selected"));
    card.classList.add("is-selected");
  });
});

document.querySelector("#accept-result").addEventListener("click", (event) => {
  event.currentTarget.innerHTML = "<span>✓</span> 已接受 · PR #418";
  event.currentTarget.disabled = true;
  event.currentTarget.style.background = "#74817b";
  document.querySelector(".screen-task-result .eyebrow").textContent = "DES-088 · 已交付";
});

document.querySelector("#simulate-offline").addEventListener("click", (event) => {
  event.currentTarget.textContent = "重新连接";
  document.querySelector(".runtime-detail-head .eyebrow").textContent = "本地 Runtime · 连接中断";
  document.querySelector(".runtime-detail-head p").textContent = "最后心跳：刚刚 · 当前 Run 等待本设备恢复，不会自动转移";
  document.querySelector(".runtime-detail-head").classList.add("is-offline");
});

document.querySelector("#simulate-gap").addEventListener("click", (event) => {
  event.currentTarget.textContent = "缺口已修复";
  event.currentTarget.disabled = true;
  document.querySelector(".sync-overview > div:nth-child(3) strong").textContent = "7 → 0";
  document.querySelector(".sync-status strong").textContent = "同步缺口已自动修复";
});

const initialParams = new URLSearchParams(window.location.search);
switchRoute(initialParams.get("screen") || "channel");
if (initialParams.get("modal") === "task") setModal(true);
