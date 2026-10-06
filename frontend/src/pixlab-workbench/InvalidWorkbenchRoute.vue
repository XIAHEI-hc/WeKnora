<template>
  <main class="workbench-route-error" role="alert">
    <strong>无法打开项目知识服务</strong>
    <p>当前地址不属于 PixLab 项目工作区。</p>
  </main>
</template>

<style scoped>
.workbench-route-error {
  display: grid;
  width: 100%;
  height: 100%;
  place-content: center;
  gap: 8px;
  color: var(--td-text-color-primary, #17202b);
  text-align: center;
  background: var(--td-bg-color-container, #fff);
}

.workbench-route-error p {
  margin: 0;
  color: var(--td-text-color-secondary, #6b7280);
  font-size: 13px;
}
</style>
