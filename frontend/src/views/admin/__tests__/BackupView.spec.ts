import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import BackupView from '../BackupView.vue'
import enOverview from '@/i18n/locales/en/admin/overview'
import zhOverview from '@/i18n/locales/zh/admin/overview'

const {
  getS3Config,
  updateS3Config,
  testS3Connection,
  getImageStorageConfig,
  updateImageStorageConfig,
  testImageStorageConnection,
  getSchedule,
  updateSchedule,
  deleteBackup,
  listBackups,
  getDownloadURL,
} = vi.hoisted(() => ({
  getS3Config: vi.fn(),
  updateS3Config: vi.fn(),
  testS3Connection: vi.fn(),
  getImageStorageConfig: vi.fn(),
  updateImageStorageConfig: vi.fn(),
  testImageStorageConnection: vi.fn(),
  getSchedule: vi.fn(),
  updateSchedule: vi.fn(),
  deleteBackup: vi.fn(),
  listBackups: vi.fn(),
  getDownloadURL: vi.fn(),
}))

vi.mock('@/api', () => ({
  adminAPI: {
    backup: {
      getS3Config,
      updateS3Config,
      testS3Connection,
      getImageStorageConfig,
      updateImageStorageConfig,
      testImageStorageConnection,
      getSchedule,
      updateSchedule,
      createBackup: vi.fn(),
      listBackups,
      getBackup: vi.fn(),
      deleteBackup,
      getDownloadURL,
      restoreBackup: vi.fn(),
    },
  },
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showWarning: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: (fn: () => unknown) => fn() }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      params?.index !== undefined ? `${key}:${params.index}` : params?.day !== undefined ? `${key}:${params.day}` : key,
  }),
}))

const baseRecord = (id: string, parts?: unknown[]) => ({
  id,
  status: 'completed',
  backup_type: 'postgres',
  file_name: `${id}.sql.gz`,
  s3_key: `backups/${id}.sql.gz`,
  parts,
  size_bytes: 10,
  triggered_by: 'manual',
  started_at: '2026-08-09T00:00:00Z',
})

const wrappers: ReturnType<typeof mount>[] = []

function mountBackupView() {
  const wrapper = mount(BackupView, {
    global: {
      stubs: {
        TotpStepUpDialog: true,
      },
    },
  })
  wrappers.push(wrapper)
  return wrapper
}

