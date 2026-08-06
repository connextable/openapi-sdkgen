<script setup lang="ts">
import { computed, ref, watch } from "vue";

const props = defineProps<{
  text: string;
}>();

const container = ref<HTMLElement>();
const content = ref<HTMLElement>();
const overflow = ref(0);

const marqueeStyle = computed(() => ({
  "--marquee-distance": `${-overflow.value}px`,
  "--marquee-duration": `${Math.max(1.6, overflow.value / 90 + 0.5).toFixed(2)}s`,
}));

function measure() {
  overflow.value = Math.max(0, (content.value?.scrollWidth ?? 0) - (container.value?.clientWidth ?? 0));
}

watch(
  () => props.text,
  () => {
    overflow.value = 0;
  },
);
</script>

<template>
  <span
    ref="container"
    class="tree-marquee"
    :class="{ 'is-overflowing': overflow > 0 }"
    :style="marqueeStyle"
    :title="text"
    @mouseenter="measure"
  >
    <span class="tree-marquee-ellipsis">{{ text }}</span>
    <span ref="content" class="tree-marquee-content" aria-hidden="true">{{ text }}</span>
  </span>
</template>

<style scoped>
.tree-marquee {
  display: block;
  min-width: 0;
  flex: 1;
  overflow: hidden;
  position: relative;
  white-space: nowrap;
}

.tree-marquee-ellipsis {
  display: block;
  width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.tree-marquee-content {
  position: absolute;
  top: 0;
  left: 0;
  display: block;
  min-width: max-content;
  transform: translateX(0);
  visibility: hidden;
  white-space: nowrap;
}

.tree-marquee.is-overflowing:hover .tree-marquee-ellipsis {
  visibility: hidden;
}

.tree-marquee.is-overflowing:hover .tree-marquee-content {
  animation: tree-name-marquee var(--marquee-duration) linear infinite;
  visibility: visible;
  will-change: transform;
}

@keyframes tree-name-marquee {
  0%, 8% { transform: translateX(0); }
  92%, 100% { transform: translateX(var(--marquee-distance)); }
}

@media (prefers-reduced-motion: reduce) {
  .tree-marquee.is-overflowing:hover .tree-marquee-content {
    animation: none;
    visibility: hidden;
  }

  .tree-marquee.is-overflowing:hover .tree-marquee-ellipsis {
    visibility: visible;
  }
}
</style>
