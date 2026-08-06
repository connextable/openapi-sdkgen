<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { useData } from "vitepress";
import CodeViewer from "./CodeViewer.vue";
import FileTree from "./FileTree.vue";
import { codeThemes, type CodeTheme } from "../playground/highlight";
import { readPlaygroundPreferences, writePlaygroundPreferences } from "../playground/preferences";
import { buildArtifactTree, findArtifact } from "../playground/tree";
import { generate, type GeneratedArtifact } from "../playground/wasm";

const maximumInputBytes = 64 * 1024 * 1024;
const codeThemeValues = codeThemes.map((theme) => theme.value);
const translations = {
  en: {
    heading: "SDK Playground",
    description: "Load an OpenAPI document and inspect generated source in your browser.",
    target: "Target",
    document: "OpenAPI document",
    dropFile: "Drop a JSON or YAML file",
    chooseFile: "or choose from your computer",
    loadFromURL: "or load from URL",
    urlLabel: "OpenAPI document URL",
    load: "Load",
    localNetwork: "Allow local network access",
    localNetworkHelp: "May show a browser permission prompt. The source server must still allow CORS.",
    privacy: "Files stay in this browser. URL loading requires CORS access.",
    generatedFiles: "Generated files",
    generatedCode: "Generated code",
    colorTheme: "Color theme",
    change: "Change",
    generating: "Generating SDK…",
    firstRun: "First run loads the generator into your browser.",
    stopped: "Generation stopped",
    warning: "Generated with warnings",
    emptyTitle: "Generated source appears here",
    emptyDescription: "Choose a file or paste a public OpenAPI URL to begin.",
    emptyDocument: "OpenAPI document is empty.",
    tooLarge: "OpenAPI document must be 64 MiB or smaller.",
    noFiles: "Generator returned no files.",
    invalidProtocol: "Use an HTTP or HTTPS URL.",
    invalidURL: "Enter a valid URL.",
    corsError: "Could not load this URL. The server may not allow browser CORS requests.",
    localNetworkError: "Could not access the local network. Allow browser access and make sure the source server permits CORS.",
  },
  ko: {
    heading: "SDK 플레이그라운드",
    description: "OpenAPI 문서를 불러와 브라우저에서 생성된 소스를 확인하세요.",
    target: "대상",
    document: "OpenAPI 문서",
    dropFile: "JSON 또는 YAML 파일을 놓으세요",
    chooseFile: "또는 컴퓨터에서 파일 선택",
    loadFromURL: "또는 URL에서 불러오기",
    urlLabel: "OpenAPI 문서 URL",
    load: "불러오기",
    localNetwork: "로컬 네트워크 접근 허용",
    localNetworkHelp: "브라우저 권한 요청이 표시될 수 있습니다. 원본 서버의 CORS 허용도 필요합니다.",
    privacy: "파일은 이 브라우저 안에서만 처리됩니다. URL은 CORS 접근을 허용해야 합니다.",
    generatedFiles: "생성된 파일",
    generatedCode: "생성된 코드",
    colorTheme: "색상 테마",
    change: "변경",
    generating: "SDK 생성 중…",
    firstRun: "처음 실행할 때 브라우저에 생성기를 불러옵니다.",
    stopped: "생성 중단",
    warning: "경고와 함께 생성됨",
    emptyTitle: "생성된 소스가 여기에 표시됩니다",
    emptyDescription: "파일을 선택하거나 공개 OpenAPI URL을 입력하세요.",
    emptyDocument: "OpenAPI 문서가 비어 있습니다.",
    tooLarge: "OpenAPI 문서는 64 MiB 이하여야 합니다.",
    noFiles: "생성된 파일이 없습니다.",
    invalidProtocol: "HTTP 또는 HTTPS URL을 사용하세요.",
    invalidURL: "올바른 URL을 입력하세요.",
    corsError: "URL을 불러올 수 없습니다. 서버에서 브라우저 CORS 요청을 허용하지 않을 수 있습니다.",
    localNetworkError: "로컬 네트워크에 접근할 수 없습니다. 브라우저 권한과 원본 서버의 CORS 설정을 확인하세요.",
  },
} as const;

