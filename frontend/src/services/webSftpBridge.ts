/**
 * frontend/src/services/webSftpBridge.ts
 *
 * Browser-side adaptation layer for uniTerm running in Wails v3 Server Mode (-tags server).
 * Provides:
 * 1. Direct browser SFTP/SCP file streaming download & multi-file .tar.gz archive download
 * 2. Direct browser SFTP/SCP streaming upload (File picker + Drag-and-Drop folders) with progress & cancellation
 * 3. Browser file staging to replace native OS file dialogs (app.Dialog.OpenFile) for SSH keys, SQLite & imports
 * 4. Multi-user session inspection, user management (admin), and logout helpers
 */

import { setLocalTerminalSupport } from '../utils/platform'

export interface WebModeStatus {
  serverMode: boolean;
  user: string;
  role: 'admin' | 'user' | string;
  homeDir: string;
  /** Whether container-local terminals are enabled (UNITERM_ENABLE_LOCAL_SHELL). */
  localShell?: boolean;
}

export interface BrowserUploadProgress {
  transferId: string;
  loadedBytes: number;
  totalBytes: number;
  percent: number;
}

export interface BrowserUploadOptions {
  sessionId: string;
  remoteDir: string;
  files?: FileList | File[];
  directoryMode?: boolean;
  onProgress?: (progress: BrowserUploadProgress) => void;
  signal?: AbortSignal;
}

export interface StagedLocalFileResult {
  status: string;
  stagedPath: string;
  fileName: string;
  size: number;
}

let cachedWebMode: WebModeStatus | null = null;

/**
 * Checks whether uniTerm is running in Wails v3 Server Mode (Docker/Browser)
 * and returns the current isolated tenant user info.
 */
export async function getWebModeStatus(forceRefresh = false): Promise<WebModeStatus> {
  if (cachedWebMode && !forceRefresh) {
    return cachedWebMode;
  }
  try {
    const resp = await fetch('/api/web/mode', {
      method: 'GET',
      credentials: 'same-origin',
    });
    if (resp.status === 401) {
      window.location.href = '/login';
      throw new Error('Unauthorized');
    }
    if (!resp.ok) {
      cachedWebMode = { serverMode: false, user: 'local', role: 'admin', homeDir: '' };
      return cachedWebMode;
    }
    cachedWebMode = (await resp.json()) as WebModeStatus;
    setLocalTerminalSupport(cachedWebMode.serverMode ? Boolean(cachedWebMode.localShell) : true)
    return cachedWebMode;
  } catch {
    cachedWebMode = { serverMode: false, user: 'local', role: 'admin', homeDir: '' };
    return cachedWebMode;
  }
}

/**
 * Triggers a direct browser streaming download of a single remote file over SFTP/SCP.
 * Zero memory buffering in JS — the browser's native download manager handles the stream.
 */
export function downloadRemoteFileInBrowser(sessionId: string, remotePath: string, transferId?: string): void {
  const params = new URLSearchParams({
    sessionId,
    path: remotePath,
  });
  if (transferId) {
    params.set('transferId', transferId);
  }
  triggerHiddenDownloadLink(`/api/web/sftp/download?${params.toString()}`);
}

/**
 * Streams multiple remote files or directories as an on-the-fly .tar.gz archive to the browser.
 */
export function downloadRemoteArchiveInBrowser(
  sessionId: string,
  remotePaths: string[],
  archiveName?: string,
): void {
  const params = new URLSearchParams({
    sessionId,
    paths: JSON.stringify(remotePaths),
  });
  if (archiveName) {
    params.set('name', archiveName);
  }
  triggerHiddenDownloadLink(`/api/web/sftp/download-archive?${params.toString()}`);
}

/**
 * Opens the browser's native file picker (if `options.files` is omitted) and streams
 * the selected files directly to the remote SFTP/SCP directory via multipart/form-data.
 */
