const root = document.querySelector(".desktop-window");
const rootViews = new Set(["messages", "search", "activity", "profile"]);
const validViews = new Set([...rootViews, "channel", "task", "approval"]);
const isMobile = () => window.matchMedia("(max-width: 820px)").matches;
const requestedView = new URLSearchParams(window.location.search).get("view");
const initialView = requestedView === "home" ? "messages" : requestedView || (isMobile() ? "messages" : "channel");

function normalizeView(view) {
  if (view === "home") return "messages";
  return validViews.has(view) ? view : isMobile() ? "messages" : "channel";
}

function renderView(view) {
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

  if (rootViewActive) renderHomePanel(nextView === "search" ? "messages" : nextView);
  setMobileSearch(nextView === "search" && isMobile());

  document.querySelectorAll("[data-view-target]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.viewTarget === workbenchView);
  });
}

function navigateView(view, { replace = false } = {}) {
  const nextView = normalizeView(view);
  const currentView = root.dataset.mobileView || "channel";
  if (!replace && currentView === nextView) return;

  const url = new URL(window.location.href);
  url.searchParams.set("view", nextView);
  const state = { view: nextView, previous: replace ? null : currentView };
  window.history[replace ? "replaceState" : "pushState"](state, "", url);
  renderView(nextView);
}

function returnFromSheet() {
  if (window.history.state?.previous) {
    window.history.back();
    return;
  }
  const currentView = root.dataset.mobileView;
  const fallback = currentView === "approval" ? "task" : isMobile() ? "messages" : "channel";
  navigateView(fallback, { replace: true });
}

document.querySelectorAll("[data-view-target]").forEach((button) => {
  button.addEventListener("click", () => navigateView(button.dataset.viewTarget));
});

document.querySelectorAll(".close-sheet").forEach((button) => {
  button.addEventListener("click", returnFromSheet);
});

window.addEventListener("popstate", (event) => {
  const queryView = new URLSearchParams(window.location.search).get("view");
  renderView(event.state?.view || queryView || (isMobile() ? "messages" : "channel"));
});

window.addEventListener("resize", () => renderView(root.dataset.mobileView || "channel"));

document.querySelectorAll(".sheet-tabs").forEach((tabs) => {
  tabs.querySelectorAll("button").forEach((button) => {
    button.addEventListener("click", () => {
      tabs.querySelectorAll("button").forEach((item) => item.classList.remove("is-active"));
      button.classList.add("is-active");
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
renderView(normalizedInitialView);
