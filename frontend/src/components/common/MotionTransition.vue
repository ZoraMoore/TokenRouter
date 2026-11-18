<template>
  <Transition v-bind="$attrs" @before-enter="restoreEnteringElement" @before-leave="isolateLeavingElement">
    <slot />
  </Transition>
</template>

<script setup lang="ts">
import { isolateLeavingElement, restoreEnteringElement } from '@/utils/leavingElement'

defineOptions({ inheritAttrs: false })
</script>
