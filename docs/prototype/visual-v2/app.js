const root = document.querySelector(".desktop-window");
const validViews = new Set(["channel", "task", "approval"]);

function setView(view) {
  const nextView = validViews.has(view) ? view : "channel";
  root.dataset.view = nextView;

  document.querySelectorAll("[data-view-target]").forEach((button) => {
    button.classList.toggle("is-active", button.dataset.viewTarget === nextView);
  });

  const url = new URL(window.location.href);
  url.searchParams.set("view", nextView);
  window.history.replaceState({}, "", url);
}

document.querySelectorAll("[data-view-target]").forEach((button) => {
  button.addEventListener("click", () => setView(button.dataset.viewTarget));
});

document.querySelectorAll(".close-sheet").forEach((button) => {
  button.addEventListener("click", () => setView("channel"));
});

document.querySelectorAll(".sheet-tabs").forEach((tabs) => {
  tabs.querySelectorAll("button").forEach((button) => {
    button.addEventListener("click", () => {
      tabs.querySelectorAll("button").forEach((item) => item.classList.remove("is-active"));
      button.classList.add("is-active");
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

setView(new URLSearchParams(window.location.search).get("view") || "channel");
