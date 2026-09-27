<template>
  <AppLayout>
    <div class="mx-auto max-w-5xl space-y-6 p-4 sm:p-6">
      <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 class="text-xl font-semibold text-gray-900 dark:text-white">{{ t('oss.title') }}</h1>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">{{ t('oss.description') }}</p>
        </div>
        <div class="flex gap-2">
          <router-link to="/docs/oss" class="btn btn-secondary" data-testid="oss-docs-link">{{ t('oss.docsLink') }}</router-link>
          <button type="button" class="btn btn-primary" data-testid="oss-create" @click="openCreate">{{ t('oss.create') }}</button>
        </div>
      </div>

      <p v-if="message" class="text-sm text-green-600 dark:text-green-400" data-testid="oss-message">{{ message }}</p>
      <p v-if="error" class="text-sm text-red-600 dark:text-red-400" data-testid="oss-error">{{ error }}</p>

      <div v-if="loading" class="text-sm text-gray-500">...</div>
      <p v-else-if="items.length === 0" class="text-sm text-gray-500" data-testid="oss-empty">{{ t('oss.empty') }}</p>
      <div v-else class="overflow-x-auto rounded-xl border border-gray-200 dark:border-dark-700">
        <table class="min-w-full text-sm">
          <thead class="bg-gray-50 text-left text-gray-500 dark:bg-dark-800 dark:text-gray-400">
            <tr>
              <th class="px-4 py-3">{{ t('oss.provider') }}</th>
              <th class="px-4 py-3">{{ t('oss.bucket') }}</th>
              <th class="px-4 py-3">{{ t('oss.domain') }}</th>
              <th class="px-4 py-3">{{ t('oss.actions') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in items" :key="item.id" class="border-t border-gray-200 dark:border-dark-700" :data-testid="`oss-row-${item.id}`">
              <td class="px-4 py-3" data-testid="oss-provider">{{ t(`oss.providers.${item.provider}`) }}</td>
              <td class="px-4 py-3" data-testid="oss-bucket">{{ item.bucket_url || item.bucket }}</td>
              <td class="px-4 py-3" data-testid="oss-domain">{{ item.domain || '—' }}</td>
              <td class="px-4 py-3">
                <div class="flex flex-wrap gap-2">
                  <button type="button" class="btn btn-secondary btn-sm" :data-testid="`oss-check-${item.id}`" @click="check(item)">{{ t('oss.check') }}</button>
                  <button type="button" class="btn btn-secondary btn-sm" :data-testid="`oss-edit-${item.id}`" @click="openEdit(item)">{{ t('oss.edit') }}</button>
                  <button type="button" class="btn btn-secondary btn-sm" :data-testid="`oss-delete-${item.id}`" @click="remove(item)">{{ t('oss.delete') }}</button>
                  <button type="button" class="btn btn-secondary btn-sm" :data-testid="`oss-copy-${item.id}`" @click="copyId(item.id)">{{ t('oss.copyId') }}</button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <BaseDialog :show="modalOpen" :title="editingId ? t('oss.edit') : t('oss.create')" @close="closeModal">
      <form class="space-y-3" data-testid="oss-form" @submit.prevent="save">
        <label class="block text-sm">
          <span class="mb-1 block text-gray-600 dark:text-gray-300">{{ t('oss.provider') }}</span>
          <select v-model="form.provider" class="input" data-testid="oss-form-provider">
            <option v-for="provider in providers" :key="provider" :value="provider">{{ t(`oss.providers.${provider}`) }}</option>
          </select>
        </label>

        <template v-if="form.provider === 'aliyun'">
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.accessKeyId') }}</span><input v-model="form.access_key_id" class="input" data-testid="oss-access-key-id" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.accessKeySecret') }}</span><input v-model="form.access_key_secret" class="input" type="password" :placeholder="editingId ? t('oss.secretPlaceholder') : ''" data-testid="oss-secret" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.bucket') }}</span><input v-model="form.bucket" class="input" data-testid="oss-form-bucket" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.region') }}</span><input v-model="form.region" class="input" data-testid="oss-region" /></label>
        </template>

        <template v-else-if="form.provider === 'tencent'">
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.secretId') }}</span><input v-model="form.secret_id" class="input" data-testid="oss-secret-id" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.secretKey') }}</span><input v-model="form.secret_key" class="input" type="password" :placeholder="editingId ? t('oss.secretPlaceholder') : ''" data-testid="oss-secret" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.bucketUrl') }}</span><input v-model="form.bucket_url" class="input" data-testid="oss-bucket-url" /></label>
        </template>

        <template v-else-if="form.provider === 'qiniuyun'">
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.accessKey') }}</span><input v-model="form.access_key" class="input" data-testid="oss-access-key" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.secretKey') }}</span><input v-model="form.secret_key" class="input" type="password" :placeholder="editingId ? t('oss.secretPlaceholder') : ''" data-testid="oss-secret" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.bucket') }}</span><input v-model="form.bucket" class="input" data-testid="oss-form-bucket" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.region') }}</span><input v-model="form.region" class="input" data-testid="oss-region" /></label>
        </template>

        <template v-else-if="form.provider === 'cloudflare'">
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.accountId') }}</span><input v-model="form.account_id" class="input" data-testid="oss-account-id" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.accessKeyId') }}</span><input v-model="form.access_key_id" class="input" data-testid="oss-access-key-id" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.accessKeySecret') }}</span><input v-model="form.access_key_secret" class="input" type="password" :placeholder="editingId ? t('oss.secretPlaceholder') : ''" data-testid="oss-secret" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.bucketName') }}</span><input v-model="form.bucket_name" class="input" data-testid="oss-bucket-name" /></label>
        </template>

        <template v-else>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.accessKeyId') }}</span><input v-model="form.access_key_id" class="input" data-testid="oss-access-key-id" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.secretAccessKey') }}</span><input v-model="form.secret_access_key" class="input" type="password" :placeholder="editingId ? t('oss.secretPlaceholder') : ''" data-testid="oss-secret" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.bucket') }}</span><input v-model="form.bucket" class="input" data-testid="oss-form-bucket" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.region') }}</span><input v-model="form.region" class="input" data-testid="oss-region" /></label>
          <label class="block text-sm"><span class="mb-1 block">{{ t('oss.fields.endpoint') }}</span><input v-model="form.endpoint" class="input" data-testid="oss-endpoint" /></label>
          <label class="flex items-center gap-2 text-sm">
            <input v-model="form.force_path_style" type="checkbox" data-testid="oss-path-style" />
            {{ t('oss.fields.forcePathStyle') }}
          </label>
        </template>

        <label class="block text-sm">
          <span class="mb-1 block">{{ t('oss.fields.domain') }}</span>
          <input v-model="form.domain" class="input" data-testid="oss-form-domain" />
        </label>
        <p v-if="editingId && secretConfigured" class="text-xs text-gray-500">{{ t('oss.secretConfigured') }}</p>
      </form>
      <template #footer>
        <button type="button" class="btn btn-secondary" @click="closeModal">{{ t('oss.cancel') }}</button>
        <button type="button" class="btn btn-primary" data-testid="oss-save" :disabled="saving" @click="save">{{ t('oss.save') }}</button>
      </template>
    </BaseDialog>
  </AppLayout>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import {
  checkOSSRepository,
  createOSSRepository,
  deleteOSSRepository,
  listOSSRepositories,
  updateOSSRepository,
  type UserOSSInput,
  type UserOSSProvider,
  type UserOSSRepository,
} from '@/api/oss'

