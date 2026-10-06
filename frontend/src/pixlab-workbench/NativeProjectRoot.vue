<template>
  <RouterView v-slot="{ Component, route }">
    <component :is="Component" :key="String(route.params.projectCode || route.fullPath)" />
  </RouterView>
</template>
