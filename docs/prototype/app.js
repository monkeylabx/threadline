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
    const v14Search = document.querySelector("[data-v14-search]");
    const unifiedSearch = document.querySelector("[data-unified-search]");
    if (document.body.classList.contains("is-im-agent-prototype") && v14Search) {
      v14Search.focus();
      v14Search.select();
    } else if (document.body.classList.contains("is-im-agent-prototype") && unifiedSearch) {
      unifiedSearch.focus();
      unifiedSearch.select();
    } else {
      switchRoute("search");
    }
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

// PROTOTYPE #183 — three disposable task-delivery layouts, enabled only by URL.
const initialParams = new URLSearchParams(window.location.search);
const artifactPrototypeEnabled = initialParams.get("prototype") === "artifact";
const artifactVariantNames = {
  A: "A · 画布审阅",
  B: "B · 版本舞台",
  C: "C · 交付三栏",
};
const artifactVariantKeys = Object.keys(artifactVariantNames);
const artifactSwitcher = document.querySelector("#artifact-prototype-switcher");
const artifactSwitcherLabel = document.querySelector("#artifact-prototype-label");

function setArtifactVariant(requestedVariant, { updateUrl = true } = {}) {
  if (!artifactPrototypeEnabled) return;
  const variant = artifactVariantNames[requestedVariant] ? requestedVariant : "A";
  document.querySelectorAll("[data-artifact-variant]").forEach((element) => {
    const selected = element.dataset.artifactVariant === variant;
    element.hidden = !selected;
    element.setAttribute("aria-hidden", String(!selected));
  });
  artifactSwitcherLabel.textContent = artifactVariantNames[variant];
  if (updateUrl) {
    const nextUrl = new URL(window.location.href);
    nextUrl.searchParams.set("prototype", "artifact");
    nextUrl.searchParams.set("variant", variant);
    nextUrl.searchParams.set("screen", "task-result");
    window.history.replaceState({}, "", nextUrl);
  }
}

if (artifactPrototypeEnabled) {
  document.body.classList.add("is-artifact-prototype");
  artifactSwitcher.hidden = false;
  artifactSwitcher.querySelectorAll("[data-artifact-cycle]").forEach((button) => {
    button.addEventListener("click", () => {
      const current = new URLSearchParams(window.location.search).get("variant") || "A";
      const currentIndex = artifactVariantKeys.indexOf(current);
      const step = button.dataset.artifactCycle === "previous" ? -1 : 1;
      const nextIndex = (currentIndex + step + artifactVariantKeys.length) % artifactVariantKeys.length;
      setArtifactVariant(artifactVariantKeys[nextIndex]);
    });
  });
  document.addEventListener("keydown", (event) => {
    const active = document.activeElement;
    const isEditing = active instanceof HTMLElement
      && (active.matches("input, textarea, [contenteditable='true']"));
    if (isEditing || (event.key !== "ArrowLeft" && event.key !== "ArrowRight")) return;
    event.preventDefault();
    const current = new URLSearchParams(window.location.search).get("variant") || "A";
    const currentIndex = artifactVariantKeys.indexOf(current);
    const step = event.key === "ArrowLeft" ? -1 : 1;
    const nextIndex = (currentIndex + step + artifactVariantKeys.length) % artifactVariantKeys.length;
    setArtifactVariant(artifactVariantKeys[nextIndex]);
  });
}

// PROTOTYPE #183 V2 — three disposable layouts for private making → public delivery.
const privatePublishPrototypeEnabled = initialParams.get("prototype") === "private-publish";
const privatePublishVariantNames = {
  A: "A · 双域工作台",
  B: "B · AI 主工作台",
  C: "C · 流转轨道",
};
const privatePublishVariantKeys = Object.keys(privatePublishVariantNames);
const privatePublishSwitcher = document.querySelector("#private-publish-switcher");
const privatePublishSwitcherLabel = document.querySelector("#private-publish-label");
let privatePublishState = "draft";

