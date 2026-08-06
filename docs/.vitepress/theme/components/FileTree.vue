<script setup lang="ts">
import OverflowMarquee from "./OverflowMarquee.vue";
import type { TreeNode } from "../playground/tree";

const props = withDefaults(defineProps<{
  depth?: number;
  expandedPaths: ReadonlySet<string>;
  nodes: TreeNode[];
  selectedPath?: string;
}>(), {
  depth: 0,
});

defineEmits<{
  select: [path: string];
  toggle: [path: string];
}>();

function isExpanded(path: string) {
  return props.expandedPaths.has(path);
}
</script>

<template>
  <ul class="file-tree" :class="{ 'file-tree-nested': depth > 0 }" :role="depth === 0 ? 'tree' : 'group'">
    <li
      v-for="node in nodes"
      :key="node.path"
      class="tree-item"
      role="treeitem"
      :aria-expanded="node.type === 'directory' ? isExpanded(node.path) : undefined"
      :aria-selected="node.type === 'file' ? node.path === selectedPath : undefined"
    >
      <button
        v-if="node.type === 'directory'"
        class="tree-row tree-directory"
        type="button"
        @click="$emit('toggle', node.path)"
      >
        <span class="tree-chevron" :class="{ expanded: isExpanded(node.path) }" aria-hidden="true">
          <svg viewBox="0 0 16 16" focusable="false">
            <path d="M5.75 3.5 10.25 8l-4.5 4.5" />
          </svg>
        </span>
        <OverflowMarquee :text="node.name" />
      </button>
      <button
        v-else
        class="tree-row tree-file"
        :class="{ selected: node.path === selectedPath }"
        type="button"
        @click="$emit('select', node.path)"
      >
        <span class="tree-file-icon" aria-hidden="true">TS</span>
        <OverflowMarquee :text="node.name" />
      </button>
      <FileTree
        v-if="node.children?.length && isExpanded(node.path)"
        :depth="depth + 1"
        :expanded-paths="expandedPaths"
        :nodes="node.children"
        :selected-path="selectedPath"
        @select="$emit('select', $event)"
        @toggle="$emit('toggle', $event)"
      />
    </li>
  </ul>
</template>

<style scoped>
.file-tree {
  width: 100%;
  min-width: 0;
  margin: 0;
  padding: 0;
  list-style: none;
}

.file-tree-nested {
  box-sizing: border-box;
  padding-left: 16px;
}

.tree-item {
  width: 100%;
  min-width: 0;
}

.tree-row {
  display: flex;
  box-sizing: border-box;
  width: 100%;
  min-width: 0;
  height: 32px;
  align-items: center;
  gap: 7px;
  padding: 0 8px;
  border: 0;
  border-radius: 6px;
  color: var(--vp-c-text-2);
  background: transparent;
  cursor: pointer;
  font: inherit;
  font-size: 13px;
  text-align: left;
}

.tree-row:hover {
  background: var(--vp-c-bg-soft);
}

.tree-directory {
  font-weight: 650;
}

.tree-file.selected {
  color: var(--vp-c-brand-1);
  background: color-mix(in srgb, var(--vp-c-brand-1) 11%, transparent);
  font-weight: 650;
}

.tree-chevron {
  display: grid;
  width: 18px;
  height: 18px;
  flex: 0 0 18px;
  place-items: center;
  color: var(--vp-c-text-3);
}

.tree-chevron svg {
  width: 14px;
  height: 14px;
  overflow: visible;
  fill: none;
  stroke: currentColor;
  stroke-linecap: round;
  stroke-linejoin: round;
  stroke-width: 1.6;
  transform: rotate(0deg);
  transform-origin: center;
  transition: transform 140ms ease;
}

.tree-chevron.expanded svg {
  transform: rotate(90deg);
}

.tree-file-icon {
  width: 18px;
  flex: 0 0 18px;
  color: #3178c6;
  font-size: 9px;
  font-weight: 850;
  letter-spacing: -.03em;
  text-align: center;
}
</style>