const { lang } = useData();
const copy = computed(() => lang.value === "ko-KR" ? translations.ko : translations.en);
const target = ref("typescript");
const colorTheme = ref<CodeTheme>("github-dark");
const expandedPaths = ref<ReadonlySet<string>>(new Set());
const url = ref("");
const localNetworkAccess = ref(false);
const sourceLabel = ref("");
const artifacts = ref<GeneratedArtifact[]>([]);
const selectedPath = ref("");
const diagnostics = ref("");
const error = ref("");
const loading = ref(false);
const dragging = ref(false);
const fileInput = ref<HTMLInputElement>();

const selectedArtifact = computed(() => findArtifact(artifacts.value, selectedPath.value));
const selectedLineCount = computed(() => selectedArtifact.value?.content.split("\n").length ?? 0);
const lineCountLabel = computed(() => lang.value === "ko-KR" ? `${selectedLineCount.value}줄` : `${selectedLineCount.value} lines`);
const loaded = computed(() => artifacts.value.length > 0);
const tree = computed(() => buildArtifactTree(artifacts.value));
let preferencesReady = false;

onMounted(() => {
  const preferences = readPlaygroundPreferences(preferenceStorage(), codeThemeValues);
  if (preferences.codeTheme) colorTheme.value = preferences.codeTheme;
  expandedPaths.value = new Set(preferences.expandedPaths);
  preferencesReady = true;
});

watch([colorTheme, expandedPaths], () => {
  if (!preferencesReady) return;
  writePlaygroundPreferences(preferenceStorage(), {
    codeTheme: colorTheme.value,
    expandedPaths: expandedPaths.value,
  });
});

function preferenceStorage(): Storage | undefined {
  try {
    return window.localStorage;
  } catch {
    return undefined;
  }
}

type TargetAddressSpace = "local" | "loopback";
type LocalNetworkRequestInit = RequestInit & { targetAddressSpace?: TargetAddressSpace };

function targetAddressSpace(url: URL): TargetAddressSpace {
  const hostname = url.hostname.toLowerCase().replace(/^\[|\]$/g, "");
  if (
    hostname === "localhost" ||
    hostname.endsWith(".localhost") ||
    hostname === "::1" ||
    hostname.startsWith("127.")
  ) {
    return "loopback";
  }
  return "local";
}

function resetMessages() {
  error.value = "";
  diagnostics.value = "";
}

function validateSize(size: number) {
  if (size === 0) throw new Error(copy.value.emptyDocument);
  if (size > maximumInputBytes) throw new Error(copy.value.tooLarge);
}

async function runGeneration(contents: string, label: string) {
  resetMessages();
  artifacts.value = [];
  selectedPath.value = "";
  loading.value = true;
  try {
    const bytes = new TextEncoder().encode(contents).byteLength;
    validateSize(bytes);
    const result = await generate(contents, target.value);
    if (result.error) throw new Error(result.error);
    diagnostics.value = result.diagnostics ?? "";
    if (!result.artifacts?.length) {
      if (!diagnostics.value) throw new Error(copy.value.noFiles);
      return;
    }
    sourceLabel.value = label;
    artifacts.value = result.artifacts;
    selectedPath.value = result.artifacts[0].path;
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  } finally {
    loading.value = false;
  }
}

async function loadFile(file?: File) {
  if (!file) return;
  try {
    validateSize(file.size);
    await runGeneration(await file.text(), file.name);
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : String(cause);
  }
}

async function loadURL() {
  resetMessages();
  let parsed: URL;
  try {
    parsed = new URL(url.value);
  } catch {
    error.value = copy.value.invalidURL;
    return;
  }
  if (parsed.protocol !== "https:" && parsed.protocol !== "http:") {
    error.value = copy.value.invalidProtocol;
    return;
  }

  loading.value = true;
  try {
    const request: LocalNetworkRequestInit = { mode: "cors" };
    if (localNetworkAccess.value) request.targetAddressSpace = targetAddressSpace(parsed);
    const response = await fetch(parsed, request);
    if (!response.ok) {
      const message = lang.value === "ko-KR"
        ? `문서를 불러올 수 없습니다 (${response.status}).`
        : `Could not load the document (${response.status}).`;
      throw new Error(message);
    }
    const length = Number(response.headers.get("content-length"));
    if (Number.isFinite(length) && length > 0) validateSize(length);
    const label = `${parsed.hostname}${parsed.pathname}`;
    await runGeneration(await response.text(), label);
  } catch (cause) {
    error.value = cause instanceof TypeError
      ? localNetworkAccess.value ? copy.value.localNetworkError : copy.value.corsError
      : cause instanceof Error ? cause.message : String(cause);
    loading.value = false;
  }
}

