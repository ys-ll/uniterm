import { defineStore } from "pinia";
import { ref, computed } from "vue";
import {
  ListSkills,
  GetSkillBody,
  SetSkillEnabled,
  SetSkillLocked,
  DeleteSkill,
  CreateSkill,
  SaveSkill,
  OpenPathInExplorer,
} from "../../wailsjs/go/main/App";
import type { SkillMeta } from "../types/skill";
import { useAsyncAction } from "../composables/useAsyncAction";

export const useSkillStore = defineStore("skills", () => {
  const skills = ref<SkillMeta[]>([]);
  const loaded = ref(false);

  async function load() {
    if (loaded.value) return;
    const { run } = useAsyncAction(
      async () => {
        skills.value = (await ListSkills()) || [];
      },
      { errorMessage: "Failed to load skills" },
    );
    await run();
    skills.value = skills.value || [];
    loaded.value = true;
  }

  function reload() {
    loaded.value = false;
    return load();
  }

  const enabledSkills = computed(() => skills.value.filter((s) => s.enabled));

  async function toggleEnabled(name: string) {
    const s = skills.value.find((x) => x.name === name);
    if (!s) return;
    s.enabled = !s.enabled;
    const { run } = useAsyncAction(() => SetSkillEnabled(name, s.enabled), {
      errorMessage: "Failed to toggle skill enabled",
      onError: () => {
        s.enabled = !s.enabled;
      },
    });
    await run();
  }

  async function toggleLocked(name: string) {
    const s = skills.value.find((x) => x.name === name);
    if (!s) return;
    s.locked = !s.locked;
    const { run } = useAsyncAction(() => SetSkillLocked(name, s.locked), {
      errorMessage: "Failed to toggle skill lock",
      onError: () => {
        s.locked = !s.locked;
      },
    });
    await run();
  }

  async function remove(name: string) {
    const s = skills.value.find((x) => x.name === name);
    if (!s) return;
    if (s.locked) {
      return;
    }
    const { run } = useAsyncAction(
      async () => {
        await DeleteSkill(name);
        skills.value = skills.value.filter((x) => x.name !== name);
      },
      { errorMessage: "Failed to delete skill" },
    );
    await run();
  }

  async function create(name: string, description: string, body: string) {
    const { run } = useAsyncAction(
      async () => {
        await CreateSkill(name, description, body);
        await reload();
      },
      { errorMessage: "Failed to create skill", rethrow: true },
    );
    await run();
  }

  async function save(name: string, description: string, body: string) {
    const { run } = useAsyncAction(
      async () => {
        await SaveSkill(name, description, body);
        await reload();
      },
      { errorMessage: "Failed to save skill", rethrow: true },
    );
    await run();
  }

  // AI 侧保存：已存在则覆写（后端拒绝 locked），不存在则新建。
  async function saveByAgent(name: string, description: string, body: string) {
    await load();
    const exists = skills.value.some((s) => s.name === name);
    if (exists) {
      await SaveSkill(name, description, body);
    } else {
      await CreateSkill(name, description, body);
    }
    await reload();
  }

  async function getBody(name: string): Promise<string> {
    const { run } = useAsyncAction(() => GetSkillBody(name), {
      errorMessage: "Failed to get skill body",
    });
    const out = await run();
    return out ?? "";
  }

  async function openFolder(path: string): Promise<void> {
    if (!path) return;
    const { run } = useAsyncAction(() => OpenPathInExplorer(path), {
      errorMessage: "Failed to open skill folder",
    });
    await run();
  }

  return {
    skills,
    loaded,
    enabledSkills,
    load,
    reload,
    toggleEnabled,
    toggleLocked,
    remove,
    create,
    save,
    saveByAgent,
    getBody,
    openFolder,
  };
});