describe('admin BackupView', () => {
  beforeEach(() => {
    getS3Config.mockResolvedValue({})
    updateS3Config.mockReset().mockResolvedValue({})
    testS3Connection.mockReset().mockResolvedValue({ ok: true, message: 'ok' })
    getImageStorageConfig.mockResolvedValue({ config: {}, secret_configured: false })
    updateImageStorageConfig.mockReset().mockResolvedValue({})
    testImageStorageConnection.mockReset().mockResolvedValue({ ok: true, message: 'ok' })
    getSchedule.mockResolvedValue({ enabled: false, cron_expr: '', retain_days: 14, retain_count: 10 })
    updateSchedule.mockReset().mockResolvedValue({})
    deleteBackup.mockReset().mockResolvedValue(undefined)
    listBackups.mockResolvedValue({ items: [] })
    getDownloadURL.mockReset()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
  })

  afterEach(() => {
    wrappers.splice(0).forEach(wrapper => wrapper.unmount())
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('显示分卷数并在下载时列出每个分卷链接', async () => {
    listBackups.mockResolvedValue({
      items: [baseRecord('split', [{ index: 1 }, { index: 2 }, { index: 3 }])],
    })
    getDownloadURL.mockResolvedValue({
      parts: [
        { index: 1, size_bytes: 5, url: 'https://example.test/part-1' },
        { index: 2, size_bytes: 6, url: 'https://example.test/part-2' },
        { index: 3, size_bytes: 7, url: 'https://example.test/part-3' },
      ],
    })

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.text()).toContain('3')
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    expect(downloadButton).toBeDefined()
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:1')
    expect(document.body.textContent).toContain('admin.backup.actions.partLabel:3')
    expect(document.body.querySelector('a[href="https://example.test/part-2"]')).not.toBeNull()
  })

  it('旧单文件记录仍使用单个下载地址', async () => {
    listBackups.mockResolvedValue({ items: [baseRecord('legacy')] })
    getDownloadURL.mockResolvedValue({ url: 'https://example.test/legacy.sql.gz' })

    const wrapper = mountBackupView()
    await flushPromises()
    const downloadButton = wrapper.findAll('button').find(button =>
      button.text().includes('admin.backup.actions.download'),
    )
    await downloadButton!.trigger('click')
    await flushPromises()

    expect(getDownloadURL).toHaveBeenCalledWith('legacy')
    expect(document.body.textContent).not.toContain('admin.backup.actions.downloadParts')
  })

  it('运行中的备份不显示删除入口', async () => {
    listBackups.mockResolvedValue({
      items: [{ ...baseRecord('running'), status: 'running', progress: 'uploading' }],
    })

    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.find('tbody tr td:nth-child(5)').text()).toBe('-')
    expect(wrapper.findAll('button').some(button => button.text() === 'common.delete')).toBe(false)
  })

  it('兼容旧配置并保留 0 值，不自动启用月度归档', async () => {
    getSchedule.mockResolvedValue({ enabled: true, cron_expr: '0 4 * * *', retain_days: 0, retain_count: 0 })
    const wrapper = mountBackupView()
    await flushPromises()
    expect((wrapper.get('[data-testid="backup-retain-days"]').element as HTMLInputElement).value).toBe('0')
    expect((wrapper.get('[data-testid="backup-retain-count"]').element as HTMLInputElement).value).toBe('0')
    expect((wrapper.get('[data-testid="archive-enabled"]').element as HTMLInputElement).checked).toBe(false)
    await wrapper.get('[data-testid="backup-schedule"] .btn-primary').trigger('click')
    await flushPromises()
    expect(updateSchedule).toHaveBeenCalledWith(expect.objectContaining({ retain_days: 0, retain_count: 0, monthly_archive: expect.objectContaining({ enabled: false }) }))
  })

  it('多选日期，手填份数；永久保留隐藏数量并在取消后恢复', async () => {
    const wrapper = mountBackupView()
    await flushPromises()
    await wrapper.get('[data-testid="archive-enabled"]').setValue(true)
    await wrapper.get('#backup-archive-dates').trigger('click')
    await wrapper.get('#backup-archive-options input[value="15"]').setValue(true)
    await wrapper.get('#backup-archive-options input[value="last"]').setValue(true)
    expect(wrapper.get('#backup-archive-dates').attributes('aria-expanded')).toBe('true')
    expect(wrapper.find('[data-testid="archive-count"]').exists()).toBe(false)
    await wrapper.get('[data-testid="archive-forever"]').setValue(false)
    await wrapper.get('[data-testid="archive-count"]').setValue(17)
    await wrapper.get('[data-testid="archive-forever"]').setValue(true)
    expect(wrapper.find('[data-testid="archive-count"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'BackupArchiveSettings' }).text()).not.toContain('admin.backup.archive.copies')
    await wrapper.get('[data-testid="archive-forever"]').setValue(false)
    expect((wrapper.get('[data-testid="archive-count"]').element as HTMLInputElement).value).toBe('17')
    await wrapper.get('[data-testid="backup-schedule"] .btn-primary').trigger('click')
    await flushPromises()
    expect(updateSchedule).toHaveBeenLastCalledWith(expect.objectContaining({ monthly_archive: { enabled: true, days: [1, 15], include_month_end: true, retain_count: 17 } }))
    await wrapper.get('[data-testid="archive-forever"]').setValue(true)
    await wrapper.get('[data-testid="backup-schedule"] .btn-primary').trigger('click')
    expect(updateSchedule).toHaveBeenLastCalledWith(expect.objectContaining({ monthly_archive: expect.objectContaining({ retain_count: 0 }) }))
  })

  it('空日期和非法份数不可保存，输入 0 不会误选永久保留', async () => {
    const wrapper = mountBackupView()
    await flushPromises()
    await wrapper.get('[data-testid="archive-enabled"]').setValue(true)
    await wrapper.get('#backup-archive-dates').trigger('click')
    await wrapper.get('#backup-archive-options input[value="1"]').setValue(false)
    const save = wrapper.get('[data-testid="backup-schedule"] .btn-primary')
    expect(save.attributes('disabled')).toBeDefined()
    await wrapper.get('#backup-archive-options input[value="15"]').setValue(true)
    await wrapper.get('[data-testid="archive-forever"]').setValue(false)
    for (const value of ['0', '-1', '1.5', '']) {
      await wrapper.get('[data-testid="archive-count"]').setValue(value)
      expect(save.attributes('disabled')).toBeDefined()
      expect((wrapper.get('[data-testid="archive-forever"]').element as HTMLInputElement).checked).toBe(false)
    }
    await save.trigger('click')
    expect(updateSchedule).not.toHaveBeenCalled()
  })

  it('关闭月度归档后不提交隐藏的归档参数，重新启用时保留编辑值', async () => {
    getSchedule.mockResolvedValue({ enabled: true, cron_expr: '0 4 * * *', retain_days: 14, retain_count: 10,
      monthly_archive: { enabled: true, days: [1, 15], include_month_end: false, retain_count: 12 } })
    const wrapper = mountBackupView()
    await flushPromises()
    await wrapper.get('[data-testid="archive-count"]').setValue(1)
    await wrapper.get('[data-testid="archive-enabled"]').setValue(false)
    expect(wrapper.find('[data-testid="archive-count"]').exists()).toBe(false)
    const save = wrapper.get('[data-testid="backup-schedule"] .btn-primary')
    await save.trigger('click')
    await flushPromises()
    expect(updateSchedule).toHaveBeenLastCalledWith(expect.objectContaining({ monthly_archive: { enabled: false, days: [1, 15], include_month_end: false, retain_count: 12 } }))
    await wrapper.get('[data-testid="archive-enabled"]').setValue(true)
    expect((wrapper.get('[data-testid="archive-count"]').element as HTMLInputElement).value).toBe('1')
    // A hidden invalid count neither blocks saving nor reaches the request.
    await wrapper.get('[data-testid="archive-count"]').setValue('')
    expect(save.attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="archive-enabled"]').setValue(false)
    expect(save.attributes('disabled')).toBeUndefined()
    await save.trigger('click')
    await flushPromises()
    expect(updateSchedule).toHaveBeenLastCalledWith(expect.objectContaining({ monthly_archive: { enabled: false, days: [1, 15], include_month_end: false, retain_count: 12 } }))
    await wrapper.get('[data-testid="archive-enabled"]').setValue(true)
    await wrapper.get('[data-testid="archive-count"]').setValue(1)
    await save.trigger('click')
    await flushPromises()
    expect(updateSchedule).toHaveBeenLastCalledWith(expect.objectContaining({ monthly_archive: { enabled: true, days: [1, 15], include_month_end: false, retain_count: 1 } }))
  })

  it('加载归档设置并支持键盘关闭日期选择器', async () => {
    getSchedule.mockResolvedValue({ enabled: true, cron_expr: '0 4 * * *', retain_days: 14, retain_count: 10,
      monthly_archive: { enabled: true, days: [1, 15], include_month_end: true, retain_count: 23 } })
    const wrapper = mountBackupView()
    await flushPromises()
    expect((wrapper.get('[data-testid="archive-count"]').element as HTMLInputElement).value).toBe('23')
    await wrapper.get('#backup-archive-dates').trigger('keydown', { key: 'ArrowDown' })
    await flushPromises()
    expect(wrapper.find('#backup-archive-options').exists()).toBe(true)
    expect((wrapper.get('#backup-archive-options input[value="15"]').element as HTMLInputElement).checked).toBe(true)
    await wrapper.get('#backup-archive-options').trigger('keydown', { key: 'Escape' })
    expect(wrapper.find('#backup-archive-options').exists()).toBe(false)
  })

  it('归档删除需要单独确认，有限归档不会显示永不过期', async () => {
    listBackups.mockResolvedValue({ items: [{ ...baseRecord('archived'), monthly_archive: { dates: ['2026-09-01', '2026-09-15'], retain_count: 12 } }] })
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const wrapper = mountBackupView()
    await flushPromises()
    expect(wrapper.text()).toContain('admin.backup.archive.badge')
    expect(wrapper.get('tbody tr td:nth-child(6)').text()).toBe('admin.backup.archive.retainLatest')
    const button = wrapper.findAll('button').find(button => button.text() === 'common.delete')!
    await button.trigger('click')
    expect(confirm).toHaveBeenCalledWith('admin.backup.archive.deleteConfirm')
    expect(deleteBackup).not.toHaveBeenCalled()
    confirm.mockReturnValue(true)
    await button.trigger('click')
    await flushPromises()
    expect(deleteBackup).toHaveBeenCalledWith('archived', true)
  })

  function field(wrapper: ReturnType<typeof mount>, testId: string) {
    return wrapper.get(`[data-testid="${testId}"]`).element as HTMLInputElement
  }

  it('中英文包含四家服务商文案和合同里的提示', () => {
    expect(zhOverview.backup.s3.providers).toEqual({
      s3: 'Cloudflare R2 / S3 兼容',
      aliyun_oss: '阿里云 OSS',
      tencent_cos: '腾讯云 COS',
      qiniu: '七牛云',
    })
    expect(enOverview.backup.s3.providers).toEqual({
      s3: 'Cloudflare R2 / S3-compatible',
      aliyun_oss: 'Alibaba Cloud OSS',
      tencent_cos: 'Tencent Cloud COS',
      qiniu: 'Qiniu Kodo',
    })
    expect(zhOverview.backup.s3.endpointDeriveHint).toBe('留空则按区域生成外网 S3 兼容地址')
    expect(zhOverview.backup.s3.tencentBucketHint).toBe('example-1250000000')
    expect(zhOverview.backup.s3.qiniuBucketHint).toBe('请使用存储桶概览中的 S3 空间名称')
    expect(zhOverview.backup.s3.secretConfigured).toBe('已配置，留空保持不变')
    expect(zhOverview.backup.imageStorage.reuseBackupS3).toBe('复用上方备份的对象存储配置（只用不同的存储桶/前缀）')
    expect(zhOverview.backup.imageStorage.qiniuPublicBaseUrlHint).toBe('七牛 S3 域名不能匿名访问；公开直链请填已绑定域名，留空则返回预签名链接')
    expect(enOverview.backup.s3.endpointDeriveHint).toBe('Leave empty to derive the public S3-compatible endpoint from the region.')
    expect(enOverview.backup.s3.qiniuBucketHint).toBe('Use the S3 space name from the bucket overview.')
    expect(enOverview.backup.imageStorage.reuseBackupS3).toBe('Reuse the object storage configuration above (different bucket/prefix only)')
    expect(enOverview.backup.imageStorage.qiniuPublicBaseUrlHint).toBe('The Qiniu S3 domain does not allow anonymous access. Enter a bound domain for a public direct link, or leave empty to return a presigned URL.')
  })

  it('缺少 provider 时按 s3 加载，空 region 显示 auto，空前缀回落到 backups/', async () => {
    getS3Config.mockResolvedValue({ endpoint: '', region: '  ', bucket: 'bucket-a', prefix: '', access_key_id: 'AK' })
    const wrapper = mountBackupView()
    await flushPromises()

    expect(field(wrapper, 'backup-storage-provider').value).toBe('s3')
    expect(field(wrapper, 'backup-storage-region').value).toBe('auto')
    expect(field(wrapper, 'backup-storage-prefix').value).toBe('backups/')
    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('')
    expect(field(wrapper, 'backup-storage-endpoint').placeholder).toBe('https://<account_id>.r2.cloudflarestorage.com')
    expect(field(wrapper, 'backup-storage-secret').value).toBe('')
    expect(field(wrapper, 'backup-storage-secret').placeholder).toBe('admin.backup.s3.secretConfigured')
    expect(wrapper.find('[data-testid="backup-r2-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="backup-force-path-style"]').exists()).toBe(true)

    await wrapper.get('[data-testid="backup-s3-save"]').trigger('click')
    await flushPromises()
    const payload = updateS3Config.mock.calls[0][0]
    expect(payload).toMatchObject({
      provider: 's3',
      endpoint: '',
      region: 'auto',
      bucket: 'bucket-a',
      access_key_id: 'AK',
      secret_access_key: '',
      prefix: 'backups/',
      force_path_style: false,
    })
    expect(payload).not.toHaveProperty('resolved')
  })

  it('空 endpoint 只把后端解析出的地址放进 placeholder', async () => {
    getS3Config.mockResolvedValue({
      provider: 'aliyun_oss',
      endpoint: '',
      region: 'cn-beijing',
      bucket: 'example',
      prefix: 'backups/',
      access_key_id: 'AK',
      force_path_style: true,
      resolved: {
        endpoint: 'https://s3.oss-cn-beijing.aliyuncs.com',
        region: 'cn-beijing',
        force_path_style: false,
      },
    })
    const wrapper = mountBackupView()
    await flushPromises()

    const endpoint = field(wrapper, 'backup-storage-endpoint')
    expect(endpoint.value).toBe('')
    expect(endpoint.placeholder).toBe('https://s3.oss-cn-beijing.aliyuncs.com')
    expect(field(wrapper, 'backup-storage-region').value).toBe('cn-beijing')
    expect(field(wrapper, 'backup-storage-region').placeholder).toBe('cn-hangzhou')
    expect(wrapper.find('[data-testid="backup-endpoint-derive-hint"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="backup-force-path-style"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="backup-r2-guide"]').exists()).toBe(false)

    await wrapper.get('[data-testid="backup-storage-region"]').setValue('cn-shanghai')
    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('')
    expect(field(wrapper, 'backup-storage-endpoint').placeholder).toBe('https://s3.oss-cn-hangzhou.aliyuncs.com')

    await wrapper.get('[data-testid="backup-s3-save"]').trigger('click')
    await flushPromises()
    const payload = updateS3Config.mock.calls[0][0]
    expect(payload.endpoint).toBe('')
    expect(payload.region).toBe('cn-shanghai')
    expect(payload.force_path_style).toBe(true)
    expect(payload).not.toHaveProperty('resolved')
  })

  it('自定义 endpoint 原样保留，并在非 s3 时显示路径风格', async () => {
    getS3Config.mockResolvedValue({
      provider: 'aliyun_oss',
      endpoint: 'https://oss-cn-hangzhou.aliyuncs.com',
      region: 'oss-cn-hangzhou',
      bucket: 'example',
      force_path_style: true,
      resolved: {
        endpoint: 'https://oss-cn-hangzhou.aliyuncs.com',
        region: 'cn-hangzhou',
        force_path_style: true,
      },
    })
    const wrapper = mountBackupView()
    await flushPromises()

    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('https://oss-cn-hangzhou.aliyuncs.com')
    expect(field(wrapper, 'backup-storage-region').value).toBe('oss-cn-hangzhou')
    expect(wrapper.find('[data-testid="backup-force-path-style"]').exists()).toBe(true)
  })

  it('切到非 s3 时清空 auto 和 endpoint，切回 s3 时不清除已填写的 endpoint', async () => {
    getS3Config.mockResolvedValue({
      provider: 's3',
      endpoint: 'https://acct.r2.cloudflarestorage.com',
      region: 'auto',
      bucket: 'keep-bucket',
      prefix: 'keep/',
      access_key_id: 'AK',
    })
    const wrapper = mountBackupView()
    await flushPromises()

    await wrapper.get('[data-testid="backup-storage-provider"]').setValue('tencent_cos')
    expect(field(wrapper, 'backup-storage-provider').value).toBe('tencent_cos')
    expect(field(wrapper, 'backup-storage-region').value).toBe('')
    expect(field(wrapper, 'backup-storage-region').placeholder).toBe('ap-guangzhou')
    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('')
    expect(field(wrapper, 'backup-storage-endpoint').placeholder).toBe('https://cos.ap-guangzhou.myqcloud.com')
    expect(field(wrapper, 'backup-storage-bucket').value).toBe('keep-bucket')
    expect(field(wrapper, 'backup-storage-bucket').placeholder).toBe('example-1250000000')
    expect(wrapper.get('[data-testid="backup-bucket-hint"]').text()).toBe('admin.backup.s3.tencentBucketHint')
    expect(field(wrapper, 'backup-storage-prefix').value).toBe('keep/')
    expect(field(wrapper, 'backup-storage-access-key').value).toBe('AK')
    expect(wrapper.find('[data-testid="backup-force-path-style"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="backup-r2-guide"]').exists()).toBe(false)

    await wrapper.get('[data-testid="backup-storage-region"]').setValue('ap-guangzhou')
    await wrapper.get('[data-testid="backup-storage-endpoint"]').setValue('https://cos.example.com')
    await wrapper.get('[data-testid="backup-storage-provider"]').setValue('s3')
    expect(field(wrapper, 'backup-storage-region').value).toBe('ap-guangzhou')
    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('https://cos.example.com')
    expect(wrapper.find('[data-testid="backup-r2-guide"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="backup-force-path-style"]').exists()).toBe(true)

    await wrapper.get('[data-testid="backup-storage-region"]').setValue('')
    await wrapper.get('[data-testid="backup-storage-provider"]').setValue('qiniu')
    expect(field(wrapper, 'backup-storage-region').value).toBe('')
    expect(field(wrapper, 'backup-storage-region').placeholder).toBe('cn-east-1')
    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('')
    expect(field(wrapper, 'backup-storage-endpoint').placeholder).toBe('https://s3.cn-east-1.qiniucs.com')
    expect(wrapper.get('[data-testid="backup-bucket-hint"]').text()).toBe('admin.backup.s3.qiniuBucketHint')

    await wrapper.get('[data-testid="backup-storage-provider"]').setValue('s3')
    expect(field(wrapper, 'backup-storage-region').value).toBe('auto')
  })

  it('非空且不是 auto 的 region 在服务商之间保留', async () => {
    getS3Config.mockResolvedValue({ provider: 's3', region: 'us-east-1', endpoint: 'https://s3.example.com', bucket: 'b' })
    const wrapper = mountBackupView()
    await flushPromises()
    await wrapper.get('[data-testid="backup-storage-provider"]').setValue('qiniu')
    expect(field(wrapper, 'backup-storage-region').value).toBe('us-east-1')
    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('')
  })

  it('其他服务商的空 region 不会被替换成 auto', async () => {
    getS3Config.mockResolvedValue({ provider: 'qiniu', region: '', endpoint: '' })
    const wrapper = mountBackupView()
    await flushPromises()
    expect(field(wrapper, 'backup-storage-provider').value).toBe('qiniu')
    expect(field(wrapper, 'backup-storage-region').value).toBe('')
  })

  it('测试连接提交合同字段，并用返回的 resolved 更新 placeholder', async () => {
    getS3Config.mockResolvedValue({ provider: 'aliyun_oss', region: 'cn-hangzhou', endpoint: '', bucket: 'example' })
    testS3Connection.mockResolvedValue({
      ok: true,
      message: 'ok',
      resolved: { endpoint: 'https://s3.oss-cn-hangzhou.aliyuncs.com', region: 'cn-hangzhou', force_path_style: false },
    })
    const wrapper = mountBackupView()
    await flushPromises()
    expect(field(wrapper, 'backup-storage-endpoint').placeholder).toBe('https://s3.oss-cn-hangzhou.aliyuncs.com')

    await wrapper.get('[data-testid="backup-storage-region"]').setValue('cn-shanghai')
    expect(field(wrapper, 'backup-storage-endpoint').placeholder).toBe('https://s3.oss-cn-hangzhou.aliyuncs.com')

    testS3Connection.mockResolvedValue({
      ok: true,
      message: 'ok',
      resolved: { endpoint: 'https://s3.oss-cn-shanghai.aliyuncs.com', region: 'cn-shanghai', force_path_style: false },
    })
    const testButton = wrapper.findAll('button').find(button => button.text() === 'admin.backup.s3.testConnection')!
    await testButton.trigger('click')
    await flushPromises()
    expect(testS3Connection.mock.calls[0][0]).toMatchObject({
      provider: 'aliyun_oss',
      endpoint: '',
      region: 'cn-shanghai',
      secret_access_key: '',
    })
    expect(testS3Connection.mock.calls[0][0]).not.toHaveProperty('resolved')
    expect(field(wrapper, 'backup-storage-endpoint').value).toBe('')
    expect(field(wrapper, 'backup-storage-endpoint').placeholder).toBe('https://s3.oss-cn-shanghai.aliyuncs.com')
  })

  it('复用备份配置时隐藏图像的服务商和密钥，七牛公开域名提示跟随实际服务商', async () => {
    getS3Config.mockResolvedValue({ provider: 'qiniu', region: 'cn-east-1', bucket: 'space' })
    getImageStorageConfig.mockResolvedValue({
      config: {
        enabled: true,
        reuse_backup_s3: true,
        bucket: '',
        prefix: '',
        public_base_url: 'https://img.example.com',
        presign_expiry_hours: 12,
        provider: 'aliyun_oss',
        endpoint: 'https://hidden.example.com',
        region: 'cn-hangzhou',
        access_key_id: 'IMG',
        force_path_style: true,
      },
      secret_configured: true,
    })
    const wrapper = mountBackupView()
    await flushPromises()

    expect(wrapper.text()).toContain('admin.backup.imageStorage.reuseBackupS3')
    expect(wrapper.find('[data-testid="image-storage-provider"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-storage-endpoint"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-storage-region"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-storage-access-key"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-storage-secret"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-force-path-style"]').exists()).toBe(false)
    expect(field(wrapper, 'image-storage-prefix').value).toBe('images/')
    expect(field(wrapper, 'image-public-base-url').value).toBe('https://img.example.com')
    expect(wrapper.find('[data-testid="image-qiniu-public-base-hint"]').exists()).toBe(true)
    expect(field(wrapper, 'image-storage-bucket').placeholder).toBe('admin.backup.imageStorage.bucketInherited')

    await wrapper.get('[data-testid="image-storage-save"]').trigger('click')
    await flushPromises()
    const payload = updateImageStorageConfig.mock.calls[0][0]
    expect(payload.secret_access_key).toBe('')
    expect(payload.force_path_style).toBe(true)
    expect(payload.public_base_url).toBe('https://img.example.com')
    expect(payload.presign_expiry_hours).toBe(12)
    expect(payload).not.toHaveProperty('resolved')
  })

  it('图像存储单独配置时遵循同样的服务商切换规则', async () => {
    getImageStorageConfig.mockResolvedValue({
      config: {
        reuse_backup_s3: false,
        provider: 's3',
        region: '',
        endpoint: 'https://acct.r2.cloudflarestorage.com',
        bucket: 'images',
        prefix: 'img/',
        public_base_url: 'https://cdn.example.com',
        presign_expiry_hours: 6,
        access_key_id: 'IMG',
        force_path_style: false,
      },
      secret_configured: true,
    })
    const wrapper = mountBackupView()
    await flushPromises()

    expect(field(wrapper, 'image-storage-provider').value).toBe('s3')
    expect(field(wrapper, 'image-storage-region').value).toBe('auto')
    expect(field(wrapper, 'image-storage-secret').placeholder).toBe('admin.backup.s3.secretConfigured')
    expect(wrapper.find('[data-testid="image-qiniu-public-base-hint"]').exists()).toBe(false)

    await wrapper.get('[data-testid="image-storage-provider"]').setValue('aliyun_oss')
    expect(field(wrapper, 'image-storage-region').value).toBe('')
    expect(field(wrapper, 'image-storage-region').placeholder).toBe('cn-hangzhou')
    expect(field(wrapper, 'image-storage-endpoint').value).toBe('')
    expect(field(wrapper, 'image-storage-endpoint').placeholder).toBe('https://s3.oss-cn-hangzhou.aliyuncs.com')
    expect(field(wrapper, 'image-storage-bucket').value).toBe('images')
    expect(field(wrapper, 'image-storage-prefix').value).toBe('img/')
    expect(field(wrapper, 'image-public-base-url').value).toBe('https://cdn.example.com')
    expect(field(wrapper, 'image-presign-hours').value).toBe('6')
    expect(field(wrapper, 'image-storage-access-key').value).toBe('IMG')
    expect(wrapper.find('[data-testid="image-endpoint-derive-hint"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="image-force-path-style"]').exists()).toBe(false)

    await wrapper.get('[data-testid="image-storage-provider"]').setValue('qiniu')
    expect(wrapper.find('[data-testid="image-qiniu-public-base-hint"]').exists()).toBe(true)
    expect(wrapper.get('[data-testid="image-bucket-hint"]').text()).toBe('admin.backup.s3.qiniuBucketHint')

    await wrapper.get('[data-testid="image-reuse-backup"]').setValue(true)
    expect(wrapper.find('[data-testid="image-storage-provider"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="image-storage-endpoint"]').exists()).toBe(false)
    expect(field(wrapper, 'image-public-base-url').value).toBe('https://cdn.example.com')
  })

  it('图像存储空 region 在非 s3 时保持为空，解析地址不写入输入框', async () => {
    getImageStorageConfig.mockResolvedValue({
      config: {
        reuse_backup_s3: false,
        provider: 'tencent_cos',
        region: '',
        endpoint: '',
        bucket: 'example-1250000000',
        force_path_style: true,
        resolved: {
          endpoint: 'https://cos.ap-guangzhou.myqcloud.com',
          region: 'ap-guangzhou',
          force_path_style: false,
        },
      },
      secret_configured: false,
    })
    const wrapper = mountBackupView()
    await flushPromises()
    expect(field(wrapper, 'image-storage-region').value).toBe('')
    expect(field(wrapper, 'image-storage-endpoint').value).toBe('')
    expect(field(wrapper, 'image-storage-endpoint').placeholder).toBe('https://cos.ap-guangzhou.myqcloud.com')
    expect(wrapper.find('[data-testid="image-force-path-style"]').exists()).toBe(false)
    expect(wrapper.get('[data-testid="image-bucket-hint"]').text()).toBe('admin.backup.s3.tencentBucketHint')
  })
})