function handleDrop(event: DragEvent) {
  dragging.value = false;
  void loadFile(event.dataTransfer?.files[0]);
}

function startOver() {
  sourceLabel.value = "";
  artifacts.value = [];
  selectedPath.value = "";
  diagnostics.value = "";
  error.value = "";
  url.value = "";
  if (fileInput.value) fileInput.value.value = "";
}

function toggleDirectory(path: string) {
  const next = new Set(expandedPaths.value);
  if (next.has(path)) next.delete(path);
  else next.add(path);
  expandedPaths.value = next;
}
</script>

<template>
  <main class="sdk-playground">
    <header class="playground-heading">
      <div>
        <p class="eyebrow">OPENAPI SDKGEN</p>
        <h1>{{ copy.heading }}</h1>
        <p>{{ copy.description }}</p>
      </div>
    </header>

    <section class="workbench" :class="{ 'has-files': loaded }">
      <aside class="source-panel">
        <div v-if="!loaded" class="source-controls">
          <label class="field-label" for="playground-target">{{ copy.target }}</label>
          <select id="playground-target" v-model="target">
            <option value="typescript">TypeScript</option>
          </select>

          <p class="field-label input-label">{{ copy.document }}</p>
          <button
            class="drop-zone"
            :class="{ dragging }"
            type="button"
            @click="fileInput?.click()"
            @dragenter.prevent="dragging = true"
            @dragover.prevent
            @dragleave.prevent="dragging = false"
            @drop.prevent="handleDrop"
          >
            <span class="upload-icon" aria-hidden="true">↑</span>
            <strong>{{ copy.dropFile }}</strong>
            <span>{{ copy.chooseFile }}</span>
          </button>
          <input
            ref="fileInput"
            class="visually-hidden"
            type="file"
            accept=".json,.yaml,.yml,application/json,application/yaml,text/yaml"
            @change="loadFile(($event.target as HTMLInputElement).files?.[0])"
          />

          <div class="separator"><span>{{ copy.loadFromURL }}</span></div>
          <form class="url-form" @submit.prevent="loadURL">
            <input v-model.trim="url" type="url" placeholder="https://example.com/openapi.yaml" :aria-label="copy.urlLabel" />
            <button type="submit" :disabled="loading || !url">{{ copy.load }}</button>
          </form>
          <label class="network-option">
            <input v-model="localNetworkAccess" type="checkbox" />
            <span>
              <strong>{{ copy.localNetwork }}</strong>
              <small>{{ copy.localNetworkHelp }}</small>
            </span>
          </label>
          <p class="privacy-note">{{ copy.privacy }}</p>
        </div>

        <template v-else>
          <div class="tree-header">
            <div>
              <span class="field-label">{{ copy.generatedFiles }}</span>
              <strong :title="sourceLabel">{{ sourceLabel }}</strong>
            </div>
            <button type="button" @click="startOver">{{ copy.change }}</button>
          </div>
          <div class="target-summary"><span>{{ copy.target }}</span><strong>TypeScript</strong></div>
          <nav class="tree-scroll" :aria-label="copy.generatedFiles">
            <FileTree
              :expanded-paths="expandedPaths"
              :nodes="tree"
              :selected-path="selectedPath"
              @select="selectedPath = $event"
              @toggle="toggleDirectory"
            />
          </nav>
        </template>
      </aside>

      <section class="code-panel">
        <div v-if="loading" class="empty-code loading-state" aria-live="polite">
          <span class="spinner" aria-hidden="true"></span>
          <strong>{{ copy.generating }}</strong>
          <span>{{ copy.firstRun }}</span>
        </div>
        <div v-else-if="error || (diagnostics && !loaded)" class="empty-code error-state" aria-live="polite">
          <span class="error-icon" aria-hidden="true">!</span>
          <strong>{{ copy.stopped }}</strong>
          <pre>{{ error || diagnostics }}</pre>
        </div>
        <template v-else-if="selectedArtifact">
          <header class="code-header">
            <div class="file-title"><span class="file-dot"></span><strong>{{ selectedArtifact.path }}</strong></div>
            <div class="code-meta">
              <span>{{ lineCountLabel }}</span>
              <label class="theme-picker">
                <span class="visually-hidden">{{ copy.colorTheme }}</span>
                <select v-model="colorTheme" :aria-label="copy.colorTheme">
                  <option v-for="themeOption in codeThemes" :key="themeOption.value" :value="themeOption.value">
                    {{ themeOption.label }}
                  </option>
                </select>
              </label>
            </div>
          </header>
          <div v-if="diagnostics" class="diagnostic-banner" :title="diagnostics">{{ copy.warning }}</div>
          <CodeViewer
            :content="selectedArtifact.content"
            :path="selectedArtifact.path"
            :theme="colorTheme"
            :aria-label="copy.generatedCode"
          />
        </template>
        <div v-else class="empty-code">
          <span class="code-placeholder" aria-hidden="true">&lt;/&gt;</span>
          <strong>{{ copy.emptyTitle }}</strong>
          <span>{{ copy.emptyDescription }}</span>
        </div>
      </section>
    </section>
  </main>
