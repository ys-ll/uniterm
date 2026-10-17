<template>
  <details class="remote-clipboard" @keydown.stop @keyup.stop @paste.stop>
    <summary>Clipboard</summary>
    <div class="remote-clipboard-body">
      <label :for="id">Browser text — copy remote text here, or paste text to send</label>
      <textarea :id="id" v-model="text" rows="4" />
      <div>
        <button type="button" @click="copy">Copy text</button>
        <button type="button" @click="paste">Paste from browser</button>
        <button type="button" :disabled="disabled" @click="$emit('send', text)">Send to remote</button>
      </div>
      <p role="status">{{ hint }}</p>
    </div>
  </details>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
const props = defineProps<{ id: string; received: string; disabled?: boolean }>()
defineEmits<{ send: [text: string] }>()
const text = ref(props.received)
const hint = ref('You can always select/copy or paste in this text area.')
watch(() => props.received, value => { text.value = value })
async function copy() {
  try {
    await navigator.clipboard.writeText(text.value)
    hint.value = 'Copied to browser clipboard.'
  } catch {
    const area = document.getElementById(props.id) as HTMLTextAreaElement
    area.focus(); area.select()
    hint.value = 'Use your browser Copy command (Ctrl/Cmd+C) on the selected text.'
  }
}
async function paste() {
  try {
    text.value = await navigator.clipboard.readText()
    hint.value = 'Review the text, then send it to the remote desktop.'
  } catch {
    document.getElementById(props.id)?.focus()
    hint.value = 'Paste into the text area with Ctrl/Cmd+V or the browser Paste command.'
  }
}
</script>

<style scoped>
.remote-clipboard { position: absolute; right: 1rem; top: 0.5rem; z-index: 20; color: var(--text-primary); background: var(--bg-elevated); padding: 0.35rem; border-radius: 0.25rem; }
.remote-clipboard summary { cursor: pointer; }
.remote-clipboard-body { width: min(26rem, 75vw); padding: 0.5rem; font-size: 0.8rem; }
textarea { display: block; width: 100%; box-sizing: border-box; margin: 0.5rem 0; color: inherit; background: var(--bg-base); }
button { margin-right: 0.5rem; cursor: pointer; }
</style>