export async function uploadFilesToRemoteInBrowser(
  options: BrowserUploadOptions,
): Promise<{ transferId: string; uploadedFiles: string[] }> {
  const selectedFiles =
    options.files && options.files.length > 0
      ? Array.from(options.files)
      : await promptBrowserFileInput({ multiple: true, directory: options.directoryMode });

  if (!selectedFiles.length) {
    return { transferId: '', uploadedFiles: [] };
  }

  const transferId = generateClientTransferId();
  const formData = new FormData();

  for (const file of selectedFiles) {
    const relPath = (file as File & { webkitRelativePath?: string }).webkitRelativePath || file.name;
    formData.append('relativePath', relPath);
    formData.append('file', file, file.name);
  }

  const params = new URLSearchParams({
    sessionId: options.sessionId,
    remoteDir: options.remoteDir,
    transferId,
  });

  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest();
    xhr.open('POST', `/api/web/sftp/upload?${params.toString()}`, true);
    xhr.withCredentials = true;
    xhr.setRequestHeader('X-Transfer-ID', transferId);

    if (options.signal) {
      options.signal.addEventListener('abort', () => {
        xhr.abort();
        void cancelBrowserSftpTransfer(transferId);
      });
    }

    xhr.upload.onprogress = (event) => {
      if (event.lengthComputable && options.onProgress) {
        options.onProgress({
          transferId,
          loadedBytes: event.loaded,
          totalBytes: event.total,
          percent: Math.round((event.loaded / event.total) * 100),
        });
      }
    };

    xhr.onload = () => {
      if (xhr.status === 401) {
        window.location.href = '/login';
        reject(new Error('Session expired'));
        return;
      }
      try {
        const body = JSON.parse(xhr.responseText || '{}');
        if (xhr.status >= 200 && xhr.status < 300) {
          resolve({
            transferId: body.transferId || transferId,
            uploadedFiles: body.uploadedFiles || [],
          });
        } else {
          reject(new Error(body.error || `Upload failed with HTTP ${xhr.status}`));
        }
      } catch (err) {
        reject(err instanceof Error ? err : new Error(String(err)));
      }
    };

    xhr.onerror = () => reject(new Error('Network error during streaming SFTP upload'));
    xhr.onabort = () => reject(new Error('SFTP upload cancelled'));
    xhr.send(formData);
  });
}

/**
 * Cancels an active streaming upload or download on the Go backend.
 */
export async function cancelBrowserSftpTransfer(transferId: string): Promise<void> {
  if (!transferId) return;
  await fetch(`/api/web/sftp/cancel?transferId=${encodeURIComponent(transferId)}`, {
    method: 'POST',
    credentials: 'same-origin',
  });
}

/**
 * Replaces native OS file dialogs (app.Dialog.OpenFile) in Web Server Mode.
 * Prompts the user to pick a local file in their browser (e.g., SSH private key,
 * SQLite database, or connection import file), uploads it to the user's isolated
 * staging directory in the container, and returns the container file path.
 *
 * `opts.keep` places the file in the janitor-exempt keep/ area — use it when
 * the returned path will be stored in a saved config (SSH keyPath, SQLite
 * host, kubeconfig) rather than consumed immediately.
 */
export async function pickAndStageLocalFileInBrowser(
  accept = '*/*',
  opts?: { keep?: boolean },
): Promise<StagedLocalFileResult | null> {
  const files = await promptBrowserFileInput({ multiple: false, accept });
  if (!files.length) {
    return null;
  }
  return stageLocalFilesInBrowser(files[0], opts);
}

/**
 * Stages an already-picked File object (from any browser file input or
 * drag-and-drop) into the worker's staging area. See
 * pickAndStageLocalFileInBrowser for the keep semantics.
 */
export async function stageLocalFilesInBrowser(
  file: File,
  opts?: { keep?: boolean },
): Promise<StagedLocalFileResult> {
  const formData = new FormData();
  if (opts?.keep) {
    formData.append('keep', '1');
  }
  formData.append('file', file, file.name);

  const resp = await fetch('/api/web/dialog/stage-file', {
    method: 'POST',
    body: formData,
    credentials: 'same-origin',
  });
  if (!resp.ok) {
    const errBody = await resp.json().catch(() => ({}));
    throw new Error(errBody.error || `Failed to stage local file (HTTP ${resp.status})`);
  }
  return (await resp.json()) as StagedLocalFileResult;
}

/**
 * Reads a picked File as text — the web-mode replacement for Go-side
 * "return the file content" dialogs (OpenPrivateKeyFile, OpenKubeconfigFile)
 * and ReadFileBase64 consumers.
 */