</template>

<style scoped>
.sdk-playground {
  width: min(1440px, calc(100vw - 48px));
  margin: 0 0 0 50%;
  padding-top: 48px;
  transform: translateX(-50%);
  color: var(--vp-c-text-1);
}

.playground-heading { display: flex; align-items: flex-end; justify-content: space-between; gap: 24px; margin-bottom: 28px; }
.playground-heading h1 { margin: 2px 0 6px; border: 0; font-size: clamp(30px, 4vw, 44px); line-height: 1.08; letter-spacing: -0.035em; }
.playground-heading p { margin: 0; color: var(--vp-c-text-2); }
.playground-heading .eyebrow { color: var(--vp-c-brand-1); font-size: 12px; font-weight: 750; letter-spacing: .14em; }
.workbench { display: grid; height: min(680px, calc(100dvh - 220px)); max-height: calc(100dvh - 96px); grid-template-columns: minmax(280px, 360px) minmax(0, 1fr); overflow: hidden; border: 1px solid var(--vp-c-divider); border-radius: 16px; background: var(--vp-c-bg); box-shadow: 0 22px 65px rgba(15, 23, 42, .08); }
.source-panel { display: flex; min-height: 0; overflow: hidden; flex-direction: column; border-right: 1px solid var(--vp-c-divider); background: color-mix(in srgb, var(--vp-c-bg-soft) 70%, var(--vp-c-bg)); }
.source-controls { overflow: auto; padding: 26px; }
.field-label { display: block; margin-bottom: 8px; color: var(--vp-c-text-2); font-size: 12px; font-weight: 750; letter-spacing: .06em; text-transform: uppercase; }
.input-label { margin-top: 24px; }
select, .url-form input { width: 100%; height: 42px; border: 1px solid var(--vp-c-divider); border-radius: 9px; color: var(--vp-c-text-1); background: var(--vp-c-bg); font: inherit; }
select { padding: 0 12px; }
.drop-zone { display: flex; width: 100%; min-height: 185px; align-items: center; justify-content: center; flex-direction: column; gap: 6px; border: 1.5px dashed var(--vp-c-divider); border-radius: 12px; color: var(--vp-c-text-2); background: var(--vp-c-bg); cursor: pointer; transition: .15s ease; }
.drop-zone:hover, .drop-zone.dragging { border-color: var(--vp-c-brand-1); background: color-mix(in srgb, var(--vp-c-brand-1) 6%, var(--vp-c-bg)); }
.drop-zone strong { color: var(--vp-c-text-1); font-size: 14px; }
.drop-zone span:last-child { font-size: 12px; }
.upload-icon { display: grid; width: 38px; height: 38px; margin-bottom: 5px; place-items: center; border-radius: 10px; color: var(--vp-c-brand-1); background: color-mix(in srgb, var(--vp-c-brand-1) 10%, transparent); font-size: 21px; }
.separator { display: flex; align-items: center; gap: 10px; margin: 22px 0 12px; color: var(--vp-c-text-3); font-size: 11px; text-transform: uppercase; }
.separator::before, .separator::after { content: ""; flex: 1; height: 1px; background: var(--vp-c-divider); }
.url-form { display: grid; grid-template-columns: minmax(0, 1fr) auto; gap: 8px; }
.url-form input { padding: 0 11px; min-width: 0; font-size: 13px; }
.url-form button, .tree-header button { border: 0; border-radius: 9px; font-weight: 700; cursor: pointer; }
.url-form button { padding: 0 15px; color: white; background: var(--vp-c-brand-1); }
.url-form button:disabled { opacity: .5; cursor: not-allowed; }
.network-option { display: flex; align-items: flex-start; gap: 9px; margin-top: 14px; color: var(--vp-c-text-2); cursor: pointer; }
.network-option input { width: 16px; height: 16px; margin: 2px 0 0; accent-color: var(--vp-c-brand-1); }
.network-option span { display: flex; min-width: 0; flex-direction: column; gap: 2px; }
.network-option strong { color: var(--vp-c-text-1); font-size: 12px; font-weight: 650; }
.network-option small { color: var(--vp-c-text-3); font-size: 10.5px; line-height: 1.45; }
.privacy-note { margin: 12px 0 0; color: var(--vp-c-text-3); font-size: 11px; line-height: 1.5; }
.visually-hidden { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0, 0, 0, 0); white-space: nowrap; }

