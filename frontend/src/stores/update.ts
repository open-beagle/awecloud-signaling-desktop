import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export interface ArtifactInfo {
  id: string
  role: string
  os: string
  arch: string
  package_type: string
  filename: string
  download_url: string
  size: number
  sha256: string
}

export interface PublicManifest {
  schema_version: number
  generated_at: string
  expires_at: string
  release: {
    id?: string
    version: string
    commit_id?: string
    commit_time?: string
    published_at?: string
    channel: string
    release_notes?: string
    min_supported_version?: string
  }
  update: { available: boolean; required: boolean }
  artifacts: { app: ArtifactInfo }
}

export interface DesktopUpdateState {
  current_version: string
  current_commit_id: string
  current_commit_time: string
  checked_at: string
  manifest: PublicManifest
}

export const useUpdateStore = defineStore('update', () => {
  const currentVersion = ref('')
  const currentCommitID = ref('')
  const currentCommitTime = ref('')
  const manifest = ref<PublicManifest | null>(null)
  const checkedAt = ref('')
  const isChecking = ref(false)
  const checkError = ref('')
  const isHandingOff = ref(false)
  const modalRequest = ref(0)

  const updateAvailable = computed(() => Boolean(manifest.value?.update?.available))
  const targetVersion = computed(() => manifest.value?.release?.version || '')
  const releaseNotes = computed(() => manifest.value?.release?.release_notes || '')
  const artifactSize = computed(() => manifest.value?.artifacts?.app?.size || 0)
  const required = computed(() => Boolean(manifest.value?.update?.required))

  const setLocalVersion = (version: string, commitID: string, commitTime: string) => {
    currentVersion.value = version
    currentCommitID.value = commitID
    currentCommitTime.value = commitTime
  }

  const applyCheck = (state: DesktopUpdateState) => {
    setLocalVersion(state.current_version, state.current_commit_id, state.current_commit_time)
    manifest.value = state.manifest
    checkedAt.value = state.checked_at
    checkError.value = ''
  }

  const openUpdate = () => { modalRequest.value += 1 }

  return {
    currentVersion, currentCommitID, currentCommitTime, manifest, checkedAt,
    isChecking, checkError, isHandingOff, modalRequest,
    updateAvailable, targetVersion, releaseNotes, artifactSize, required,
    setLocalVersion, applyCheck, openUpdate
  }
})