const { t } = useI18n()
const providers: UserOSSProvider[] = ['aliyun', 'tencent', 'qiniuyun', 'cloudflare', 's3']
const items = ref<UserOSSRepository[]>([])
const loading = ref(false)
const saving = ref(false)
const message = ref('')
const error = ref('')
const modalOpen = ref(false)
const editingId = ref<number | null>(null)
const secretConfigured = ref(false)

const emptyForm = (): UserOSSInput => ({
  provider: 'aliyun',
  access_key_id: '',
  access_key_secret: '',
  bucket: '',
  region: 'cn-hangzhou',
  domain: '',
  secret_id: '',
  secret_key: '',
  bucket_url: '',
  access_key: '',
  account_id: '',
  bucket_name: '',
  endpoint: '',
  secret_access_key: '',
  force_path_style: false,
})

const form = reactive<UserOSSInput>(emptyForm())

function resetForm(provider: UserOSSProvider = 'aliyun') {
  Object.assign(form, emptyForm(), { provider, region: provider === 'qiniuyun' ? 'cn-east-1' : provider === 's3' ? 'auto' : 'cn-hangzhou' })
}

async function load() {
  loading.value = true
  error.value = ''
  try {
    items.value = await listOSSRepositories()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  secretConfigured.value = false
  resetForm()
  modalOpen.value = true
}

function openEdit(item: UserOSSRepository) {
  editingId.value = item.id
  secretConfigured.value = item.secret_configured
  resetForm(item.provider)
  form.domain = item.domain
  form.region = item.region || form.region
  form.endpoint = item.endpoint || ''
  form.access_key_id = item.access_key_id || ''
  form.account_id = item.account_id || ''
  form.bucket_url = item.bucket_url || ''
  form.force_path_style = !!item.force_path_style
  if (item.provider === 'qiniuyun') form.access_key = item.access_key_id || ''
  if (item.provider === 'tencent') form.secret_id = item.access_key_id || ''
  if (item.provider === 'cloudflare') form.bucket_name = item.bucket
  else if (item.provider !== 'tencent') form.bucket = item.bucket
  modalOpen.value = true
}

function closeModal() {
  modalOpen.value = false
}

async function save() {
  saving.value = true
  error.value = ''
  message.value = ''
  try {
    if (editingId.value) {
      await updateOSSRepository(editingId.value, { ...form })
    } else {
      await createOSSRepository({ ...form })
    }
    modalOpen.value = false
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    saving.value = false
  }
}

async function check(item: UserOSSRepository) {
  error.value = ''
  message.value = ''
  try {
    const result = await checkOSSRepository(item.id)
    message.value = result.ok ? t('oss.checkOk') : result.message || t('oss.checkFailed')
    if (!result.ok) error.value = result.message
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function remove(item: UserOSSRepository) {
  if (!window.confirm(t('oss.confirmDelete'))) return
  error.value = ''
  try {
    await deleteOSSRepository(item.id)
    await load()
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  }
}

async function copyId(id: number) {
  const text = String(id)
  if (navigator.clipboard) {
    await navigator.clipboard.writeText(text)
  }
  message.value = `${t('oss.copied')}: ${text}`
}

onMounted(load)
</script>