.tree-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 22px 20px 16px; border-bottom: 1px solid var(--vp-c-divider); }
.tree-header .field-label { margin: 0 0 3px; }
.tree-header strong { display: block; max-width: 210px; overflow: hidden; font-size: 13px; text-overflow: ellipsis; white-space: nowrap; }
.tree-header button { padding: 7px 10px; color: var(--vp-c-brand-1); background: color-mix(in srgb, var(--vp-c-brand-1) 10%, transparent); font-size: 12px; }
.target-summary { display: flex; justify-content: space-between; padding: 12px 20px; border-bottom: 1px solid var(--vp-c-divider); color: var(--vp-c-text-3); font-size: 12px; }
.target-summary strong { color: var(--vp-c-text-2); }
.tree-scroll { min-height: 0; flex: 1; padding: 12px 10px; overflow-x: hidden; overflow-y: auto; }

.code-panel { display: flex; min-width: 0; min-height: 0; overflow: hidden; flex-direction: column; background: #0d1117; color: #d1d7e0; }
.code-header { display: flex; min-height: 52px; align-items: center; justify-content: space-between; padding: 0 18px; border-bottom: 1px solid #252b35; background: #111720; font-size: 12px; }
.file-title, .code-meta { display: flex; min-width: 0; align-items: center; gap: 9px; }
.file-title strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.code-meta { flex: 0 0 auto; color: #788391; }
.theme-picker select { width: auto; height: 30px; padding: 0 28px 0 9px; border-color: #303844; border-radius: 7px; color: #c9d1d9; background-color: #161d27; font-size: 11px; }
.file-dot { width: 8px; height: 8px; border-radius: 50%; background: #3178c6; box-shadow: 0 0 0 4px rgba(49, 120, 198, .13); }
.diagnostic-banner { padding: 8px 18px; color: #f8d477; border-bottom: 1px solid #4e4529; background: #272516; font-size: 12px; }
.empty-code { display: flex; min-height: 0; flex: 1; align-items: center; justify-content: center; flex-direction: column; gap: 8px; color: #778291; text-align: center; }
.empty-code strong { color: #c8d0da; font-size: 15px; }
.empty-code > span:last-child { max-width: 360px; font-size: 12px; }
.code-placeholder { display: grid; width: 52px; height: 52px; margin-bottom: 8px; place-items: center; border: 1px solid #29313c; border-radius: 14px; color: #5d6977; font: 700 15px ui-monospace, monospace; background: #111720; }
.spinner { width: 30px; height: 30px; margin-bottom: 10px; border: 3px solid #28313c; border-top-color: #2dd4bf; border-radius: 50%; animation: spin .8s linear infinite; }
.error-state { padding: 28px; }
.error-state pre { width: min(760px, 100%); max-height: 420px; overflow: auto; margin: 12px 0 0; padding: 16px; border: 1px solid #432b31; border-radius: 10px; color: #f1b8c0; background: #1c1418; font-size: 12px; line-height: 1.55; text-align: left; white-space: pre-wrap; }
.error-icon { display: grid; width: 36px; height: 36px; place-items: center; border-radius: 50%; color: #ff9daa; background: #352027; font-weight: 800; }
@keyframes spin { to { transform: rotate(360deg); } }

@media (max-width: 800px) {
  .sdk-playground { width: calc(100vw - 28px); padding-top: 28px; }
  .playground-heading { align-items: flex-start; flex-direction: column; }
  .workbench { height: auto; max-height: none; grid-template-columns: 1fr; }
  .source-panel { border-right: 0; border-bottom: 1px solid var(--vp-c-divider); }
  .tree-scroll { max-height: 230px; }
  .empty-code { min-height: 440px; }
  .code-panel { height: min(600px, calc(100dvh - 96px)); min-height: 440px; }
  .code-header { padding: 0 12px; }
  .code-meta { gap: 6px; }
}
</style>
