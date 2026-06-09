<template>
  <div class="blank-layout">
    <router-view />
  </div>
</template>

<style scoped lang="scss">
.blank-layout {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--ant-color-bg-layout, #f5f5f5);
}
</style>