function setPrivatePublishVariant(requestedVariant, { updateUrl = true } = {}) {
  if (!privatePublishPrototypeEnabled) return;
  const variant = privatePublishVariantNames[requestedVariant] ? requestedVariant : "B";
  document.querySelectorAll("[data-private-publish-variant]").forEach((element) => {
    const selected = element.dataset.privatePublishVariant === variant;
    element.hidden = !selected;
    element.setAttribute("aria-hidden", String(!selected));
  });
  privatePublishSwitcherLabel.textContent = privatePublishVariantNames[variant];
  if (updateUrl) {
    const nextUrl = new URL(window.location.href);
    nextUrl.searchParams.set("prototype", "private-publish");
    nextUrl.searchParams.set("variant", variant);
    nextUrl.searchParams.set("screen", "channel");
    window.history.replaceState({}, "", nextUrl);
  }
}

function setPrivatePublishState(nextState) {
  const supportedStates = ["draft", "published", "revision", "v2", "forwarded"];
  privatePublishState = supportedStates.includes(nextState) ? nextState : "draft";
  const visibleState = privatePublishState === "forwarded" ? "v2" : privatePublishState;
  const stateLabels = {
    draft: "私人草稿",
    published: "v1 已发布",
    revision: "私人修订中",
    v2: "v2 已发布",
    forwarded: "已接受并流转",
  };
  const publicDecisions = {
    draft: "尚未发布",
    published: "等待团队审阅",
    revision: "修改请求已发送 · 公开状态：修改中",
    v2: "v2 等待团队审阅",
    forwarded: "已接受 · 已流转到 #发布协调",
  };

  document.querySelectorAll("[data-private-publish-state-label]").forEach((element) => {
    element.textContent = stateLabels[privatePublishState];
  });
  document.querySelectorAll("[data-artifact-version]").forEach((element) => {
    element.textContent = ["v2", "forwarded"].includes(privatePublishState) ? "v2" : "v1";
  });
  document.querySelectorAll("[data-public-decision]").forEach((element) => {
    element.textContent = publicDecisions[privatePublishState];
  });
  document.querySelectorAll(".private-publish-prototype [class*='when-']").forEach((element) => {
    element.hidden = !element.classList.contains(`when-${visibleState}`);
  });

  const stateOrder = { published: 1, revision: 2, v2: 3, forwarded: 3 };
  document.querySelectorAll("[data-flow-history]").forEach((element) => {
    const itemOrder = stateOrder[element.dataset.flowHistory] || 0;
    element.classList.toggle("is-current", itemOrder <= (stateOrder[privatePublishState] || 0));
  });
}

if (privatePublishPrototypeEnabled) {
  document.body.classList.add("is-private-publish-prototype");
  privatePublishSwitcher.hidden = false;
  privatePublishSwitcher.querySelectorAll("[data-private-publish-cycle]").forEach((button) => {
    button.addEventListener("click", () => {
      const current = new URLSearchParams(window.location.search).get("variant") || "B";
      const currentIndex = privatePublishVariantKeys.indexOf(current);
      const step = button.dataset.privatePublishCycle === "previous" ? -1 : 1;
      const nextIndex = (currentIndex + step + privatePublishVariantKeys.length) % privatePublishVariantKeys.length;
      setPrivatePublishVariant(privatePublishVariantKeys[nextIndex]);
    });
  });
  document.querySelectorAll("[data-private-publish-action]").forEach((button) => {
    button.addEventListener("click", () => {
      const action = button.dataset.privatePublishAction;
      const nextState = {
        publish: "published",
        "request-revision": "revision",
        "publish-v2": "v2",
        forward: "forwarded",
        reset: "draft",
      }[action];
      if (nextState) setPrivatePublishState(nextState);
    });
  });
  const privateAiInput = document.querySelector("[data-ai-composer]");
  const privateAiSend = document.querySelector("[data-ai-send]");
  const privateAiThread = document.querySelector("#private-ai-thread");
  const privateAiActivity = document.querySelector("[data-ai-activity]");
  const sendPrivateAiMessage = () => {
    const message = privateAiInput?.value.trim();
    if (!message || !privateAiThread) return;
    const userTurn = document.createElement("article");
    userTurn.className = "ai-turn is-user";
    userTurn.innerHTML = `<div class="ai-turn-body"><strong>你</strong><p></p></div><span class="avatar avatar-small avatar-user">林</span>`;
    userTurn.querySelector("p").textContent = message;
    const agentTurn = document.createElement("article");
    agentTurn.className = "ai-turn is-agent";
    agentTurn.innerHTML = `<span class="avatar avatar-small avatar-nova">N</span><div class="ai-turn-body"><strong>Nova</strong><p>收到。我会继续在私人草稿中处理，不会自动发布到频道。</p></div>`;
    privateAiThread.append(userTurn, agentTurn);
    privateAiInput.value = "";
    privateAiActivity.textContent = "已更新私人草稿";
    privateAiThread.scrollTop = privateAiThread.scrollHeight;
  };
  privateAiSend?.addEventListener("click", sendPrivateAiMessage);
  privateAiInput?.addEventListener("keydown", (event) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault();
      sendPrivateAiMessage();
    }
  });
  document.querySelector("[data-ai-context-toggle]")?.addEventListener("click", (event) => {
    const button = event.currentTarget;
    const added = button.classList.toggle("is-added");
    button.innerHTML = added ? "<span>✓</span>已作为工作上下文" : "<span>＋</span>加入工作上下文";
  });
  document.addEventListener("keydown", (event) => {
    const active = document.activeElement;
    const isEditing = active instanceof HTMLElement
      && active.matches("input, textarea, [contenteditable='true']");
    if (isEditing || (event.key !== "ArrowLeft" && event.key !== "ArrowRight")) return;
    event.preventDefault();
    const current = new URLSearchParams(window.location.search).get("variant") || "B";
    const currentIndex = privatePublishVariantKeys.indexOf(current);
    const step = event.key === "ArrowLeft" ? -1 : 1;
    const nextIndex = (currentIndex + step + privatePublishVariantKeys.length) % privatePublishVariantKeys.length;
    setPrivatePublishVariant(privatePublishVariantKeys[nextIndex]);
  });
}

