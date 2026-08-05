<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, shallowRef, watch } from "vue";
import {
  cancelHighlight,
  codeThemePalettes,
  highlight,
  type CodeTheme,
  type HighlightResult,
  type HighlightedToken,
} from "../playground/highlight";

const props = defineProps<{
  content: string;
  path: string;
  theme: CodeTheme;
  ariaLabel: string;
}>();

const lineHeight = 21;
const overscan = 100;
const viewport = ref<HTMLElement>();
const scrollTop = ref(0);
const viewportHeight = ref(0);
const highlighted = shallowRef<HighlightResult>();
const highlightCache = new Map<string, HighlightResult>();
let resizeObserver: ResizeObserver | undefined;
let highlightSequence = 0;
const maximumCachedResults = 3;

const lines = computed(() => props.content.split("\n"));
const totalHeight = computed(() => lines.value.length * lineHeight);
const startLine = computed(() => Math.max(0, Math.floor(scrollTop.value / lineHeight) - overscan));
const endLine = computed(() => Math.min(
  lines.value.length,
  Math.ceil((scrollTop.value + viewportHeight.value) / lineHeight) + overscan,
));
const visibleLines = computed(() => Array.from(
  { length: Math.max(0, endLine.value - startLine.value) },
  (_, offset) => {
    const index = startLine.value + offset;
    return {
      index,
      source: lines.value[index] ?? "",
      tokens: highlighted.value?.tokens[index],
    };
  },
));
const viewerStyle = computed(() => ({
  "--code-background": highlighted.value?.background ?? codeThemePalettes[props.theme].background,
  "--code-foreground": highlighted.value?.foreground ?? codeThemePalettes[props.theme].foreground,
}));

function tokenStyle(token: HighlightedToken): Record<string, string | undefined> {
  return {
    color: token.color,
    backgroundColor: token.bgColor,
    fontStyle: token.fontStyle && token.fontStyle & 1 ? "italic" : undefined,
    fontWeight: token.fontStyle && token.fontStyle & 2 ? "700" : undefined,
    textDecoration: token.fontStyle && token.fontStyle & 4 ? "underline" : undefined,
    ...token.htmlStyle,
  };
}

function updateViewportHeight() {
  viewportHeight.value = viewport.value?.clientHeight ?? 0;
}

function handleScroll(event: Event) {
  scrollTop.value = (event.currentTarget as HTMLElement).scrollTop;
}

function cacheKey(path: string, theme: CodeTheme) {
  return `${path}\0${theme}`;
}

function getCachedResult(key: string): HighlightResult | undefined {
  const result = highlightCache.get(key);
  if (!result) return undefined;
  highlightCache.delete(key);
  highlightCache.set(key, result);
  return result;
}

function setCachedResult(key: string, result: HighlightResult) {
  highlightCache.delete(key);
  highlightCache.set(key, result);
  if (highlightCache.size <= maximumCachedResults) return;
  const oldestKey = highlightCache.keys().next().value;
  if (oldestKey !== undefined) highlightCache.delete(oldestKey);
}

function waitForPaint(): Promise<void> {
  return new Promise((resolve) => requestAnimationFrame(() => resolve()));
}

async function requestHighlight(path: string, source: string, theme: CodeTheme, resetScroll: boolean) {
  const sequence = ++highlightSequence;
  cancelHighlight();
  const key = cacheKey(path, theme);
  const cached = getCachedResult(key);
  if (cached) {
    highlighted.value = cached;
    return;
  }
  highlighted.value = undefined;
  if (resetScroll) {
    scrollTop.value = 0;
    await nextTick();
    viewport.value?.scrollTo({ top: 0, left: 0 });
  }
  await nextTick();
  await waitForPaint();
  if (sequence !== highlightSequence) return;
  try {
    const result = await highlight(source, theme, (progress) => {
      if (sequence === highlightSequence) highlighted.value = { ...progress };
    });
    if (sequence === highlightSequence) {
      highlighted.value = result;
      setCachedResult(key, result);
    }
  } catch {
    if (sequence === highlightSequence) highlighted.value = undefined;
  }
}

watch(
  [() => props.path, () => props.content, () => props.theme],
  ([path, source, theme], [previousPath, previousSource]) => {
    void requestHighlight(path, source, theme, path !== previousPath || source !== previousSource);
  },
  { immediate: true },
);

onMounted(() => {
  updateViewportHeight();
  resizeObserver = new ResizeObserver(updateViewportHeight);
  if (viewport.value) resizeObserver.observe(viewport.value);
});

onBeforeUnmount(() => {
  highlightSequence += 1;
  cancelHighlight();
  resizeObserver?.disconnect();
});
</script>

<template>
  <div class="code-viewer" :style="viewerStyle">
    <div
      ref="viewport"
      class="code-viewport"
      role="region"
      tabindex="0"
      :aria-label="ariaLabel"
      @scroll="handleScroll"
    >
      <div class="virtual-code" :style="{ height: `${totalHeight}px` }">
        <div class="virtual-window" :style="{ transform: `translateY(${startLine * lineHeight}px)` }">
          <div v-for="line in visibleLines" :key="line.index" class="code-line">
            <span class="line-number">{{ line.index + 1 }}</span>
            <span class="line-source">
              <template v-if="line.tokens?.length">
                <span v-for="(token, tokenIndex) in line.tokens" :key="tokenIndex" :style="tokenStyle(token)">{{ token.content }}</span>
              </template>
              <template v-else>{{ line.source || " " }}</template>
            </span>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.code-viewer {
  position: relative;
  display: flex;
  min-width: 0;
  min-height: 0;
  flex: 1;
  color: var(--code-foreground);
  background: var(--code-background);
}

.code-viewport {
  width: 100%;
  min-width: 0;
  min-height: 0;
  overflow: auto;
  color: var(--code-foreground);
  background: var(--code-background);
  font: 12.5px/21px ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  overscroll-behavior: contain;
  scrollbar-color: color-mix(in srgb, var(--code-foreground) 28%, transparent) transparent;
}

.virtual-code {
  position: relative;
  width: max-content;
  min-width: 100%;
  padding: 14px 0 28px;
}

.virtual-window {
  position: absolute;
  top: 14px;
  left: 0;
  width: max-content;
  min-width: 100%;
  will-change: transform;
}

.code-line {
  display: flex;
  width: max-content;
  min-width: 100%;
  height: 21px;
  align-items: stretch;
}

.code-line:hover {
  background: color-mix(in srgb, var(--code-foreground) 5%, transparent);
}

.line-number {
  position: sticky;
  left: 0;
  z-index: 1;
  width: 58px;
  flex: 0 0 58px;
  padding-right: 16px;
  color: color-mix(in srgb, var(--code-foreground) 38%, transparent);
  background: var(--code-background);
  text-align: right;
  user-select: none;
}

.line-source {
  min-width: max-content;
  padding-right: 24px;
  white-space: pre;
}

</style>
