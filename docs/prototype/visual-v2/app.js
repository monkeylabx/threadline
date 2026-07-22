const root = document.querySelector(".desktop-window");
const validViews = new Set(["channel", "task", "approval"]);
const initialView = new URLSearchParams(window.location.search).get("view") || "channel";

function renderView(view) {
  const nextView = validViews.has(view) ? view : "channel";
  root.dataset.view = nextView;
  closeConversationSheet();

  document.querySelectorAll("[data-view-target]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.viewTarget === nextView);
  });
}

function navigateView(view, { replace = false } = {}) {
  const nextView = validViews.has(view) ? view : "channel";
  const currentView = root.dataset.view || "channel";
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
  navigateView("channel", { replace: true });
}

document.querySelectorAll("[data-view-target]").forEach((button) => {
  button.addEventListener("click", () => navigateView(button.dataset.viewTarget));
});

document.querySelectorAll(".close-sheet").forEach((button) => {
  button.addEventListener("click", returnFromSheet);
});

window.addEventListener("popstate", (event) => {
  const queryView = new URLSearchParams(window.location.search).get("view");
  renderView(event.state?.view || queryView || "channel");
});

document.querySelectorAll(".sheet-tabs").forEach((tabs) => {
  tabs.querySelectorAll("button").forEach((button) => {
    button.addEventListener("click", () => {
      tabs.querySelectorAll("button").forEach((item) => item.classList.remove("is-active"));
      button.classList.add("is-active");
    });
  });
});

const conversationSheet = document.querySelector(".mobile-conversation-sheet");

function openConversationSheet() {
  if (!window.matchMedia("(max-width: 820px)").matches) return;
  root.classList.add("is-conversation-sheet-open");
  conversationSheet.setAttribute("aria-hidden", "false");
}

function closeConversationSheet() {
  root.classList.remove("is-conversation-sheet-open");
  conversationSheet.setAttribute("aria-hidden", "true");
}

document.querySelector(".mobile-space-trigger").addEventListener("click", openConversationSheet);
document.querySelector(".mobile-sheet-scrim").addEventListener("click", closeConversationSheet);
document.querySelector(".mobile-conversation-close").addEventListener("click", closeConversationSheet);
document.querySelectorAll(".mobile-conversation-row").forEach((button) => {
  button.addEventListener("click", closeConversationSheet);
});

document.addEventListener("keydown", (event) => {
  if (event.key === "Escape" && root.classList.contains("is-conversation-sheet-open")) {
    closeConversationSheet();
  }
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

const normalizedInitialView = validViews.has(initialView) ? initialView : "channel";
const initialUrl = new URL(window.location.href);
initialUrl.searchParams.set("view", normalizedInitialView);
window.history.replaceState({ view: normalizedInitialView, previous: null }, "", initialUrl);
renderView(normalizedInitialView);