// PROTOTYPE #183 V4 — three disposable communication layers for Agent focus work.
const imAgentPrototypeEnabled = initialParams.get("prototype") === "im-agent-fusion";
const imAgentVariantNames = {
  A: "A · 统一对话",
  B: "B · 通信边缘轨",
  C: "C · 固定频道",
};
const imAgentVariantKeys = Object.keys(imAgentVariantNames);
const imAgentSwitcher = document.querySelector("#im-agent-switcher");
const imAgentSwitcherLabel = document.querySelector("#im-agent-label");

function setImAgentVariant(requestedVariant, { updateUrl = true } = {}) {
  if (!imAgentPrototypeEnabled) return;
  const variant = imAgentVariantNames[requestedVariant] ? requestedVariant : "A";
  document.querySelectorAll("[data-im-agent-variant]").forEach((element) => {
    const selected = element.dataset.imAgentVariant === variant;
    element.hidden = !selected;
    element.setAttribute("aria-hidden", String(!selected));
  });
  document.querySelectorAll("[data-directory-menu]").forEach((menu) => { menu.hidden = true; });
  imAgentSwitcherLabel.textContent = imAgentVariantNames[variant];
  if (updateUrl) {
    const nextUrl = new URL(window.location.href);
    nextUrl.searchParams.set("screen", "channel");
    nextUrl.searchParams.set("prototype", "im-agent-fusion");
    nextUrl.searchParams.set("variant", variant);
    window.history.replaceState({}, "", nextUrl);
  }
}

