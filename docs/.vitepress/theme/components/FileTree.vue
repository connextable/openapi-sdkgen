<script setup lang="ts">
export interface TreeNode {
  name: string;
  path: string;
  type: "directory" | "file";
  children?: TreeNode[];
}

defineProps<{
  nodes: TreeNode[];
  selectedPath?: string;
}>();

defineEmits<{
  select: [path: string];
}>();
</script>

<template>
  <ul class="file-tree" role="tree">
    <li v-for="node in nodes" :key="node.path" role="treeitem">
      <div v-if="node.type === 'directory'" class="tree-directory">
        <span class="tree-icon" aria-hidden="true">⌄</span>
        <span>{{ node.name }}</span>
      </div>
      <button
        v-else
        class="tree-file"
        :class="{ selected: node.path === selectedPath }"
        type="button"
        @click="$emit('select', node.path)"
      >
        <span class="tree-file-icon" aria-hidden="true">TS</span>
        <span>{{ node.name }}</span>
      </button>
      <FileTree
        v-if="node.children?.length"
        :nodes="node.children"
        :selected-path="selectedPath"
        @select="$emit('select', $event)"
      />
    </li>
  </ul>
</template>
