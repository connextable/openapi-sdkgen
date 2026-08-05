import typescript from "@shikijs/langs/typescript";
import dracula from "@shikijs/themes/dracula";
import githubDark from "@shikijs/themes/github-dark";
import githubLight from "@shikijs/themes/github-light";
import nord from "@shikijs/themes/nord";
import vitesseDark from "@shikijs/themes/vitesse-dark";
import { createHighlighterCore } from "shiki/core";
import { createJavaScriptRegexEngine } from "shiki/engine/javascript";
import type { GrammarState } from "shiki/types";
import type { CodeTheme, HighlightedToken } from "./highlight";

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

const chunkLineCount = 250;
let activeRequestID = 0;

const highlighterPromise = createHighlighterCore({
  themes: [githubDark, githubLight, dracula, nord, vitesseDark],
  langs: [typescript],
  engine: createJavaScriptRegexEngine(),
});

self.addEventListener("message", async (event: MessageEvent<HighlightRequest>) => {
  const request = event.data;
  activeRequestID = request.id;
  try {
    const highlighter = await highlighterPromise;
    const sourceLines = request.source.split("\n");
    let grammarState: GrammarState | undefined;
    for (let startLine = 0; startLine < sourceLines.length; startLine += chunkLineCount) {
      if (activeRequestID !== request.id) return;
      const endLine = Math.min(sourceLines.length, startLine + chunkLineCount);
      const hasFollowingChunk = endLine < sourceLines.length;
      const chunkSource = sourceLines.slice(startLine, endLine).join("\n") + (hasFollowingChunk ? "\n" : "");
      const result = highlighter.codeToTokens(chunkSource, {
        lang: "typescript",
        theme: request.theme,
        grammarState,
      });
      grammarState = highlighter.getLastGrammarState(result.tokens);
      const response: HighlightResponse = {
        id: request.id,
        startLine,
        totalLines: sourceLines.length,
        tokens: result.tokens.slice(0, endLine - startLine).map((line) => line.map<HighlightedToken>((token) => ({
          content: token.content,
          color: token.color,
          bgColor: token.bgColor,
          fontStyle: token.fontStyle,
          htmlStyle: token.htmlStyle,
        }))),
        foreground: result.fg,
        background: result.bg,
        done: !hasFollowingChunk,
      };
      self.postMessage(response);
      if (hasFollowingChunk) await new Promise((resolve) => setTimeout(resolve, 0));
    }
  } catch (cause) {
    if (activeRequestID !== request.id) return;
    const response: HighlightResponse = {
      id: request.id,
      error: cause instanceof Error ? cause.message : String(cause),
    };
    self.postMessage(response);
  }
});
