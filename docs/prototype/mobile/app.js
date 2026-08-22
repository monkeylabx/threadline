const root = document.querySelector(".desktop-window");
const rootViews = new Set(["messages", "search", "activity", "profile"]);
const validViews = new Set([...rootViews, "channel", "task", "approval"]);
const isMobile = () => window.matchMedia("(max-width: 820px)").matches;
const requestedView = new URLSearchParams(window.location.search).get("view");
const initialView = requestedView === "home" ? "messages" : requestedView || (isMobile() ? "messages" : "channel");
const viewLabels = {
  messages: "消息",
  search: "搜索",
  activity: "活动",
  profile: "我的",
  channel: "产品研发频道",
  task: "任务执行现场：Runtime Lease 与 Fencing Token",
  approval: "权限审批：删除过期状态文件",
};
const viewFocusOrigins = new Map();
let pendingFocusRestore = null;

const sheetFocusableSelector = [
  "button:not([disabled])",
  "input:not([disabled])",
  "textarea:not([disabled])",
  "select:not([disabled])",
  "a[href]",
  "[tabindex]:not([tabindex='-1'])",
].join(",");

function normalizeView(view) {
  if (view === "home") return "messages";
  return validViews.has(view) ? view : isMobile() ? "messages" : "channel";
}

function focusTargetForView(view) {
  if (view === "task") return document.querySelector("#mobile-task-title");
  if (view === "approval") return document.querySelector("#mobile-approval-title");
  if (view === "channel") return document.querySelector("#mobile-channel-heading");
  return document.querySelector(`[data-home-panel="${view === "search" ? "messages" : view}"] h1`);
}

function renderView(view, { moveFocus = false, restoreFrom = null } = {}) {
  const nextView = normalizeView(view);
  const rootViewActive = rootViews.has(nextView);
  const workbenchView = rootViewActive ? "channel" : nextView;
  const mobileHomeActive = rootViewActive && isMobile();
  root.dataset.mobileView = nextView;
  root.dataset.view = workbenchView;

  const workbench = document.querySelector(".workbench");
  const mobileHome = document.querySelector(".mobile-home");
  const conversation = document.querySelector(".conversation");
  const channelDock = document.querySelector(".channel-dock");
  const taskSheet = document.querySelector(".task-sheet");
  const approvalSheet = document.querySelector(".approval-sheet");
  const modalSheetActive = isMobile() && (nextView === "task" || nextView === "approval");

  workbench.inert = mobileHomeActive;
  workbench.setAttribute("aria-hidden", String(mobileHomeActive));
  mobileHome.inert = !mobileHomeActive;
  mobileHome.setAttribute("aria-hidden", String(!mobileHomeActive));
  conversation.inert = nextView !== "channel";
  channelDock.inert = nextView !== "channel";
  taskSheet.inert = nextView !== "task";
  taskSheet.setAttribute("aria-hidden", String(nextView !== "task"));
  approvalSheet.inert = nextView !== "approval";
  approvalSheet.setAttribute("aria-hidden", String(nextView !== "approval"));
  document.querySelector(".sidebar").inert = modalSheetActive;
  document.querySelector(".topbar").inert = modalSheetActive;
  [taskSheet, approvalSheet].forEach((sheet) => {
    if (isMobile()) sheet.setAttribute("role", "dialog");
    else sheet.setAttribute("role", "region");
    if (modalSheetActive && !sheet.inert) sheet.setAttribute("aria-modal", "true");
    else sheet.removeAttribute("aria-modal");
  });

  if (rootViewActive) renderHomePanel(nextView === "search" ? "messages" : nextView);
  setMobileSearch(nextView === "search" && isMobile());

  document.querySelectorAll("[data-view-target]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.viewTarget === workbenchView);
  });

  document.querySelector("#mobile-route-announcement").textContent = viewLabels[nextView];
  if (restoreFrom) {
    const origin = viewFocusOrigins.get(restoreFrom);
    viewFocusOrigins.delete(restoreFrom);
    if (origin?.isConnected && !origin.disabled) window.requestAnimationFrame(() => origin.focus());
    else if (moveFocus) window.requestAnimationFrame(() => focusTargetForView(nextView)?.focus());
  } else if (moveFocus) {
    window.requestAnimationFrame(() => focusTargetForView(nextView)?.focus());
  }
}

