<template>
    <div class="space-y-6">
      <!-- S3 Storage Config -->
      <div class="card p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.backup.s3.title') }}
            </h3>
            <p v-if="s3Form.provider === 's3'" class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.backup.s3.descriptionPrefix') }}
              <button type="button" class="text-primary-600 underline hover:text-primary-700 dark:text-primary-400 dark:hover:text-primary-300" data-testid="backup-r2-guide" @click="showR2Guide = true">Cloudflare R2</button>
              {{ t('admin.backup.s3.descriptionSuffix') }}
            </p>
          </div>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <div class="md:col-span-2">
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400" for="backup-storage-provider">{{ t('admin.backup.s3.provider') }}</label>
            <select id="backup-storage-provider" data-testid="backup-storage-provider" class="input w-full" :value="s3Form.provider" @change="onBackupProviderChange">
              <option v-for="option in storageProviderOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
            </select>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400" for="backup-storage-endpoint">{{ t('admin.backup.s3.endpoint') }}</label>
            <input id="backup-storage-endpoint" v-model="s3Form.endpoint" data-testid="backup-storage-endpoint" class="input w-full" :placeholder="backupEndpointPlaceholder" />
            <p v-if="s3Form.provider === 'aliyun_oss'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="backup-endpoint-derive-hint">{{ t('admin.backup.s3.endpointDeriveHint') }}</p>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400" for="backup-storage-region">{{ t('admin.backup.s3.region') }}</label>
            <input id="backup-storage-region" v-model="s3Form.region" data-testid="backup-storage-region" class="input w-full" :placeholder="regionPlaceholder(s3Form.provider)" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400" for="backup-storage-bucket">{{ t('admin.backup.s3.bucket') }}</label>
            <input id="backup-storage-bucket" v-model="s3Form.bucket" data-testid="backup-storage-bucket" class="input w-full" :placeholder="bucketPlaceholder(s3Form.provider)" />
            <p v-if="s3Form.provider === 'tencent_cos'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="backup-bucket-hint">{{ t('admin.backup.s3.tencentBucketHint') }}</p>
            <p v-else-if="s3Form.provider === 'qiniu'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="backup-bucket-hint">{{ t('admin.backup.s3.qiniuBucketHint') }}</p>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.s3.prefix') }}</label>
            <input v-model="s3Form.prefix" data-testid="backup-storage-prefix" class="input w-full" placeholder="backups/" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.s3.accessKeyId') }}</label>
            <input v-model="s3Form.access_key_id" data-testid="backup-storage-access-key" class="input w-full" />
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.s3.secretAccessKey') }}</label>
            <input v-model="s3Form.secret_access_key" data-testid="backup-storage-secret" type="password" class="input w-full" :placeholder="s3SecretConfigured ? t('admin.backup.s3.secretConfigured') : ''" />
          </div>
          <label v-if="backupShowsForcePathStyle" class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-2" data-testid="backup-force-path-style">
            <input v-model="s3Form.force_path_style" type="checkbox" />
            <span>{{ t('admin.backup.s3.forcePathStyle') }}</span>
          </label>
        </div>
        <div class="mt-4 flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="testingS3" @click="testS3">
            {{ testingS3 ? t('common.loading') : t('admin.backup.s3.testConnection') }}
          </button>
          <button type="button" class="btn btn-primary btn-sm" data-testid="backup-s3-save" :disabled="savingS3" @click="saveS3Config">
            {{ savingS3 ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Async image object storage -->
      <div class="card p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.backup.imageStorage.title') }}
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.backup.imageStorage.description') }}
            </p>
          </div>
          <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
            <input v-model="imageStorageForm.enabled" type="checkbox" />
            <span>{{ t('admin.backup.imageStorage.enabled') }}</span>
          </label>
        </div>

        <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300">
          <input v-model="imageStorageForm.reuse_backup_s3" data-testid="image-reuse-backup" type="checkbox" />
          <span>{{ t('admin.backup.imageStorage.reuseBackupS3') }}</span>
        </label>

        <div v-if="!imageStorageForm.reuse_backup_s3" class="mt-3">
          <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400" for="image-storage-provider">{{ t('admin.backup.s3.provider') }}</label>
          <select id="image-storage-provider" data-testid="image-storage-provider" class="input w-full" :value="imageStorageForm.provider" @change="onImageProviderChange">
            <option v-for="option in storageProviderOptions" :key="option.value" :value="option.value">{{ option.label }}</option>
          </select>
        </div>

        <div class="mt-3 grid grid-cols-1 gap-3 md:grid-cols-2">
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.imageStorage.bucket') }}</label>
            <input v-model="imageStorageForm.bucket" data-testid="image-storage-bucket" class="input w-full" :placeholder="imageBucketPlaceholder" />
            <p v-if="!imageStorageForm.reuse_backup_s3 && imageStorageForm.provider === 'tencent_cos'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="image-bucket-hint">{{ t('admin.backup.s3.tencentBucketHint') }}</p>
            <p v-else-if="!imageStorageForm.reuse_backup_s3 && imageStorageForm.provider === 'qiniu'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="image-bucket-hint">{{ t('admin.backup.s3.qiniuBucketHint') }}</p>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.imageStorage.prefix') }}</label>
            <input v-model="imageStorageForm.prefix" data-testid="image-storage-prefix" class="input w-full" placeholder="images/" />
          </div>

          <template v-if="!imageStorageForm.reuse_backup_s3">
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400" for="image-storage-endpoint">{{ t('admin.backup.s3.endpoint') }}</label>
              <input id="image-storage-endpoint" v-model="imageStorageForm.endpoint" data-testid="image-storage-endpoint" class="input w-full" :placeholder="imageEndpointPlaceholder" />
              <p v-if="imageStorageForm.provider === 'aliyun_oss'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="image-endpoint-derive-hint">{{ t('admin.backup.s3.endpointDeriveHint') }}</p>
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400" for="image-storage-region">{{ t('admin.backup.s3.region') }}</label>
              <input id="image-storage-region" v-model="imageStorageForm.region" data-testid="image-storage-region" class="input w-full" :placeholder="regionPlaceholder(imageStorageForm.provider || 's3')" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.s3.accessKeyId') }}</label>
              <input v-model="imageStorageForm.access_key_id" data-testid="image-storage-access-key" class="input w-full" />
            </div>
            <div>
              <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.s3.secretAccessKey') }}</label>
              <input v-model="imageStorageForm.secret_access_key" data-testid="image-storage-secret" type="password" class="input w-full" :placeholder="imageStorageSecretConfigured ? t('admin.backup.s3.secretConfigured') : ''" />
            </div>
            <label v-if="imageShowsForcePathStyle" class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-2" data-testid="image-force-path-style">
              <input v-model="imageStorageForm.force_path_style" type="checkbox" />
              <span>{{ t('admin.backup.s3.forcePathStyle') }}</span>
            </label>
          </template>

          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.imageStorage.publicBaseUrl') }}</label>
            <input v-model="imageStorageForm.public_base_url" data-testid="image-public-base-url" class="input w-full" :placeholder="t('admin.backup.imageStorage.publicBaseUrlPlaceholder')" />
            <p v-if="imageEffectiveProvider === 'qiniu'" class="mt-1 text-xs text-gray-500 dark:text-gray-400" data-testid="image-qiniu-public-base-hint">{{ t('admin.backup.imageStorage.qiniuPublicBaseUrlHint') }}</p>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.imageStorage.presignExpiryHours') }}</label>
            <input v-model.number="imageStorageForm.presign_expiry_hours" data-testid="image-presign-hours" type="number" min="1" class="input w-full" />
          </div>
        </div>

        <div class="mt-4 flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary btn-sm" :disabled="testingImageStorage" @click="testImageStorage">
            {{ testingImageStorage ? t('common.loading') : t('admin.backup.s3.testConnection') }}
          </button>
          <button type="button" class="btn btn-primary btn-sm" data-testid="image-storage-save" :disabled="savingImageStorage" @click="saveImageStorageConfig">
            {{ savingImageStorage ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Schedule Config -->
      <div class="card p-6" data-testid="backup-schedule">
        <div class="mb-4">
          <h3 class="text-base font-semibold text-gray-900 dark:text-white">
            {{ t('admin.backup.schedule.title') }}
          </h3>
          <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
            {{ t('admin.backup.schedule.description') }}
          </p>
        </div>
        <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
          <label class="inline-flex items-center gap-2 text-sm text-gray-700 dark:text-gray-300 md:col-span-2">
            <input v-model="scheduleForm.enabled" type="checkbox" />
            <span>{{ t('admin.backup.schedule.enabled') }}</span>
          </label>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.schedule.cronExpr') }}</label>
            <input v-model="scheduleForm.cron_expr" class="input w-full" placeholder="0 2 * * *" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.schedule.cronHint') }}</p>
          </div>
          <h4 class="mt-2 text-sm font-medium text-gray-900 dark:text-white md:col-span-2">{{ t('admin.backup.schedule.ordinaryRetention') }}</h4>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.schedule.retainDays') }}</label>
            <input v-model.number="scheduleForm.retain_days" data-testid="backup-retain-days" type="number" min="0" step="1" class="input w-full" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.schedule.retainDaysHint') }}</p>
          </div>
          <div>
            <label class="mb-1 block text-xs font-medium text-gray-600 dark:text-gray-400">{{ t('admin.backup.schedule.retainCount') }}</label>
            <input v-model.number="scheduleForm.retain_count" data-testid="backup-retain-count" type="number" min="0" step="1" class="input w-full" />
            <p class="mt-1 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.schedule.retainCountHint') }}</p>
          </div>
        </div>
        <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">{{ t('admin.backup.schedule.ordinaryHint') }}</p>
        <BackupArchiveSettings v-model="archiveForm" />
        <p v-if="scheduleValidationError" class="mt-3 text-sm text-red-600 dark:text-red-400" role="alert">{{ scheduleValidationError }}</p>
        <div class="mt-4 rounded-lg bg-gray-50 p-3 text-sm text-gray-700 dark:bg-dark-800 dark:text-gray-300" aria-live="polite">
          <p class="font-medium">{{ t('admin.backup.schedule.preview') }}</p>
          <p>{{ retentionPreview }}</p>
        </div>
        <div class="mt-4">
          <button type="button" class="btn btn-primary btn-sm" :disabled="savingSchedule || !!scheduleValidationError" @click="saveSchedule">
            {{ savingSchedule ? t('common.loading') : t('common.save') }}
          </button>
        </div>
      </div>

      <!-- Backup Operations -->
      <div class="card p-6">
        <div class="mb-4 flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-base font-semibold text-gray-900 dark:text-white">
              {{ t('admin.backup.operations.title') }}
            </h3>
            <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.backup.operations.description') }}
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <div class="flex items-center gap-1">
              <label class="text-xs text-gray-600 dark:text-gray-400">{{ t('admin.backup.operations.expireDays') }}</label>
              <input v-model.number="manualExpireDays" type="number" min="0" class="input w-20 text-xs" />
            </div>
            <button type="button" class="btn btn-primary btn-sm" :disabled="creatingBackup" @click="createBackup">
              {{ creatingBackup ? t('admin.backup.operations.backing') : t('admin.backup.operations.createBackup') }}
            </button>
            <button type="button" class="btn btn-secondary btn-sm" :disabled="loadingBackups" @click="loadBackups">
              {{ loadingBackups ? t('common.loading') : t('common.refresh') }}
            </button>
          </div>
        </div>

        <div class="overflow-x-auto">
          <table class="w-full min-w-[800px] text-sm">
            <thead>
              <tr class="border-b border-gray-200 text-left text-xs uppercase tracking-wide text-gray-500 dark:border-dark-700 dark:text-gray-400">
                <th class="py-2 pr-4">ID</th>
                <th class="py-2 pr-4">{{ t('admin.backup.columns.status') }}</th>
                <th class="py-2 pr-4">{{ t('admin.backup.columns.fileName') }}</th>
                <th class="py-2 pr-4">{{ t('admin.backup.columns.size') }}</th>
                <th class="py-2 pr-4">{{ t('admin.backup.columns.parts') }}</th>
                <th class="py-2 pr-4">{{ t('admin.backup.columns.expiresAt') }}</th>
                <th class="py-2 pr-4">{{ t('admin.backup.columns.triggeredBy') }}</th>
                <th class="py-2 pr-4">{{ t('admin.backup.columns.startedAt') }}</th>
                <th class="py-2">{{ t('admin.backup.columns.actions') }}</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="record in backups" :key="record.id" class="border-b border-gray-100 align-top dark:border-dark-800">
                <td class="py-3 pr-4 font-mono text-xs">{{ record.id }}</td>
                <td class="py-3 pr-4">
                  <span
                    class="rounded px-2 py-0.5 text-xs"
                    :class="statusClass(record.status)"
                  >
                    {{ record.status === 'running' && record.progress
                      ? t(`admin.backup.progress.${record.progress}`)
                      : t(`admin.backup.status.${record.status}`) }}
                  </span>
                </td>
                <td class="py-3 pr-4 text-xs">
                  {{ record.file_name }}
                  <span v-if="record.monthly_archive" class="ml-1 inline-block rounded bg-primary-50 px-1.5 py-0.5 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300">{{ t('admin.backup.archive.badge') }}</span>
                  <div v-if="record.monthly_archive" class="mt-1 text-gray-500 dark:text-gray-400">{{ record.monthly_archive.dates.join(' / ') }}</div>
                </td>
                <td class="py-3 pr-4 text-xs">{{ formatSize(record.size_bytes) }}</td>
                <td class="py-3 pr-4 text-xs">{{ record.parts?.length || (record.status === 'running' ? '-' : 1) }}</td>
                <td class="py-3 pr-4 text-xs">
                  {{ record.monthly_archive
                    ? record.monthly_archive.retain_count === 0 ? t('admin.backup.archive.forever') : t('admin.backup.archive.retainLatest', { count: record.monthly_archive.retain_count })
                    : record.expires_at ? formatDate(record.expires_at) : t('admin.backup.neverExpire') }}
                </td>
                <td class="py-3 pr-4 text-xs">
                  {{ record.triggered_by === 'scheduled' ? t('admin.backup.trigger.scheduled') : t('admin.backup.trigger.manual') }}
                </td>
                <td class="py-3 pr-4 text-xs">{{ formatDate(record.started_at) }}</td>
                <td class="py-3 text-xs">
                  <div class="flex flex-wrap gap-1">
                    <button
                      v-if="record.status === 'completed'"
                      type="button"
                      class="btn btn-secondary btn-xs"
                      @click="downloadBackup(record.id)"
                    >
                      {{ t('admin.backup.actions.download') }}
                    </button>
                    <button
                      v-if="record.status === 'completed'"
                      type="button"
                      class="btn btn-secondary btn-xs"
                      :disabled="restoringId === record.id"
                      @click="restoreBackup(record.id)"
                    >
                      {{ restoringId === record.id ? t('common.loading') : t('admin.backup.actions.restore') }}
                    </button>
                    <button
                      v-if="record.status !== 'running'"
                      type="button"
                      class="btn btn-danger btn-xs"
                      @click="removeBackup(record.id)"
                    >
                      {{ t('common.delete') }}
                    </button>
                  </div>
                </td>
              </tr>
              <tr v-if="backups.length === 0">
                <td colspan="9" class="py-6 text-center text-sm text-gray-500 dark:text-gray-400">
                  {{ t('admin.backup.empty') }}
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <!-- Cloudflare R2 Setup Guide Modal -->
    <teleport to="body">
      <transition name="modal">
        <div v-if="showR2Guide" class="fixed inset-0 z-50 flex items-center justify-center p-4" @mousedown.self="showR2Guide = false">
          <div class="fixed inset-0 bg-black/50" @click="showR2Guide = false"></div>
          <div class="relative max-h-[85vh] w-full max-w-2xl overflow-y-auto rounded-xl bg-white p-6 shadow-2xl dark:bg-dark-800">
            <button type="button" class="absolute right-4 top-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200" @click="showR2Guide = false">
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>

            <h2 class="mb-4 text-lg font-bold text-gray-900 dark:text-white">{{ t('admin.backup.r2Guide.title') }}</h2>
            <p class="mb-4 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.backup.r2Guide.intro') }}</p>

            <!-- Step 1 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">1</span>
                {{ t('admin.backup.r2Guide.step1.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.backup.r2Guide.step1.line1') }}</li>
                <li>{{ t('admin.backup.r2Guide.step1.line2') }}</li>
                <li>{{ t('admin.backup.r2Guide.step1.line3') }}</li>
              </ol>
            </div>

            <!-- Step 2 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">2</span>
                {{ t('admin.backup.r2Guide.step2.title') }}
              </h3>
              <ol class="ml-8 list-decimal space-y-1 text-sm text-gray-600 dark:text-gray-300">
                <li>{{ t('admin.backup.r2Guide.step2.line1') }}</li>
                <li>{{ t('admin.backup.r2Guide.step2.line2') }}</li>
                <li>{{ t('admin.backup.r2Guide.step2.line3') }}</li>
                <li>{{ t('admin.backup.r2Guide.step2.line4') }}</li>
              </ol>
              <div class="mt-2 rounded-lg bg-amber-50 p-3 text-xs text-amber-700 dark:bg-amber-900/20 dark:text-amber-300">
                {{ t('admin.backup.r2Guide.step2.warning') }}
              </div>
            </div>

            <!-- Step 3 -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">3</span>
                {{ t('admin.backup.r2Guide.step3.title') }}
              </h3>
              <p class="ml-8 text-sm text-gray-600 dark:text-gray-300">{{ t('admin.backup.r2Guide.step3.desc') }}</p>
              <code class="ml-8 mt-1 block rounded bg-gray-100 px-3 py-2 text-xs text-gray-800 dark:bg-dark-700 dark:text-gray-200">https://&lt;{{ t('admin.backup.r2Guide.step3.accountId') }}&gt;.r2.cloudflarestorage.com</code>
            </div>

            <!-- Step 4: Fill form -->
            <div class="mb-5">
              <h3 class="mb-2 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-white">
                <span class="flex h-6 w-6 items-center justify-center rounded-full bg-primary-100 text-xs font-bold text-primary-700 dark:bg-primary-900/40 dark:text-primary-300">4</span>
                {{ t('admin.backup.r2Guide.step4.title') }}
              </h3>
              <div class="ml-8 overflow-hidden rounded-lg border border-gray-200 dark:border-dark-600">
                <table class="w-full text-sm">
                  <tbody>
                    <tr v-for="(row, i) in r2ConfigRows" :key="i" class="border-b border-gray-100 dark:border-dark-700 last:border-0">
                      <td class="whitespace-nowrap bg-gray-50 px-3 py-2 font-medium text-gray-700 dark:bg-dark-700 dark:text-gray-300">{{ row.field }}</td>
                      <td class="px-3 py-2 text-gray-600 dark:text-gray-400"><code class="text-xs">{{ row.value }}</code></td>
                    </tr>
                  </tbody>
                </table>
              </div>
            </div>

            <!-- Free tier note -->
            <div class="rounded-lg bg-green-50 p-3 text-xs text-green-700 dark:bg-green-900/20 dark:text-green-300">
              {{ t('admin.backup.r2Guide.freeTier') }}
            </div>

            <div class="mt-4 text-right">
              <button type="button" class="btn btn-primary btn-sm" @click="showR2Guide = false">{{ t('common.close') }}</button>
            </div>
          </div>
        </div>
      </transition>
    </teleport>
    <!-- 分卷下载链接 -->
    <teleport to="body">
      <transition name="modal">
        <div
          v-if="downloadPartsModalOpen"
          class="fixed inset-0 z-50 flex items-center justify-center p-4"
          @mousedown.self="closeDownloadParts"
        >
          <div class="fixed inset-0 bg-black/50" @click="closeDownloadParts"></div>
          <div class="relative max-h-[85vh] w-full max-w-lg overflow-y-auto rounded-xl bg-white p-6 shadow-2xl dark:bg-dark-800">
            <button
              type="button"
              class="absolute right-4 top-4 text-gray-400 hover:text-gray-600 dark:hover:text-gray-200"
              :aria-label="t('common.close')"
              @click="closeDownloadParts"
            >
              <svg class="h-5 w-5" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2"><path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" /></svg>
            </button>
            <h2 class="mb-1 text-lg font-bold text-gray-900 dark:text-white">{{ t('admin.backup.actions.downloadParts') }}</h2>
            <p class="mb-4 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.backup.actions.downloadPartsHint') }}</p>
            <div class="space-y-2">
              <div
                v-for="part in downloadParts"
                :key="part.index"
                class="flex items-center justify-between gap-3 rounded-lg border border-gray-200 px-3 py-2 dark:border-dark-600"
              >
                <span class="text-sm text-gray-700 dark:text-gray-300">
                  {{ t('admin.backup.actions.partLabel', { index: part.index }) }}
                  <span class="ml-2 text-xs text-gray-500 dark:text-gray-400">{{ formatSize(part.size_bytes) }}</span>
                </span>
                <a :href="part.url" class="btn btn-secondary btn-xs" rel="noopener">
                  {{ t('admin.backup.actions.download') }}
                </a>
              </div>
            </div>
            <div class="mt-4 text-right">
              <button type="button" class="btn btn-primary btn-sm" @click="closeDownloadParts">{{ t('common.close') }}</button>
            </div>
          </div>
        </div>
      </transition>
    </teleport>
    <TotpStepUpDialog :controller="backupStepUp" />
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api'
import { useAppStore } from '@/stores'
import type {
  BackupS3Config,
  BackupScheduleConfig,
  BackupMonthlyArchiveConfig,
  BackupRecord,
  BackupDownloadPart,
  ImageStorageConfig,
  StorageEndpointResolved,
  StorageProvider,
} from '@/api/admin/backup'
import { useStepUp, isStepUpBlocked, isStepUpCancelled, stepUpBlockReason } from '@/composables/useStepUp'
import TotpStepUpDialog from '@/components/auth/TotpStepUpDialog.vue'
import BackupArchiveSettings from '@/components/admin/BackupArchiveSettings.vue'

const { t } = useI18n()
const appStore = useAppStore()
const backupStepUp = useStepUp()

// 敏感操作被 2FA 门控拦截时的统一提示。
function reportStepUpBlocked(error: unknown): boolean {
  if (!isStepUpBlocked(error)) return false
  appStore.showError(
    stepUpBlockReason(error) === 'STEP_UP_ADMIN_API_KEY_FORBIDDEN'
      ? t('stepUp.adminApiKeyForbidden')
      : t('stepUp.notEnabled')
  )
  return true
}

const STORAGE_PROVIDERS: StorageProvider[] = ['s3', 'aliyun_oss', 'tencent_cos', 'qiniu']

const REGION_PLACEHOLDERS: Record<StorageProvider, string> = {
  s3: 'auto',
  aliyun_oss: 'cn-hangzhou',
  tencent_cos: 'ap-guangzhou',
  qiniu: 'cn-east-1',
}

const ENDPOINT_PLACEHOLDERS: Record<StorageProvider, string> = {
  s3: 'https://<account_id>.r2.cloudflarestorage.com',
  aliyun_oss: 'https://s3.oss-cn-hangzhou.aliyuncs.com',
  tencent_cos: 'https://cos.ap-guangzhou.myqcloud.com',
  qiniu: 'https://s3.cn-east-1.qiniucs.com',
}

interface ResolvedContext {
  provider: StorageProvider
  region: string
  bucket: string
}

function normalizeStorageProvider(value: string | undefined | null): StorageProvider {
  const provider = (value || '').trim()
  return STORAGE_PROVIDERS.includes(provider as StorageProvider) ? provider as StorageProvider : 's3'
}

function displayRegion(provider: StorageProvider, region: string | undefined | null): string {
  if (provider === 's3') return region?.trim() ? region : 'auto'
  return region ?? ''
}

function applyStorageProviderChange(form: { provider: StorageProvider; region: string; endpoint: string }, next: StorageProvider) {
  if (form.provider === next) return
  form.provider = next
  if (next === 's3') {
    if (!form.region.trim()) form.region = 'auto'
    return
  }
  if (!form.region.trim() || form.region.trim() === 'auto') form.region = ''
  form.endpoint = ''
}

function regionPlaceholder(provider: StorageProvider | undefined): string {
  return REGION_PLACEHOLDERS[normalizeStorageProvider(provider)]
}

function bucketPlaceholder(provider: StorageProvider | undefined): string {
  return normalizeStorageProvider(provider) === 'tencent_cos' ? 'example-1250000000' : ''
}

function endpointPlaceholder(
  provider: StorageProvider,
  endpoint: string,
  region: string,
  bucket: string,
  resolved: StorageEndpointResolved | null,
  context: ResolvedContext | null,
): string {
  const derived = resolved?.endpoint?.trim()
  if (
    !endpoint.trim() &&
    derived &&
    context &&
    context.provider === provider &&
    context.region.trim() === region.trim() &&
    context.bucket.trim() === bucket.trim()
  ) {
    return derived
  }
  return ENDPOINT_PLACEHOLDERS[provider]
}

function rememberResolved(
  resolvedRef: { value: StorageEndpointResolved | null },
  contextRef: { value: ResolvedContext | null },
  provider: StorageProvider,
  region: string,
  bucket: string,
  resolved: StorageEndpointResolved | undefined,
) {
  if (resolved?.endpoint?.trim()) {
    resolvedRef.value = resolved
    contextRef.value = { provider, region, bucket }
    return
  }
  resolvedRef.value = null
  contextRef.value = null
}

const storageProviderOptions = computed(() => [
  { value: 's3' as const, label: t('admin.backup.s3.providers.s3') },
  { value: 'aliyun_oss' as const, label: t('admin.backup.s3.providers.aliyun_oss') },
  { value: 'tencent_cos' as const, label: t('admin.backup.s3.providers.tencent_cos') },
  { value: 'qiniu' as const, label: t('admin.backup.s3.providers.qiniu') },
])

// Backup object storage. Empty endpoint stays empty; the derived URL is only a placeholder.
const s3Form = ref<BackupS3Config>({
  provider: 's3',
  endpoint: '',
  region: 'auto',
  bucket: '',
  access_key_id: '',
  secret_access_key: '',
  prefix: 'backups/',
  force_path_style: false,
})
const s3Resolved = ref<StorageEndpointResolved | null>(null)
const s3ResolvedContext = ref<ResolvedContext | null>(null)
const s3SecretConfigured = ref(false)
const savingS3 = ref(false)
const testingS3 = ref(false)
const backupEndpointPlaceholder = computed(() => endpointPlaceholder(
  s3Form.value.provider || 's3',
  s3Form.value.endpoint,
  s3Form.value.region,
  s3Form.value.bucket,
  s3Resolved.value,
  s3ResolvedContext.value,
))
const backupShowsForcePathStyle = computed(() =>
  (s3Form.value.provider || 's3') === 's3' || s3Form.value.endpoint.trim() !== '',
)

// Async image object storage. Reuse borrows the backup provider and credentials.
const imageStorageForm = ref<ImageStorageConfig>({
  enabled: false,
  reuse_backup_s3: true,
  bucket: '',
  prefix: 'images/',
  public_base_url: '',
  presign_expiry_hours: 24,
  max_download_bytes: 33554432,
  provider: 's3',
  endpoint: '',
  region: 'auto',
  access_key_id: '',
  secret_access_key: '',
  force_path_style: false,
})
const imageResolved = ref<StorageEndpointResolved | null>(null)
const imageResolvedContext = ref<ResolvedContext | null>(null)
const imageStorageSecretConfigured = ref(false)
const savingImageStorage = ref(false)
const testingImageStorage = ref(false)
const imageEndpointPlaceholder = computed(() => endpointPlaceholder(
  imageStorageForm.value.provider || 's3',
  imageStorageForm.value.endpoint,
  imageStorageForm.value.region,
  imageStorageForm.value.bucket,
  imageResolved.value,
  imageResolvedContext.value,
))
const imageShowsForcePathStyle = computed(() =>
  (imageStorageForm.value.provider || 's3') === 's3' || imageStorageForm.value.endpoint.trim() !== '',
)
const imageBucketPlaceholder = computed(() => {
  if (imageStorageForm.value.reuse_backup_s3) return t('admin.backup.imageStorage.bucketInherited')
  return bucketPlaceholder(imageStorageForm.value.provider || 's3')
})
const imageEffectiveProvider = computed(() =>
  imageStorageForm.value.reuse_backup_s3
    ? (s3Form.value.provider || 's3')
    : (imageStorageForm.value.provider || 's3'),
)

function onBackupProviderChange(event: Event) {
  const next = normalizeStorageProvider((event.target as HTMLSelectElement).value)
  if (next === s3Form.value.provider) return
  applyStorageProviderChange(s3Form.value as { provider: StorageProvider; region: string; endpoint: string }, next)
  s3Resolved.value = null
  s3ResolvedContext.value = null
}

function onImageProviderChange(event: Event) {
  const next = normalizeStorageProvider((event.target as HTMLSelectElement).value)
  if (next === imageStorageForm.value.provider) return
  applyStorageProviderChange(imageStorageForm.value as { provider: StorageProvider; region: string; endpoint: string }, next)
  imageResolved.value = null
  imageResolvedContext.value = null
}

function backupSubmitPayload(): BackupS3Config {
  const form = s3Form.value
  return {
    provider: form.provider || 's3',
    endpoint: form.endpoint,
    region: form.region,
    bucket: form.bucket,
    access_key_id: form.access_key_id,
    secret_access_key: form.secret_access_key ?? '',
    prefix: form.prefix,
    force_path_style: form.force_path_style,
  }
}

function imageSubmitPayload(): ImageStorageConfig {
  const form = imageStorageForm.value
  return {
    enabled: form.enabled,
    reuse_backup_s3: form.reuse_backup_s3,
    bucket: form.bucket,
    prefix: form.prefix,
    public_base_url: form.public_base_url,
    presign_expiry_hours: form.presign_expiry_hours,
    max_download_bytes: form.max_download_bytes,
    provider: form.provider || 's3',
    endpoint: form.endpoint,
    region: form.region,
    access_key_id: form.access_key_id,
    secret_access_key: form.secret_access_key ?? '',
    force_path_style: form.force_path_style,
  }
}

// Schedule config
const scheduleForm = ref<BackupScheduleConfig>({
  enabled: false,
  cron_expr: '0 2 * * *',
  retain_days: 14,
  retain_count: 10,
})
const savingSchedule = ref(false)
const archiveForm = ref<BackupMonthlyArchiveConfig>({ enabled: false, days: [1], include_month_end: false, retain_count: 0 })
// Disabling hides the archive parameters, so a save while disabled keeps the
// persisted parameters and only turns the rule off.
const savedArchive = ref<BackupMonthlyArchiveConfig>(cloneArchive(archiveForm.value))
const archivePayload = computed<BackupMonthlyArchiveConfig>(() =>
  archiveForm.value.enabled ? cloneArchive(archiveForm.value) : { ...cloneArchive(savedArchive.value), enabled: false },
)
function cloneArchive(config: BackupMonthlyArchiveConfig): BackupMonthlyArchiveConfig {
  return { ...config, days: [...config.days] }
}
const scheduleValidationError = computed(() => {
  const counts = [scheduleForm.value.retain_days, scheduleForm.value.retain_count]
  if (archiveForm.value.enabled) counts.push(archiveForm.value.retain_count)
  if (!counts.every(value => Number.isSafeInteger(value) && value >= 0)) {
    return t('admin.backup.archive.invalidRetention')
  }
  if (archiveForm.value.enabled && !archiveForm.value.days.length && !archiveForm.value.include_month_end) {
    return t('admin.backup.archive.selectDates')
  }
  return ''
})
const retentionPreview = computed(() => {
  if (scheduleValidationError.value) return scheduleValidationError.value
  const { retain_days: days, retain_count: count } = scheduleForm.value
  const ordinary = days > 0 && count > 0
    ? t('admin.backup.schedule.previewBoth', { days, count })
    : days > 0 ? t('admin.backup.schedule.previewDays', { days })
      : count > 0 ? t('admin.backup.schedule.previewCount', { count }) : t('admin.backup.schedule.previewUnlimited')
  if (!archiveForm.value.enabled) return ordinary
  const dates = [
    ...[...archiveForm.value.days].sort((a, b) => a - b).map(day => t('admin.backup.archive.day', { day })),
    ...(archiveForm.value.include_month_end ? [t('admin.backup.archive.monthEnd')] : []),
  ].join(', ')
  const retention = archiveForm.value.retain_count === 0 ? t('admin.backup.archive.forever') : t('admin.backup.archive.retainLatest', { count: archiveForm.value.retain_count })
  return `${ordinary} ${t('admin.backup.archive.preview', { dates, retention })}`
})

// Backups
const backups = ref<BackupRecord[]>([])
const loadingBackups = ref(false)
const creatingBackup = ref(false)
const restoringId = ref('')
const manualExpireDays = ref(14)
const downloadParts = ref<BackupDownloadPart[]>([])
const downloadPartsModalOpen = ref(false)

// Polling
const pollingTimer = ref<ReturnType<typeof setInterval> | null>(null)
const restoringPollingTimer = ref<ReturnType<typeof setInterval> | null>(null)
const MAX_POLL_COUNT = 900

function updateRecordInList(updated: BackupRecord) {
  const idx = backups.value.findIndex(r => r.id === updated.id)
  if (idx >= 0) {
    backups.value[idx] = updated
  }
}

function startPolling(backupId: string) {
  stopPolling()
  let count = 0
  pollingTimer.value = setInterval(async () => {
    if (count++ >= MAX_POLL_COUNT) {
      stopPolling()
      creatingBackup.value = false
      appStore.showWarning(t('admin.backup.operations.backupRunning'))
      return
    }
    try {
      const record = await adminAPI.backup.getBackup(backupId)
      updateRecordInList(record)
      if (record.status === 'completed' || record.status === 'failed') {
        stopPolling()
        creatingBackup.value = false
        if (record.status === 'completed') {
          appStore.showSuccess(t('admin.backup.operations.backupCreated'))
        } else {
          appStore.showError(record.error_message || t('admin.backup.operations.backupFailed'))
        }
        await loadBackups()
      }
    } catch {
      // 轮询失败时不中断
    }
  }, 2000)
}

function stopPolling() {
  if (pollingTimer.value) {
    clearInterval(pollingTimer.value)
    pollingTimer.value = null
  }
}

function startRestorePolling(backupId: string) {
  stopRestorePolling()
  let count = 0
  restoringPollingTimer.value = setInterval(async () => {
    if (count++ >= MAX_POLL_COUNT) {
      stopRestorePolling()
      restoringId.value = ''
      appStore.showWarning(t('admin.backup.operations.restoreRunning'))
      return
    }
    try {
      const record = await adminAPI.backup.getBackup(backupId)
      updateRecordInList(record)
      if (record.restore_status === 'completed' || record.restore_status === 'failed') {
        stopRestorePolling()
        restoringId.value = ''
        if (record.restore_status === 'completed') {
          appStore.showSuccess(t('admin.backup.actions.restoreSuccess'))
        } else {
          appStore.showError(record.restore_error || t('admin.backup.operations.restoreFailed'))
        }
        await loadBackups()
      }
    } catch {
      // 轮询失败时不中断
    }
  }, 2000)
}

function stopRestorePolling() {
  if (restoringPollingTimer.value) {
    clearInterval(restoringPollingTimer.value)
    restoringPollingTimer.value = null
  }
}

function handleVisibilityChange() {
  if (document.hidden) {
    stopPolling()
    stopRestorePolling()
  } else {
    // 标签页恢复时刷新列表，检查是否仍有活跃操作
    loadBackups().then(() => {
      const running = backups.value.find(r => r.status === 'running')
      if (running) {
        creatingBackup.value = true
        startPolling(running.id)
      }
      const restoring = backups.value.find(r => r.restore_status === 'running')
      if (restoring) {
        restoringId.value = restoring.id
        startRestorePolling(restoring.id)
      }
    })
  }
}

// R2 guide
const showR2Guide = ref(false)
const r2ConfigRows = computed(() => [
  { field: t('admin.backup.s3.endpoint'), value: 'https://<account_id>.r2.cloudflarestorage.com' },
  { field: t('admin.backup.s3.region'), value: 'auto' },
  { field: t('admin.backup.s3.bucket'), value: t('admin.backup.r2Guide.step4.bucketValue') },
  { field: t('admin.backup.s3.prefix'), value: 'backups/' },
  { field: 'Access Key ID', value: t('admin.backup.r2Guide.step4.fromStep2') },
  { field: 'Secret Access Key', value: t('admin.backup.r2Guide.step4.fromStep2') },
  { field: t('admin.backup.s3.forcePathStyle'), value: t('admin.backup.r2Guide.step4.unchecked') },
])

async function loadS3Config() {
  try {
    const cfg = await adminAPI.backup.getS3Config()
    const provider = normalizeStorageProvider(cfg.provider)
    const region = displayRegion(provider, cfg.region)
    const bucket = cfg.bucket || ''
    s3Form.value = {
      provider,
      endpoint: cfg.endpoint || '',
      region,
      bucket,
      access_key_id: cfg.access_key_id || '',
      secret_access_key: '',
      prefix: cfg.prefix || 'backups/',
      force_path_style: Boolean(cfg.force_path_style),
    }
    s3SecretConfigured.value = Boolean(cfg.access_key_id)
    rememberResolved(s3Resolved, s3ResolvedContext, provider, region, bucket, cfg.resolved)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

async function saveS3Config() {
  savingS3.value = true
  try {
    await backupStepUp.run(() => adminAPI.backup.updateS3Config(backupSubmitPayload()))
    appStore.showSuccess(t('admin.backup.s3.saved'))
    await loadS3Config()
  } catch (error) {
    if (isStepUpCancelled(error)) {
      savingS3.value = false
      return
    }
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    savingS3.value = false
  }
}

async function loadImageStorageConfig() {
  try {
    const { config, secret_configured } = await adminAPI.backup.getImageStorageConfig()
    const provider = normalizeStorageProvider(config.provider)
    const region = displayRegion(provider, config.region)
    const bucket = config.bucket || ''
    imageStorageForm.value = {
      enabled: Boolean(config.enabled),
      reuse_backup_s3: Boolean(config.reuse_backup_s3),
      bucket,
      prefix: config.prefix || 'images/',
      public_base_url: config.public_base_url || '',
      presign_expiry_hours: config.presign_expiry_hours ?? 24,
      max_download_bytes: config.max_download_bytes ?? 33554432,
      provider,
      endpoint: config.endpoint || '',
      region,
      access_key_id: config.access_key_id || '',
      secret_access_key: '',
      force_path_style: Boolean(config.force_path_style),
    }
    imageStorageSecretConfigured.value = secret_configured
    rememberResolved(imageResolved, imageResolvedContext, provider, region, bucket, config.resolved)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

async function saveImageStorageConfig() {
  savingImageStorage.value = true
  try {
    await backupStepUp.run(() => adminAPI.backup.updateImageStorageConfig(imageSubmitPayload()))
    appStore.showSuccess(t('admin.backup.imageStorage.saved'))
    await loadImageStorageConfig()
  } catch (error) {
    if (isStepUpCancelled(error)) {
      savingImageStorage.value = false
      return
    }
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    savingImageStorage.value = false
  }
}

function applyTestResolved(
  result: { resolved?: StorageEndpointResolved },
  form: { provider?: StorageProvider; region: string; bucket: string; endpoint: string },
  resolvedRef: { value: StorageEndpointResolved | null },
  contextRef: { value: ResolvedContext | null },
) {
  if (!result.resolved?.endpoint?.trim() || form.endpoint.trim()) return
  rememberResolved(
    resolvedRef,
    contextRef,
    form.provider || 's3',
    form.region,
    form.bucket,
    result.resolved,
  )
}

async function testImageStorage() {
  testingImageStorage.value = true
  try {
    const result = await adminAPI.backup.testImageStorageConnection(imageSubmitPayload())
    applyTestResolved(result, imageStorageForm.value, imageResolved, imageResolvedContext)
    if (result.ok) {
      appStore.showSuccess(result.message || t('admin.backup.s3.testSuccess'))
    } else {
      appStore.showError(result.message || t('admin.backup.s3.testFailed'))
    }
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testingImageStorage.value = false
  }
}

async function testS3() {
  testingS3.value = true
  try {
    const result = await adminAPI.backup.testS3Connection(backupSubmitPayload())
    applyTestResolved(result, s3Form.value, s3Resolved, s3ResolvedContext)
    if (result.ok) {
      appStore.showSuccess(result.message || t('admin.backup.s3.testSuccess'))
    } else {
      appStore.showError(result.message || t('admin.backup.s3.testFailed'))
    }
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    testingS3.value = false
  }
}

async function loadSchedule() {
  try {
    const cfg = await adminAPI.backup.getSchedule()
    scheduleForm.value = {
      enabled: cfg.enabled,
      cron_expr: cfg.cron_expr || '0 2 * * *',
      retain_days: cfg.retain_days ?? 14,
      retain_count: cfg.retain_count ?? 10,
    }
    archiveForm.value = {
      enabled: cfg.monthly_archive?.enabled ?? false,
      days: cfg.monthly_archive?.days ?? [1],
      include_month_end: cfg.monthly_archive?.include_month_end ?? false,
      retain_count: cfg.monthly_archive?.retain_count ?? 0,
    }
    savedArchive.value = cloneArchive(archiveForm.value)
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

async function saveSchedule() {
  if (scheduleValidationError.value) return
  savingSchedule.value = true
  try {
    const archive = archivePayload.value
    await adminAPI.backup.updateSchedule({ ...scheduleForm.value, monthly_archive: archive })
    savedArchive.value = cloneArchive(archive)
    appStore.showSuccess(t('admin.backup.schedule.saved'))
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    savingSchedule.value = false
  }
}

async function loadBackups() {
  loadingBackups.value = true
  try {
    const result = await adminAPI.backup.listBackups()
    backups.value = result.items || []
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  } finally {
    loadingBackups.value = false
  }
}

async function createBackup() {
  creatingBackup.value = true
  try {
    const record = await backupStepUp.run(() => adminAPI.backup.createBackup({ expire_days: manualExpireDays.value }))
    // 插入到列表顶部
    backups.value.unshift(record)
    startPolling(record.id)
  } catch (error: any) {
    if (isStepUpCancelled(error)) {
      creatingBackup.value = false
      return
    }
    if (reportStepUpBlocked(error)) {
      creatingBackup.value = false
      return
    }
    if (error?.response?.status === 409) {
      appStore.showWarning(t('admin.backup.operations.alreadyInProgress'))
    } else {
      appStore.showError(error?.message || t('errors.networkError'))
    }
    creatingBackup.value = false
  }
}

async function downloadBackup(id: string) {
  try {
    const result = await backupStepUp.run(() => adminAPI.backup.getDownloadURL(id))
    if (result.parts && result.parts.length > 0) {
      downloadParts.value = result.parts
      downloadPartsModalOpen.value = true
      return
    }
    if (!result.url) {
      throw new Error(t('admin.backup.actions.downloadFailed'))
    }
    // 预签名 URL 带 attachment disposition，同页 anchor 导航直接触发下载；
    // 不用 window.open：step-up 弹窗 await 会耗尽瞬态用户激活，新标签页会被浏览器拦截。
    const link = document.createElement('a')
    link.href = result.url
    link.rel = 'noopener'
    link.click()
  } catch (error) {
    if (isStepUpCancelled(error)) return
    if (reportStepUpBlocked(error)) return
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

function closeDownloadParts() {
  downloadPartsModalOpen.value = false
  downloadParts.value = []
}

async function restoreBackup(id: string) {
  if (!window.confirm(t('admin.backup.actions.restoreConfirm'))) return
  const password = window.prompt(t('admin.backup.actions.restorePasswordPrompt'))
  if (!password) return
  restoringId.value = id
  try {
    const record = await backupStepUp.run(() => adminAPI.backup.restoreBackup(id, password))
    updateRecordInList(record)
    startRestorePolling(id)
  } catch (error: any) {
    restoringId.value = ''
    if (isStepUpCancelled(error)) return
    if (reportStepUpBlocked(error)) return
    // apiClient 拦截器把 HTTP 错误归一化为顶层 { status } 平面对象（无 response 字段）
    if (error?.status === 409 || error?.response?.status === 409) {
      appStore.showWarning(t('admin.backup.operations.restoreRunning'))
    } else {
      appStore.showError(error?.message || t('errors.networkError'))
    }
  }
}

async function removeBackup(id: string) {
  const archived = !!backups.value.find(record => record.id === id)?.monthly_archive
  if (!window.confirm(t(archived ? 'admin.backup.archive.deleteConfirm' : 'admin.backup.actions.deleteConfirm'))) return
  try {
    await adminAPI.backup.deleteBackup(id, archived)
    appStore.showSuccess(t('admin.backup.actions.deleted'))
    await loadBackups()
  } catch (error) {
    appStore.showError((error as { message?: string })?.message || t('errors.networkError'))
  }
}

function statusClass(status: string): string {
  switch (status) {
    case 'completed':
      return 'bg-green-100 text-green-700 dark:bg-green-900/30 dark:text-green-300'
    case 'running':
      return 'bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-300'
    case 'failed':
      return 'bg-red-100 text-red-700 dark:bg-red-900/30 dark:text-red-300'
    default:
      return 'bg-gray-100 text-gray-700 dark:bg-dark-800 dark:text-gray-300'
  }
}

function formatSize(bytes: number): string {
  if (!bytes || bytes <= 0) return '-'
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
}

function formatDate(value?: string): string {
  if (!value) return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  return date.toLocaleString()
}

onMounted(async () => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  await Promise.all([loadS3Config(), loadImageStorageConfig(), loadSchedule(), loadBackups()])

  // 如果有正在 running 的备份，恢复轮询
  const runningBackup = backups.value.find(r => r.status === 'running')
  if (runningBackup) {
    creatingBackup.value = true
    startPolling(runningBackup.id)
  }
  const restoringBackup = backups.value.find(r => r.restore_status === 'running')
  if (restoringBackup) {
    restoringId.value = restoringBackup.id
    startRestorePolling(restoringBackup.id)
  }
})

onBeforeUnmount(() => {
  stopPolling()
  stopRestorePolling()
  document.removeEventListener('visibilitychange', handleVisibilityChange)
})
</script>

<style scoped>
.modal-enter-active,
.modal-leave-active {
  transition: opacity 0.2s ease;
}
.modal-enter-from,
.modal-leave-to {
  opacity: 0;
}
</style>
