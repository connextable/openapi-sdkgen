export const codeThemes = [
  { value: "github-dark", label: "GitHub Dark" },
  { value: "github-light", label: "GitHub Light" },
  { value: "dracula", label: "Dracula" },
  { value: "nord", label: "Nord" },
  { value: "vitesse-dark", label: "Vitesse Dark" },
] as const;

export type CodeTheme = (typeof codeThemes)[number]["value"];

export const codeThemePalettes: Record<CodeTheme, { background: string; foreground: string }> = {
  "github-dark": { background: "#24292e", foreground: "#e1e4e8" },
  "github-light": { background: "#ffffff", foreground: "#24292e" },
  dracula: { background: "#282a36", foreground: "#f8f8f2" },
  nord: { background: "#2e3440", foreground: "#d8dee9" },
  "vitesse-dark": { background: "#121212", foreground: "#dbd7ca" },
};

export interface HighlightedToken {
  content: string;
  color?: string;
  bgColor?: string;
  fontStyle?: number;
  htmlStyle?: Record<string, string>;
}

export interface HighlightResult {
  tokens: HighlightedToken[][];
  theme: CodeTheme;
  foreground?: string;
  background?: string;
}

interface HighlightRequest {
  id: number;
  source: string;
  theme: CodeTheme;
}

interface HighlightResponse {
  id: number;
  startLine?: number;
  tokens?: HighlightedToken[][];
  totalLines?: number;
  foreground?: string;
  background?: string;
  done?: boolean;
  error?: string;
}

interface PendingHighlight {
  result: HighlightResult;
  onProgress?: (result: HighlightResult) => void;
  resolve: (result: HighlightResult) => void;
  reject: (error: Error) => void;
}

let highlightWorker: Worker | undefined;
let nextRequestID = 0;
const pendingHighlights = new Map<number, PendingHighlight>();

function rejectPending(error: Error) {
  pendingHighlights.forEach(({ reject }) => reject(error));
  pendingHighlights.clear();
}

function stopWorker(error: Error) {
  rejectPending(error);
  highlightWorker?.terminate();
  highlightWorker = undefined;
}

function worker(): Worker {
  if (highlightWorker) return highlightWorker;

  highlightWorker = new Worker(new URL("./highlight.worker.ts", import.meta.url), { type: "module" });
  highlightWorker.addEventListener("message", (event: MessageEvent<HighlightResponse>) => {
    const pending = pendingHighlights.get(event.data.id);
    if (!pending) return;
    if (event.data.error) {
      pendingHighlights.delete(event.data.id);
      pending.reject(new Error(event.data.error));
      return;
    }
    if (event.data.totalLines !== undefined) pending.result.tokens.length = event.data.totalLines;
    if (event.data.tokens && event.data.startLine !== undefined) {
      event.data.tokens.forEach((line, offset) => {
        pending.result.tokens[event.data.startLine! + offset] = line;
      });
    }
    pending.result.foreground = event.data.foreground ?? pending.result.foreground;
    pending.result.background = event.data.background ?? pending.result.background;
    pending.onProgress?.(pending.result);
    if (event.data.done) {
      pendingHighlights.delete(event.data.id);
      pending.resolve(pending.result);
    }
  });
  highlightWorker.addEventListener("error", () => {
    stopWorker(new Error("Syntax highlighting stopped unexpectedly."));
  });
  return highlightWorker;
}

export function cancelHighlight() {
  rejectPending(new Error("Syntax highlighting was superseded by a newer request."));
}

export function highlight(
  source: string,
  theme: CodeTheme,
  onProgress?: (result: HighlightResult) => void,
): Promise<HighlightResult> {
  cancelHighlight();
  return new Promise((resolve, reject) => {
    const id = nextRequestID;
    nextRequestID += 1;
    pendingHighlights.set(id, {
      result: { tokens: [], theme },
      onProgress,
      resolve,
      reject,
    });
    const request: HighlightRequest = { id, source, theme };
    worker().postMessage(request);
  });
}