function navigateView(view, { replace = false, restoreFrom = null } = {}) {
  const nextView = normalizeView(view);
  const currentView = root.dataset.mobileView || "channel";
  if (!replace && currentView === nextView) return;

  if ((nextView === "task" || nextView === "approval") && document.activeElement instanceof HTMLElement) {
    viewFocusOrigins.set(nextView, document.activeElement);
  }

  const url = new URL(window.location.href);
  url.searchParams.set("view", nextView);
  const state = { view: nextView, previous: replace ? null : currentView };
  window.history[replace ? "replaceState" : "pushState"](state, "", url);
  renderView(nextView, { moveFocus: true, restoreFrom });
}

function returnFromSheet() {
  const currentView = root.dataset.mobileView;
  if (window.history.state?.previous) {
    pendingFocusRestore = currentView;
    window.history.back();
    return;
  }
  const fallback = currentView === "approval" ? "task" : isMobile() ? "messages" : "channel";
  navigateView(fallback, { replace: true, restoreFrom: currentView });
}

document.querySelectorAll("[data-view-target]").forEach((button) => {
  button.addEventListener("click", () => navigateView(button.dataset.viewTarget));
});

document.querySelectorAll(".close-sheet").forEach((button) => {
  button.addEventListener("click", returnFromSheet);
});