export function readBrowserFileAsText(file: File): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result ?? ''));
    reader.onerror = () => reject(reader.error ?? new Error('failed to read file'));
    reader.readAsText(file);
  });
}

/** True when the app runs in the headless server mode (browser deployment). */
export async function isWebServerMode(): Promise<boolean> {
  try {
    return (await getWebModeStatus()).serverMode;
  } catch {
    return false;
  }
}

/**
 * Multi-user gateway helpers: Logout and User Administration (for admin users).
 */
export async function logoutWebUser(): Promise<void> {
  await fetch('/api/auth/logout', { method: 'POST', credentials: 'same-origin' });
  window.location.href = '/login';
}

export async function changeWebPassword(currentPassword: string, newPassword: string): Promise<void> {
  const resp = await fetch('/api/auth/password', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ currentPassword, newPassword }),
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({}));
    throw new Error(err.error || 'Failed to change password');
  }
}

export async function listGatewayUsers(): Promise<
  Array<{ username: string; role: string; createdAt: string; disabled?: boolean }>
> {
  const resp = await fetch('/api/auth/users', { credentials: 'same-origin' });
  if (!resp.ok) {
    throw new Error('Failed to load users');
  }
  const data = await resp.json();
  return data.users || [];
}

export async function createGatewayUser(
  username: string,
  password: string,
  role: 'admin' | 'user' = 'user',
): Promise<void> {
  const resp = await fetch('/api/auth/users', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ username, password, role }),
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({}));
    throw new Error(err.error || 'Failed to create user');
  }
}

export async function deleteGatewayUser(username: string, purge = false): Promise<void> {
  const resp = await fetch(`/api/auth/users/${encodeURIComponent(username)}?purge=${purge}`, {
    method: 'DELETE',
    credentials: 'same-origin',
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({}));
    throw new Error(err.error || 'Failed to delete user');
  }
}

/** Self-service password change (POST /api/auth/password). */
export async function changeOwnPassword(currentPassword: string, newPassword: string): Promise<void> {
  const resp = await fetch('/api/auth/password', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ currentPassword, newPassword }),
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({}));
    throw new Error(err.error || 'Failed to change password');
  }
}

/** Admin-only password reset for any account (POST /api/auth/users/<name>/password). */
export async function adminResetUserPassword(username: string, newPassword: string): Promise<void> {
  const resp = await fetch(`/api/auth/users/${encodeURIComponent(username)}/password`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    credentials: 'same-origin',
    body: JSON.stringify({ newPassword }),
  });
  if (!resp.ok) {
    const err = await resp.json().catch(() => ({}));
    throw new Error(err.error || 'Failed to reset password');
  }
}

// --- Internal Browser DOM Helpers ---

/**
 * Opens the browser's native file picker and resolves with the picked File
 * objects. The web-mode replacement for native "pick a file/dialog" call
 * sites whose content is read client-side.
 */
export function pickBrowserFiles(opts?: {
  multiple?: boolean;
  accept?: string;
  directory?: boolean;
}): Promise<File[]> {
  return promptBrowserFileInput({
    multiple: opts?.multiple,
    accept: opts?.accept,
    directory: opts?.directory,
  });
}

function triggerHiddenDownloadLink(url: string): void {
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.style.display = 'none';
  document.body.appendChild(anchor);
  anchor.click();
  setTimeout(() => {
    document.body.removeChild(anchor);
  }, 1000);
}

function promptBrowserFileInput(opts: {
  multiple?: boolean;
  directory?: boolean;
  accept?: string;
}): Promise<File[]> {
  return new Promise((resolve) => {
    const input = document.createElement('input');
    input.type = 'file';
    input.multiple = Boolean(opts.multiple);
    if (opts.accept) {
      input.accept = opts.accept;
    }
    if (opts.directory) {
      input.setAttribute('webkitdirectory', '');
      input.setAttribute('directory', '');
    }
    input.style.display = 'none';
    document.body.appendChild(input);

    input.addEventListener('change', () => {
      const files = input.files ? Array.from(input.files) : [];
      document.body.removeChild(input);
      resolve(files);
    });
    input.addEventListener('cancel', () => {
      document.body.removeChild(input);
      resolve([]);
    });

    input.click();
  });
}

function generateClientTransferId(): string {
  const bytes = new Uint8Array(12);
  window.crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, '0')).join('');
}
