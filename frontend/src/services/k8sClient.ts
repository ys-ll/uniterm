import {
  K8sListContexts,
  K8sConnect,
  K8sDisconnect,
  K8sRequest,
  K8sStartWatch,
  K8sStopWatch,
  K8sStartLogStream,
  K8sStopLogStream,
  K8sExecSession,
} from "../../wailsjs/go/main/App";
import { EventsOn, EventsOff } from "../../wailsjs/runtime/runtime";
import type { K8sContextInfo, K8sWatchEvent } from "../types/k8s";

export async function listContexts(
  source: string,
  sourceIsPath: boolean,
): Promise<K8sContextInfo[]> {
  return (await K8sListContexts(source, sourceIsPath)) as K8sContextInfo[];
}

export async function connect(
  source: string,
  sourceIsPath: boolean,
  contextName: string,
  tunnelSSHConnId = "",
  tunnelSSHUser = "",
  tunnelSSHPassword = "",
  insecureTLS = false,
): Promise<string> {
  return await K8sConnect(
    source,
    sourceIsPath,
    contextName,
    tunnelSSHConnId,
    tunnelSSHUser,
    tunnelSSHPassword,
    insecureTLS,
  );
}

export function disconnect(connID: string): void {
  K8sDisconnect(connID);
}

export async function requestJSON<T = any>(
  connID: string,
  method: string,
  path: string,
  body: any = null,
  contentType = "",
): Promise<{ status: number; data: T | null; raw: string }> {
  const rawBody =
    body == null ? "" : typeof body === "string" ? body : JSON.stringify(body);
  const resp = await K8sRequest(connID, method, path, rawBody, contentType);
  let data: T | null = null;
  try {
    data = resp.body ? (JSON.parse(resp.body) as T) : null;
  } catch {
    data = null;
  }
  return { status: resp.status, data, raw: resp.body };
}

export interface WatchHandle {
  id: string;
  stop: () => void;
}

export async function startWatch(
  connID: string,
  path: string,
  onEvent: (ev: K8sWatchEvent) => void,
  onEnd?: (err: string) => void,
): Promise<WatchHandle> {
  const id = await K8sStartWatch(connID, path);
  const eventName = `k8s:watch:${id}`;
  const endName = `k8s:watch-end:${id}`;
  // Backend now batches events in 50ms windows and emits an array.
// The consumer still gets a single event at a time; we just iterate
// the array if a batch arrives.
  EventsOn(eventName, (payload: K8sWatchEvent | K8sWatchEvent[]) => {
    if (Array.isArray(payload)) {
      for (const ev of payload) onEvent(ev);
    } else if (payload) {
      onEvent(payload);
    }
  });
  EventsOn(endName, (payload: { error: string }) => {
    onEnd?.(payload?.error || "");
  });
  return {
    id,
    stop: () => {
      EventsOff(eventName);
      EventsOff(endName);
      K8sStopWatch(id);
    },
  };
}

export interface LogHandle {
  stop(): void;
}

export async function startLogStream(
  connId: string,
  ns: string,
  pod: string,
  container: string,
  tailLines: number,
  timestamps: boolean,
  previous: boolean,
  onLine: (line: string) => void,
  onEnd: (err: string) => void,
): Promise<LogHandle> {
  const streamId = await K8sStartLogStream(
    connId,
    ns,
    pod,
    container,
    tailLines,
    timestamps,
    previous,
  );
  const evName = `k8s:log:${streamId}`;
  const endName = `k8s:log-end:${streamId}`;
  // Backend now batches log lines in 50ms windows. Iterate batched
  // arrays exactly like the unwrapped single-line case so the consumer
  // doesn't need to change.
  EventsOn(evName, (payload: string | string[]) => {
    if (Array.isArray(payload)) {
      for (const l of payload) onLine(l);
    } else if (typeof payload === "string") {
      onLine(payload);
    }
  });
  EventsOn(endName, (p: any) => onEnd(p?.error || ""));
  return {
    stop() {
      EventsOff(evName);
      EventsOff(endName);
      K8sStopLogStream(streamId);
    },
  };
}

export async function execSession(
  connId: string,
  ns: string,
  pod: string,
  container: string,
) {
  return await K8sExecSession(connId, ns, pod, container);
}
