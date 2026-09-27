<template>
  <AppLayout>
    <div class="mx-auto max-w-3xl space-y-6 p-4 sm:p-6">
      <div>
        <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('oss.docs.title') }}</h1>
        <p class="mt-2 text-sm text-gray-600 dark:text-gray-300">{{ t('oss.docs.description') }}</p>
      </div>
      <section class="space-y-3">
        <h2 class="text-base font-medium text-gray-900 dark:text-white">{{ t('oss.docs.headersTitle') }}</h2>
        <p class="text-sm text-gray-600 dark:text-gray-300">{{ t('oss.docs.headersHint') }}</p>
        <pre class="overflow-x-auto rounded-xl bg-gray-900 p-4 text-sm text-gray-100" data-testid="oss-headers-example"><code>{{ headersExample }}</code></pre>
        <p class="text-sm text-gray-500">{{ t('oss.docs.absent') }}</p>
      </section>
      <router-link to="/oss" class="btn btn-secondary inline-flex">{{ t('oss.title') }}</router-link>
    </div>
  </AppLayout>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'

const { t } = useI18n()

const headersExample = `{
  "Content-Type": "application/json",
  "Authorization": "Bearer apikey",
  "oss-id": "id",
  "oss-path": "file/images"
}`
</script>
