<template>
  <div v-if="webStatus.serverMode" class="uniterm-web-floating-bar">
    <div class="uwb-pill" @click="panelOpen = !panelOpen">
      <span class="uwb-dot"></span>
      <span class="uwb-user">{{ webStatus.user }} ({{ webStatus.role }})</span>
      <span class="uwb-divider">|</span>
      <span class="uwb-action">Web SFTP 流式传输 & 多用户控制台</span>
    </div>

    <div v-if="panelOpen" class="uwb-drawer">
      <div class="uwb-header">
        <strong>uniTerm Web 控制台（用户：{{ webStatus.user }}）</strong>
        <button class="uwb-close" @click="panelOpen = false">×</button>
      </div>

      <div class="uwb-section">
        <div class="uwb-title">浏览器本地流式 SFTP 传输</div>
        <div class="uwb-row">
          <input v-model="sessionId" placeholder="输入或粘贴当前活动 SFTP/SSH Session ID" class="uwb-input" />
        </div>
        <div class="uwb-row">
          <input v-model="remotePath" placeholder="远程路径（如 /root/app.tar.gz 或上传目标目录 /tmp）" class="uwb-input" />
        </div>
        <div class="uwb-btn-group">
          <button class="uwb-btn" @click="handleDownload">浏览器流式下载文件</button>
          <button class="uwb-btn" @click="handleUpload(false)">选择本地文件流式上传</button>
          <button class="uwb-btn" @click="handleUpload(true)">上传整个本地文件夹</button>
        </div>
        <div v-if="uploadProgress > 0" class="uwb-progress">
          上传进度：{{ uploadProgress }}%
        </div>
        <div v-if="statusMsg" class="uwb-msg">{{ statusMsg }}</div>
      </div>

      <div v-if="webStatus.role === 'admin'" class="uwb-section">
        <div class="uwb-title">多用户隔离账号管理（Admin）</div>
        <div class="uwb-row uwb-flex">
          <input v-model="newUsername" placeholder="新用户名" class="uwb-input" />
          <input v-model="newPassword" type="password" placeholder="初始密码（>=6位）" class="uwb-input" />
          <button class="uwb-btn" @click="handleCreateUser">新增用户</button>
        </div>
        <ul class="uwb-user-list">
          <li v-for="u in userList" :key="u.username">
            <span>{{ u.username }} ({{ u.role }})</span>
            <button v-if="u.username !== webStatus.user" class="uwb-del" @click="handleDeleteUser(u.username)">删除</button>
          </li>
        </ul>
      </div>

      <div class="uwb-footer">
        <button class="uwb-btn uwb-danger" @click="handleLogout">退出当前账号 ({{ webStatus.user }})</button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue';
import {
  getWebModeStatus,
  downloadRemoteFileInBrowser,
  uploadFilesToRemoteInBrowser,
  logoutWebUser,
  listGatewayUsers,
  createGatewayUser,
  deleteGatewayUser,
  type WebModeStatus,
} from '../services/webSftpBridge';

const webStatus = ref<WebModeStatus>({ serverMode: false, user: '', role: 'user', homeDir: '' });
const panelOpen = ref(false);
const sessionId = ref('');
const remotePath = ref('/tmp');
const uploadProgress = ref(0);
const statusMsg = ref('');

const userList = ref<Array<{ username: string; role: string }>>([]);
const newUsername = ref('');
const newPassword = ref('');

onMounted(async () => {
  webStatus.value = await getWebModeStatus();
  if (webStatus.value.serverMode && webStatus.value.role === 'admin') {
    await refreshUsers();
  }
});

async function refreshUsers() {
  try {
    userList.value = await listGatewayUsers();
  } catch (e) {
    console.warn(e);
  }
}

function handleDownload() {
  if (!sessionId.value || !remotePath.value) {
    statusMsg.value = '请填写 Session ID 与远程文件路径';
    return;
  }
  downloadRemoteFileInBrowser(sessionId.value.trim(), remotePath.value.trim());
  statusMsg.value = '已触发浏览器原生流式下载';
}