document.addEventListener("keydown", (event) => {
  const currentView = root.dataset.mobileView;
  const activeSheet = currentView === "task"
    ? document.querySelector(".task-sheet")
    : currentView === "approval"
      ? document.querySelector(".approval-sheet")
      : null;
  if (!activeSheet || !isMobile()) return;

  if (event.key === "Escape") {
    event.preventDefault();
    returnFromSheet();
    return;
  }
  if (event.key !== "Tab") return;

  const focusable = [...activeSheet.querySelectorAll(sheetFocusableSelector)]
    .filter((element) => !element.hidden && !element.closest("[hidden]"));
  if (focusable.length === 0) {
    event.preventDefault();
    activeSheet.focus();
    return;
  }

  const first = focusable[0];
  const last = focusable[focusable.length - 1];
  if (!activeSheet.contains(document.activeElement) || !focusable.includes(document.activeElement)) {
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

window.addEventListener("popstate", (event) => {
  const queryView = new URLSearchParams(window.location.search).get("view");
  const restoreFrom = pendingFocusRestore;
  pendingFocusRestore = null;
  renderView(event.state?.view || queryView || (isMobile() ? "messages" : "channel"), {
    moveFocus: true,
    restoreFrom,
  });
});

window.addEventListener("resize", () => renderView(root.dataset.mobileView || "channel"));

document.querySelectorAll(".sheet-tabs").forEach((tabs) => {
  const tabButtons = [...tabs.querySelectorAll("[role='tab']")];
  const selectTab = (selected, { focus = false } = {}) => {
    tabButtons.forEach((tab) => {
      const active = tab === selected;
      tab.classList.toggle("is-active", active);
      tab.setAttribute("aria-selected", String(active));
      tab.tabIndex = active ? 0 : -1;
      document.querySelector(`#${tab.getAttribute("aria-controls")}`).hidden = !active;
    });
    if (focus) selected.focus();
  };

  tabButtons.forEach((button, index) => {
    button.addEventListener("click", () => selectTab(button));
    button.addEventListener("keydown", (event) => {
      let targetIndex = null;
      if (event.key === "ArrowRight") targetIndex = (index + 1) % tabButtons.length;
      if (event.key === "ArrowLeft") targetIndex = (index - 1 + tabButtons.length) % tabButtons.length;
      if (event.key === "Home") targetIndex = 0;
      if (event.key === "End") targetIndex = tabButtons.length - 1;
      if (targetIndex === null) return;
      event.preventDefault();
      selectTab(tabButtons[targetIndex], { focus: true });
    });
  });
});

document.querySelector(".mobile-back-to-home").addEventListener("click", () => {
  if (rootViews.has(window.history.state?.previous)) {
    window.history.back();
    return;
  }
  navigateView("messages", { replace: true });
});

document.querySelectorAll("[data-mobile-route-target]").forEach((button) => {
  button.addEventListener("click", () => navigateView(button.dataset.mobileRouteTarget));
});

function renderHomePanel(panelName) {
  document.querySelectorAll("[data-home-panel]").forEach((panel) => {
    panel.classList.toggle("is-active", panel.dataset.homePanel === panelName);
  });
  document.querySelectorAll("[data-home-panel-target]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.homePanelTarget === panelName);
  });
  document.querySelector(".mobile-home-content").scrollTop = 0;
}

const mobileHome = document.querySelector(".mobile-home");
const mobileSearchScreen = document.querySelector(".mobile-search-screen");
const mobileSearchInput = document.querySelector(".mobile-search-input");

function setMobileSearch(open) {
  const wasOpen = mobileHome.classList.contains("is-searching");
  mobileHome.classList.toggle("is-searching", open);
  mobileSearchScreen.inert = !open;
  mobileSearchScreen.setAttribute("aria-hidden", String(!open));
  document.querySelector(".mobile-home-header").inert = open;
  document.querySelector(".mobile-home-content").inert = open;
  document.querySelector(".mobile-home-nav").inert = open;
  if (open && !wasOpen) window.setTimeout(() => mobileSearchInput.focus(), 190);
}

document.querySelector(".mobile-search-trigger").addEventListener("click", () => navigateView("search"));
document.querySelector(".mobile-search-back").addEventListener("click", () => {
  if (rootViews.has(window.history.state?.previous)) window.history.back();
  else navigateView("messages", { replace: true });
});

mobileSearchInput.addEventListener("input", () => {
  const query = mobileSearchInput.value.trim().toLocaleLowerCase();
  let visibleResults = 0;
  document.querySelectorAll(".mobile-search-result").forEach((result) => {
    const visible = !query || result.textContent.toLocaleLowerCase().includes(query);
    result.hidden = !visible;
    if (visible) visibleResults += 1;
  });
  document.querySelector(".mobile-search-empty").hidden = visibleResults > 0;
});

document.querySelector(".mobile-search-clear").addEventListener("click", () => {
  document.querySelectorAll(".mobile-search-result").forEach((result) => { result.hidden = true; });
  document.querySelector(".mobile-search-empty").hidden = false;
});

document.querySelectorAll(".mobile-search-tabs button").forEach((button) => {
  button.addEventListener("click", () => {
    document.querySelectorAll(".mobile-search-tabs button").forEach((item) => item.classList.remove("is-active"));
    button.classList.add("is-active");
  });
});

document.querySelectorAll("[data-home-panel-target]").forEach((button) => {
  button.addEventListener("click", () => navigateView(button.dataset.homePanelTarget));
});

document.querySelectorAll(".mobile-filter-tabs button").forEach((button) => {
  button.addEventListener("click", () => {
    document.querySelectorAll(".mobile-filter-tabs button").forEach((item) => item.classList.remove("is-active"));
    button.classList.add("is-active");
  });
});

const messageFilterButtons = document.querySelectorAll("[data-message-filter]");
messageFilterButtons.forEach((button) => {
  button.addEventListener("click", () => {
    const filter = button.dataset.messageFilter;
    messageFilterButtons.forEach((item) => {
      const active = item === button;
      item.classList.toggle("is-active", active);
      item.setAttribute("aria-pressed", String(active));
    });

    document.querySelectorAll(".mobile-conversation-row[data-message-filters]").forEach((row) => {
      const filters = row.dataset.messageFilters.split(" ").filter(Boolean);
      row.hidden = filter !== "all" && !filters.includes(filter);
    });

    document.querySelectorAll(".mobile-conversation-group").forEach((group) => {
      group.hidden = !group.querySelector(".mobile-conversation-row:not([hidden])");
    });
  });
});

const messageInput = document.querySelector("#message-input");
document.querySelector("#send-message").addEventListener("click", () => {
  const text = messageInput.value.trim();
  if (!text) return;

  const message = document.createElement("article");
  message.className = "message";
  message.innerHTML = `
    <span class="avatar avatar-user">林</span>
    <div class="message-copy">
      <header><strong>林嘉</strong><time>刚刚</time></header>
      <p></p>
    </div>`;
  message.querySelector("p").textContent = text;
  document.querySelector("#timeline").appendChild(message);
  messageInput.value = "";
  message.scrollIntoView({ behavior: "smooth", block: "nearest" });
});

messageInput.addEventListener("keydown", (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key === "Enter") {
    document.querySelector("#send-message").click();
  }
});

document.querySelector("#approve-action").addEventListener("click", (event) => {
  event.currentTarget.innerHTML = "<span>✓</span> 已批准";
  event.currentTarget.classList.add("is-approved");
  event.currentTarget.disabled = true;
  document.querySelector(".sheet-kicker.amber").innerHTML = "<i></i>已授权 · 10 分钟后失效";
});

const normalizedInitialView = normalizeView(initialView);
const initialUrl = new URL(window.location.href);
initialUrl.searchParams.set("view", normalizedInitialView);
window.history.replaceState({ view: normalizedInitialView, previous: null }, "", initialUrl);
renderView(normalizedInitialView, {
  moveFocus: normalizedInitialView === "task" || normalizedInitialView === "approval",
});
