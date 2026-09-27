import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import OssView from '../OssView.vue'

const { listOSSRepositories, createOSSRepository, updateOSSRepository, deleteOSSRepository, checkOSSRepository } = vi.hoisted(() => ({
  listOSSRepositories: vi.fn(),
  createOSSRepository: vi.fn(),
  updateOSSRepository: vi.fn(),
  deleteOSSRepository: vi.fn(),
  checkOSSRepository: vi.fn(),
}))

vi.mock('@/api/oss', () => ({
  listOSSRepositories,
  createOSSRepository,
  updateOSSRepository,
  deleteOSSRepository,
  checkOSSRepository,
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

const sample = {
  id: 12,
  provider: 'aliyun' as const,
  bucket: 'photos',
  domain: 'https://cdn.example',
  region: 'cn-hangzhou',
  access_key_id: 'ak',
  secret_configured: true,
  created_at: '2026-09-27T00:00:00Z',
  updated_at: '2026-09-27T00:00:00Z',
}

function mountView() {
  return mount(OssView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: {
          props: ['show', 'title'],
          template: '<div v-if="show" data-testid="oss-dialog"><slot /><slot name="footer" /></div>',
        },
        RouterLink: { props: ['to'], template: '<a :href="to"><slot /></a>' },
      },
    },
  })
}

describe('user OssView', () => {
  beforeEach(() => {
    listOSSRepositories.mockReset().mockResolvedValue([sample])
    createOSSRepository.mockReset().mockResolvedValue(sample)
    updateOSSRepository.mockReset().mockResolvedValue(sample)
    deleteOSSRepository.mockReset().mockResolvedValue(undefined)
    checkOSSRepository.mockReset().mockResolvedValue({ ok: true, message: 'connection successful' })
    vi.stubGlobal('navigator', { clipboard: { writeText: vi.fn().mockResolvedValue(undefined) } })
    vi.stubGlobal('confirm', vi.fn(() => true))
  })

  it('lists provider, bucket, domain and copies the repository id', async () => {
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.get('[data-testid="oss-provider"]').text()).toContain('oss.providers.aliyun')
    expect(wrapper.get('[data-testid="oss-bucket"]').text()).toContain('photos')
    expect(wrapper.get('[data-testid="oss-domain"]').text()).toContain('https://cdn.example')

    await wrapper.get('[data-testid="oss-copy-12"]').trigger('click')
    await flushPromises()
    expect(navigator.clipboard.writeText).toHaveBeenCalledWith('12')
    expect(wrapper.get('[data-testid="oss-message"]').text()).toContain('12')
  })

  it('switches provider fields and creates a repository', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="oss-create"]').trigger('click')
    await wrapper.get('[data-testid="oss-form-provider"]').setValue('tencent')

    expect(wrapper.get('[data-testid="oss-bucket-url"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="oss-form-bucket"]').exists()).toBe(false)

    await wrapper.get('[data-testid="oss-secret-id"]').setValue('sid')
    await wrapper.get('[data-testid="oss-secret"]').setValue('skey')
    await wrapper.get('[data-testid="oss-bucket-url"]').setValue('https://example-1250000000.cos.ap-guangzhou.myqcloud.com')
    await wrapper.get('[data-testid="oss-form-domain"]').setValue('https://cdn.example')
    await wrapper.get('[data-testid="oss-save"]').trigger('click')
    await flushPromises()

    expect(createOSSRepository).toHaveBeenCalledWith(expect.objectContaining({
      provider: 'tencent',
      secret_id: 'sid',
      secret_key: 'skey',
      bucket_url: 'https://example-1250000000.cos.ap-guangzhou.myqcloud.com',
      domain: 'https://cdn.example',
    }))
  })

  it('checks connectivity and edits without sending a new secret', async () => {
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-testid="oss-check-12"]').trigger('click')
    await flushPromises()
    expect(checkOSSRepository).toHaveBeenCalledWith(12)

    await wrapper.get('[data-testid="oss-edit-12"]').trigger('click')
    expect(wrapper.get('[data-testid="oss-secret"]').attributes('placeholder')).toBe('oss.secretPlaceholder')
    await wrapper.get('[data-testid="oss-form-domain"]').setValue('https://cdn.example/new')
    await wrapper.get('[data-testid="oss-save"]').trigger('click')
    await flushPromises()
    expect(updateOSSRepository).toHaveBeenCalledWith(12, expect.objectContaining({
      provider: 'aliyun',
      access_key_secret: '',
      domain: 'https://cdn.example/new',
    }))
  })
})
