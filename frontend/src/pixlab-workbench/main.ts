import { createApp } from 'vue'
import TDesign from 'tdesign-vue-next'
import i18n from '@/i18n'
import PixLabEmbedPage from './PixLabEmbedPage.vue'
import './workbench.css'
import 'tdesign-vue-next/dist/tdesign.css'
import '@/assets/theme/theme.css'
import '@/assets/theme/tdesign-overrides.less'
import '@/components/css/chat-hljs-dark.less'
import { installTDesignIconOfflineGuard } from '@/utils/tdesign-icon-offline'

installTDesignIconOfflineGuard()
const app = createApp(PixLabEmbedPage)
app.use(TDesign)
app.use(i18n)
app.mount('#pixlab-workbench')
