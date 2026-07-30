import { defineStore } from "pinia";
import { computed, markRaw, shallowRef } from "vue";
import type { Panel, PanelStatus, ConnectionConfig } from "../types/workspace";
import {
  DisableSessionOutputLog,
  RegisterSessionForPanel,
  UnregisterSession,
} from "../../wailsjs/go/main/App";

export interface TransferTaskUI {
  id: string;
  type: "upload" | "download";
  name: string;
  percentage: number;
  speed: string;
  eta: string;
  status: "running" | "paused" | "done" | "error" | "cancelled";
  lastBytes: number;
  lastTime: number;
  total: number;
}

export interface VNCCache {
  rfb: any;
  container: HTMLDivElement;
}

export interface SPICECache {
  sc: any;
  container: HTMLDivElement;
}

// Panel state is a hot tree (hundreds of panels possible, every drag
// tick mutates layout-related fields). Wrapping it in reactive()
// deep-proxied every nested Panel on assignment and paid proxy-getter
// overhead on every read. shallowRef + markRaw makes the Maps themselves
// reactive but each Panel object is treated as opaque — mutating its
// fields does NOT invalidate watchers, which is what we want here:
// panel replacement is a full Panel object (e.g. on drag end) rather
// than a per-field mutation during the drag itself, so deep tracking
// was wasted work.
const panelStateRef = shallowRef({
  panels: markRaw(new Map<string, Panel>()),
  transferTasks: markRaw(new Map<string, TransferTaskUI[]>()),
  proxyAddrs: markRaw(new Map<string, string>()),
  vncCaches: markRaw(new Map<string, VNCCache>()),
  spiceCaches: markRaw(new Map<string, SPICECache>()),
});

function panelState() {
  return panelStateRef.value;
}

// Drag tick batching: drag operations rewrite the panels map wholesale.
// Callers that previously mutated individual fields during a drag tick
// now batch into a "draft" Map and atomically replace via
// commitPanelLayout() once at drag end.
const layoutDraft = shallowRef<Map<string, Panel> | null>(null);

