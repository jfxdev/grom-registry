import { QueryClient, VueQueryPlugin } from '@tanstack/vue-query'
import { createPinia } from 'pinia'
import { createApp } from 'vue'
import App from './App.vue'
import { router } from './router'
import './styles/main.css'

const app = createApp(App)
const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 20_000, retry: 1, refetchOnWindowFocus: false },
    mutations: { retry: 0 },
  },
})

app.use(createPinia())
app.use(VueQueryPlugin, { queryClient })
app.use(router)
app.mount('#app')