async function handleUpload(directoryMode: boolean) {
  if (!sessionId.value || !remotePath.value) {
    statusMsg.value = '请填写 Session ID 与远程目标目录';
    return;
  }
  try {
    uploadProgress.value = 1;
    statusMsg.value = '正在流式上传...';
    const res = await uploadFilesToRemoteInBrowser({
      sessionId: sessionId.value.trim(),
      remoteDir: remotePath.value.trim(),
      directoryMode,
      onProgress: (p) => {
        uploadProgress.value = p.percent;
      },
    });
    uploadProgress.value = 0;
    statusMsg.value = `上传完成（共 ${res.uploadedFiles.length} 个文件）`;
  } catch (err: any) {
    uploadProgress.value = 0;
    statusMsg.value = err?.message || '上传失败';
  }
}

async function handleCreateUser() {
  if (!newUsername.value || !newPassword.value) return;
  try {
    await createGatewayUser(newUsername.value.trim(), newPassword.value, 'user');
    newUsername.value = '';
    newPassword.value = '';
    await refreshUsers();
    statusMsg.value = '用户创建成功，已分配独立隔离目录';
  } catch (err: any) {
    statusMsg.value = err?.message || '创建用户失败';
  }
}

async function handleDeleteUser(username: string) {
  try {
    await deleteGatewayUser(username);
    await refreshUsers();
  } catch (err: any) {
    statusMsg.value = err?.message || '删除失败';
  }
}

async function handleLogout() {
  await logoutWebUser();
}
</script>

<style scoped>
.uniterm-web-floating-bar {
  position: fixed;
  bottom: 12px;
  right: 16px;
  z-index: 99999;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
  font-size: 12px;
  color: #e5eaf3;
}
.uwb-pill {
  display: flex;
  align-items: center;
  gap: 8px;
  background: rgba(23, 31, 44, 0.92);
  border: 1px solid #3b82f6;
  padding: 6px 12px;
  border-radius: 999px;
  cursor: pointer;
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.45);
}
.uwb-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: #22c55e;
}
.uwb-divider { color: #475569; }
.uwb-drawer {
  position: absolute;
  bottom: 40px;
  right: 0;
  width: 420px;
  background: #171f2c;
  border: 1px solid #263246;
  border-radius: 12px;
  padding: 16px;
  box-shadow: 0 16px 40px rgba(0, 0, 0, 0.6);
}
.uwb-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 12px;
  padding-bottom: 8px;
  border-bottom: 1px solid #263246;
}
.uwb-close {
  background: transparent;
  border: none;
  color: #94a3b8;
  font-size: 18px;
  cursor: pointer;
}
.uwb-section {
  margin-bottom: 14px;
}
.uwb-title {
  font-weight: 600;
  color: #93c5fd;
  margin-bottom: 8px;
}
.uwb-row { margin-bottom: 8px; }
.uwb-flex { display: flex; gap: 6px; }
.uwb-input {
  width: 100%;
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid #263246;
  background: #0f141c;
  color: #e5eaf3;
  font-size: 12px;
}
.uwb-btn-group {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.uwb-btn {
  padding: 6px 10px;
  border-radius: 6px;
  border: none;
  background: #2563eb;
  color: #fff;
  cursor: pointer;
  font-size: 12px;
}
.uwb-btn:hover { background: #1d4ed8; }
.uwb-danger { background: #dc2626; width: 100%; }
.uwb-progress, .uwb-msg {
  margin-top: 6px;
  color: #38bdf8;
}
.uwb-user-list {
  list-style: none;
  padding: 0;
  margin: 8px 0 0 0;
  max-height: 110px;
  overflow-y: auto;
}
.uwb-user-list li {
  display: flex;
  justify-content: space-between;
  padding: 4px 0;
  border-bottom: 1px dashed #263246;
}
.uwb-del {
  background: transparent;
  border: 1px solid #ef4444;
  color: #ef4444;
  border-radius: 4px;
  cursor: pointer;
  font-size: 11px;
}
</style>