if (imAgentPrototypeEnabled) {
  document.body.classList.add("is-im-agent-prototype");
  imAgentSwitcher.hidden = false;
  imAgentSwitcher.querySelectorAll("[data-im-agent-cycle]").forEach((button) => {
    button.addEventListener("click", () => {
      const current = new URLSearchParams(window.location.search).get("variant") || "A";
      const currentIndex = imAgentVariantKeys.indexOf(current);
      const step = button.dataset.imAgentCycle === "previous" ? -1 : 1;
      const nextIndex = (currentIndex + step + imAgentVariantKeys.length) % imAgentVariantKeys.length;
      setImAgentVariant(imAgentVariantKeys[nextIndex]);
    });
  });
  const directoryPicker = document.querySelector("[data-directory-picker]");
  const directoryMenu = document.querySelector("[data-directory-menu]");
  directoryPicker?.addEventListener("click", () => { directoryMenu.hidden = !directoryMenu.hidden; });
  document.querySelectorAll("[data-directory-option]").forEach((option) => {
    option.addEventListener("click", () => {
      document.querySelector("[data-directory-label]").textContent = option.dataset.directoryOption;
      document.querySelectorAll("[data-directory-option]").forEach((item) => {
        const selected = item === option;
        item.classList.toggle("is-current", selected);
        item.querySelector("i").textContent = selected ? "✓" : "";
      });
      directoryMenu.hidden = true;
    });
  });
  const unifiedSearch = document.querySelector("[data-unified-search]");
  const searchableNavItems = [...document.querySelectorAll(
    ".im-agent-variant-a .unified-channel, .im-agent-variant-a [data-agent-history-item], .im-agent-variant-a .unified-dm",
  )];
  const filterUnifiedNavigation = () => {
    const query = unifiedSearch.value.trim().toLocaleLowerCase("zh-CN");
    searchableNavItems.forEach((item) => {
      item.hidden = Boolean(query) && !item.textContent.toLocaleLowerCase("zh-CN").includes(query);
    });
    document.querySelectorAll(".im-agent-variant-a .unified-nav-section").forEach((section) => {
      const sectionItems = searchableNavItems.filter((item) => section.contains(item));
      section.classList.toggle("is-search-empty", Boolean(query) && sectionItems.every((item) => item.hidden));
    });
  };
  unifiedSearch?.addEventListener("input", filterUnifiedNavigation);
  unifiedSearch?.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    unifiedSearch.value = "";
    unifiedSearch.placeholder = "搜索频道、工作或成员";
    filterUnifiedNavigation();
    unifiedSearch.blur();
  });
  const communicationCreateMenu = document.querySelector("[data-communication-create-menu]");
  document.querySelector("[data-new-communication]")?.addEventListener("click", () => {
    communicationCreateMenu.hidden = !communicationCreateMenu.hidden;
  });
  document.querySelector("[data-new-conversation]")?.addEventListener("click", () => {
    communicationCreateMenu.hidden = true;
    unifiedSearch.value = "";
    unifiedSearch.placeholder = "输入姓名，搜索组织成员…";
    filterUnifiedNavigation();
    unifiedSearch.focus();
  });
  document.querySelector("[data-new-channel]")?.addEventListener("click", () => {
    const section = communicationCreateMenu.closest(".unified-nav-section");
    let draft = section.querySelector("[data-channel-draft]");
    if (!draft) {
      draft = document.createElement("button");
      draft.className = "unified-channel is-draft";
      draft.dataset.channelDraft = "";
      draft.innerHTML = "<b>#</b><span>未命名频道</span><i>新</i>";
      const firstConversation = section.querySelector(".unified-channel, .unified-dm");
      section.insertBefore(draft, firstConversation);
    }
    communicationCreateMenu.hidden = true;
    draft.focus();
  });
  document.querySelector("[data-attention-inbox]")?.addEventListener("click", (event) => {
    const expanded = event.currentTarget.classList.toggle("is-expanded");
    event.currentTarget.querySelector("small").textContent = expanded
      ? "待批准 1 · Agent 交付待确认 1"
      : "1 个审批 · 1 个 Agent 结果";
  });
  const focusAgentThread = document.querySelector(".im-agent-variant-a .focus-agent-thread");
  const defaultFocusThreadMarkup = focusAgentThread?.innerHTML || "";
  const historyPreviews = {
    "整理 Q3 客户反馈": ["把本季度的客户反馈按使用场景重新整理。", "已归并为上手、协作和权限三个主题，并保留了每条反馈的来源。"],
    "检查离线同步方案": ["检查离线恢复有没有覆盖新状态的风险。", "发现旧进程恢复时需要 fencing token；我已经把相关文件和测试列出来。"],
    "准备产品评审材料": ["把本周的产品决定整理成评审材料。", "已生成一页结论和三项待决定问题，当前仍是私人草稿。"],
    "分析移动端崩溃日志": ["分析昨天移动端启动崩溃的日志。", "主要问题集中在本地数据库恢复阶段，我标记了两个可复现路径。"],
  };
  document.querySelectorAll("[data-agent-history-item]").forEach((item) => {
    item.addEventListener("click", () => {
      document.querySelectorAll("[data-agent-history-item]").forEach((entry) => entry.classList.toggle("is-current", entry === item));
      const title = item.dataset.historyTitle;
      const parentChannel = item.dataset.historyChannel;
      document.querySelector("[data-agent-work-title]").textContent = title;
      document.querySelector("[data-work-parent]").textContent = parentChannel;
      document.querySelector("[data-work-context]").textContent = parentChannel === "未关联频道" ? parentChannel : `${parentChannel} · 已引用`;
      document.querySelector("[data-work-artifact]").textContent = title === "重做桌面端入职引导" ? "入职引导页 v1" : `${title} · 草稿`;
      if (title === "重做桌面端入职引导") {
        focusAgentThread.innerHTML = defaultFocusThreadMarkup;
        return;
      }
      const [request, response] = historyPreviews[title];
      focusAgentThread.innerHTML = `<div class="focus-day">历史工作 · 仅你可见</div><article class="focus-turn user"><div><b>你</b><p></p></div><span class="avatar avatar-small avatar-user">林</span></article><article class="focus-turn agent"><span class="avatar avatar-small avatar-nova">N</span><div><b>Nova</b><p></p></div></article>`;
      const paragraphs = focusAgentThread.querySelectorAll("p");
      paragraphs[0].textContent = request;
      paragraphs[1].textContent = response;
    });
  });
  const createAgentWork = () => {
    document.querySelectorAll("[data-agent-history-item]").forEach((item) => item.classList.remove("is-current"));
    document.querySelector("[data-agent-work-title]").textContent = "未命名工作";
    document.querySelector("[data-work-parent]").textContent = "未关联频道";
    document.querySelector("[data-work-context]").textContent = "未关联频道";
    document.querySelector("[data-work-artifact]").textContent = "新工作";
    focusAgentThread.innerHTML = `<div class="focus-day">新工作 · 仅你可见</div><article class="focus-turn agent"><span class="avatar avatar-small avatar-nova">N</span><div><b>Nova</b><p>这是一个新的私人工作。告诉我你想完成什么；需要团队上下文时，再明确引用频道消息。</p></div></article>`;
    const composer = document.querySelector(".im-agent-variant-a .focus-agent-composer textarea");
    composer.value = "";
    composer.focus();
  };
  document.querySelector("[data-new-agent-work]")?.addEventListener("click", createAgentWork);
  const unifiedNavToggle = document.querySelector("[data-unified-nav-toggle]");
  const unifiedNavStage = unifiedNavToggle?.closest(".overlay-stage");
  const setUnifiedNavCollapsed = (collapsed) => {
    unifiedNavStage.classList.toggle("is-nav-collapsed", collapsed);
    unifiedNavToggle.setAttribute("aria-expanded", String(!collapsed));
    unifiedNavToggle.setAttribute("aria-label", collapsed ? "展开侧栏" : "收起侧栏");
    unifiedNavToggle.title = collapsed ? "展开侧栏" : "收起侧栏";
  };
  unifiedNavToggle?.addEventListener("click", (event) => {
    setUnifiedNavCollapsed(!unifiedNavStage.classList.contains("is-nav-collapsed"));
    if (event.detail) event.currentTarget.blur();
  });
  document.querySelectorAll("[data-collapsed-nav-target]").forEach((button) => {
    button.addEventListener("click", () => {
      setUnifiedNavCollapsed(false);
      const targetSelectors = {
        attention: "[data-attention-inbox]",
        communication: ".communication-section",
        work: ".agent-history-list",
      };
      document.querySelector(`.im-agent-variant-a ${targetSelectors[button.dataset.collapsedNavTarget]}`)?.scrollIntoView({ block: "nearest" });
    });
  });
  const v14Shell = document.querySelector("[data-v14-shell]");
  const v14ContextToggle = document.querySelector("[data-v14-context-toggle]");
  const v14OrgToggle = document.querySelector("[data-v14-org-toggle]");
  const v14OrgReveal = document.querySelector("[data-v14-org-reveal]");
  const v14Search = document.querySelector("[data-v14-search]");
  const v14SearchResults = document.querySelector("[data-v14-search-results]");
  const setV14App = (app) => {
    v14Shell.dataset.v14App = app;
    document.querySelectorAll("[data-v14-app-button]").forEach((button) => button.classList.toggle("is-current", button.dataset.v14AppButton === app));
    document.querySelectorAll("[data-v14-context]").forEach((panel) => { panel.hidden = panel.dataset.v14Context !== app; });
    document.querySelectorAll("[data-v14-content]").forEach((panel) => { panel.hidden = panel.dataset.v14Content !== app; });
    v14SearchResults.hidden = true;
  };

  const setV14ContextCollapsed = (collapsed) => {
    if (!v14Shell || !v14ContextToggle) return;
    v14Shell.classList.toggle("is-context-collapsed", collapsed);
    v14ContextToggle.setAttribute("aria-expanded", String(!collapsed));
    v14ContextToggle.setAttribute("aria-label", collapsed ? "展开列表" : "收起列表");
    v14ContextToggle.title = collapsed ? "展开列表" : "收起列表";
  };

  v14ContextToggle?.addEventListener("click", (event) => {
    setV14ContextCollapsed(!v14Shell?.classList.contains("is-context-collapsed"));
    if (event.detail) event.currentTarget.blur();
  });

  const setV14OrgCollapsed = (collapsed) => {
    if (!v14Shell || !v14OrgToggle) return;
    v14Shell.classList.toggle("is-org-collapsed", collapsed);
    v14OrgToggle.setAttribute("aria-expanded", String(!collapsed));
    v14OrgToggle.setAttribute("aria-label", collapsed ? "展开企业列表" : "收起企业列表");
    v14OrgToggle.title = collapsed ? "展开企业列表" : "收起企业列表";
  };

  v14OrgToggle?.addEventListener("click", (event) => {
    setV14OrgCollapsed(true);
    if (event.detail) event.currentTarget.blur();
  });

  v14OrgReveal?.addEventListener("click", (event) => {
    setV14OrgCollapsed(false);
    if (event.detail) event.currentTarget.blur();
  });
  document.querySelectorAll("[data-v14-app-button]").forEach((button) => {
    button.addEventListener("click", () => setV14App(button.dataset.v14AppButton));
  });
  const filterV14Search = () => {
    const query = v14Search.value.trim().toLocaleLowerCase("zh-CN");
    document.querySelectorAll("[data-v14-search-item]").forEach((item) => {
      item.hidden = Boolean(query) && !item.textContent.toLocaleLowerCase("zh-CN").includes(query);
    });
  };
  v14Search?.addEventListener("focus", () => { v14SearchResults.hidden = false; });
  v14Search?.addEventListener("input", () => {
    filterV14Search();
    v14SearchResults.hidden = false;
  });
  v14Search?.addEventListener("keydown", (event) => {
    if (event.key !== "Escape") return;
    v14Search.value = "";
    filterV14Search();
    v14SearchResults.hidden = true;
    v14Search.blur();
  });
  document.querySelectorAll("[data-v14-search-item]").forEach((item) => {
    item.addEventListener("click", () => {
      setV14App(item.dataset.resultApp);
      v14Search.value = item.querySelector("b").textContent;
    });
  });
  document.querySelector("[data-v14-new-work]")?.addEventListener("click", () => {
    setV14App("work");
    document.querySelector("#v14-work-title").textContent = "未命名工作";
    const textarea = document.querySelector(".v14-work-content .focus-agent-composer textarea");
    textarea.value = "";
    textarea.focus();
  });
  document.addEventListener("keydown", (event) => {
    const active = document.activeElement;
    const isEditing = active instanceof HTMLElement && active.matches("input, textarea, [contenteditable='true']");
    if (isEditing || (event.key !== "ArrowLeft" && event.key !== "ArrowRight")) return;
    event.preventDefault();
    const current = new URLSearchParams(window.location.search).get("variant") || "A";
    const currentIndex = imAgentVariantKeys.indexOf(current);
    const step = event.key === "ArrowLeft" ? -1 : 1;
    setImAgentVariant(imAgentVariantKeys[(currentIndex + step + imAgentVariantKeys.length) % imAgentVariantKeys.length]);
  });
}

switchRoute(initialParams.get("screen") || (artifactPrototypeEnabled ? "task-result" : "channel"));
if (artifactPrototypeEnabled) setArtifactVariant(initialParams.get("variant"), { updateUrl: true });
if (privatePublishPrototypeEnabled) {
  setPrivatePublishVariant(initialParams.get("variant"), { updateUrl: true });
  setPrivatePublishState("draft");
  document.querySelector("#view-title").textContent = "我的 AI 工作台";
  document.querySelector("#view-subtitle").textContent = "私人对话、工作资料与成果 · 仅你可见";
  document.querySelector(".channel-hash").textContent = "";
}
if (imAgentPrototypeEnabled) {
  setImAgentVariant(initialParams.get("variant"), { updateUrl: true });
  document.querySelector("#view-title").textContent = "Agent 工作";
  document.querySelector("#view-subtitle").textContent = "专注工作中 · IM 保持可达";
  document.querySelector(".channel-hash").textContent = "";
}
if (initialParams.get("modal") === "task") setModal(true);