export const usePanelStore = defineStore("panel", () => {
  function getPanelsMap(): Map<string, Panel> {
    const draft = layoutDraft.value;
    return draft ?? panelState().panels;
  }

  function makeTitleUnique(title: string, excludePanelId?: string): string {
    const others = [...panelState().panels.values()]
      .filter((p) => p.id !== excludePanelId)
      .map((p) => p.title);
    if (!others.includes(title)) return title;
    let n = 2;
    while (others.includes(`${title} (${n})`)) n++;
    return `${title} (${n})`;
  }

  function createPanel(
    config: ConnectionConfig | null,
    type: Panel["type"] = "ssh",
  ): Panel {
    const id = `panel-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    let title: string;
    if (type === "local") {
      title = "Local";
    } else if (config) {
      title = `${config.host} ${config.user}`;
    } else {
      title = "New Panel";
    }
    const uniqueTitle = makeTitleUnique(title);
    const panel: Panel = {
      id,
      tabId: "",
      type,
      sessionId: null,
      title: uniqueTitle,
      status: "disconnected",
      config,
    };
    panelState().panels.set(id, panel);
    return panel;
  }

  function removePanel(id: string) {
    const p = panelState().panels.get(id);
    DisableSessionOutputLog(id).catch(() => {});
    if (p?.sessionId) {
      UnregisterSession(p.sessionId).catch(() => {});
    }
    panelState().panels.delete(id);
  }

  function getPanel(id: string): Panel | undefined {
    return panelState().panels.get(id);
  }

  function bindSession(panelId: string, sessionId: string) {
    const p = panelState().panels.get(panelId);
    if (!p) return;
    const prev = p.sessionId;
    p.sessionId = sessionId;
    if (prev && prev !== sessionId) {
      UnregisterSession(prev).catch(() => {});
    }
    if (sessionId) {
      RegisterSessionForPanel(sessionId, panelId).catch(() => {});
    }
  }

  function updateStatus(panelId: string, status: PanelStatus) {
    const p = panelState().panels.get(panelId);
    if (p) p.status = status;
  }

  function updateTitle(panelId: string, title: string) {
    const p = panelState().panels.get(panelId);
    if (!p) return;
    p.title = makeTitleUnique(title, panelId);
  }

  function setOutputLog(
    panelId: string,
    state: { enabled: boolean; path: string },
  ) {
    const p = panelState().panels.get(panelId);
    if (p) p.outputLog = state;
  }

  function movePanelToTab(panelId: string, tabId: string) {
    const p = panelState().panels.get(panelId);
    if (p) p.tabId = tabId;
  }

  function getTransferTasks(panelId: string): TransferTaskUI[] {
    const tasks = panelState().transferTasks;
    if (!tasks.has(panelId)) {
      tasks.set(panelId, []);
    }
    return tasks.get(panelId)!;
  }

  function setProxyAddr(panelId: string, addr: string) {
    panelState().proxyAddrs.set(panelId, addr);
  }

  function getProxyAddr(panelId: string): string | undefined {
    return panelState().proxyAddrs.get(panelId);
  }

  function removeProxyAddr(panelId: string) {
    panelState().proxyAddrs.delete(panelId);
  }

  function setVNCCache(panelId: string, cache: VNCCache) {
    panelState().vncCaches.set(panelId, cache);
  }

  function getVNCCache(panelId: string): VNCCache | undefined {
    return panelState().vncCaches.get(panelId);
  }

  function removeVNCCache(panelId: string) {
    const cached = panelState().vncCaches.get(panelId);
    if (cached) {
      if (cached.container.parentNode) {
        cached.container.parentNode.removeChild(cached.container);
      }
      panelState().vncCaches.delete(panelId);
    }
  }

  function disconnectVNCCache(panelId: string) {
    const cached = panelState().vncCaches.get(panelId);
    if (cached) {
      try {
        cached.rfb?.disconnect();
      } catch (_) {}
    }
  }

  function setSPICECache(panelId: string, cache: SPICECache) {
    panelState().spiceCaches.set(panelId, cache);
  }

  function getSPICECache(panelId: string): SPICECache | undefined {
    return panelState().spiceCaches.get(panelId);
  }

  function removeSPICECache(panelId: string) {
    const cached = panelState().spiceCaches.get(panelId);
    if (cached) {
      if (cached.container.parentNode) {
        cached.container.parentNode.removeChild(cached.container);
      }
      panelState().spiceCaches.delete(panelId);
    }
  }

  function disconnectSPICECache(panelId: string) {
    const cached = panelState().spiceCaches.get(panelId);
    if (cached) {
      try {
        cached.sc?.stop();
      } catch (_) {}
    }
  }

  // beginLayoutDraft swaps the active panels map to a fresh draft. Drag
  // handlers write into the draft; consumers read via getPanelsMap()
  // which sees the draft. commitLayout() atomically promotes the draft
  // into panelState so a single reactive tick notifies consumers instead
  // of one tick per drag delta.
  function beginLayoutDraft(): Map<string, Panel> {
    const draft = new Map(panelState().panels);
    layoutDraft.value = draft;
    return draft;
  }

  function commitLayout() {
    const draft = layoutDraft.value;
    if (!draft) return;
    panelStateRef.value = {
      ...panelState(),
      panels: markRaw(draft),
    };
    layoutDraft.value = null;
  }

  function cancelLayoutDraft() {
    layoutDraft.value = null;
  }

  return {
    panels: computed(() => getPanelsMap()),
    transferTasks: panelState().transferTasks,
    proxyAddrs: panelState().proxyAddrs,
    vncCaches: panelState().vncCaches,
    spiceCaches: panelState().spiceCaches,
    getPanelsMap,
    beginLayoutDraft,
    commitLayout,
    cancelLayoutDraft,
    getTransferTasks,
    createPanel,
    removePanel,
    getPanel,
    bindSession,
    updateStatus,
    updateTitle,
    setOutputLog,
    movePanelToTab,
    setProxyAddr,
    getProxyAddr,
    removeProxyAddr,
    setVNCCache,
    getVNCCache,
    removeVNCCache,
    disconnectVNCCache,
    setSPICECache,
    getSPICECache,
    removeSPICECache,
    disconnectSPICECache,
  };
});
