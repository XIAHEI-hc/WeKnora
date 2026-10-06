import { createApp } from 'vue'
import { createPinia } from 'pinia'
import TDesign from 'tdesign-vue-next'
import i18n from '@/i18n'
import router from './router'
import NativeProjectRoot from './NativeProjectRoot.vue'
import './workbench.css'
import 'tdesign-vue-next/dist/tdesign.css'
import '@/assets/theme/theme.css'
import '@/assets/theme/tdesign-overrides.less'
import '@/assets/dropdown-menu.less'
import '@/components/css/chat-hljs-dark.less'
import 'vue-virtual-scroller/dist/vue-virtual-scroller.css'
import { installTDesignIconOfflineGuard } from '@/utils/tdesign-icon-offline'

installTDesignIconOfflineGuard()
const app = createApp(NativeProjectRoot)
app.use(TDesign)
app.use(createPinia())
app.use(router)
app.use(i18n)
void router.isReady().then(() => app.mount('#pixlab-workbench'))
