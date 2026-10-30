<template>
  <div class="ipaccess-page">
    <div class="page-header">
      <div class="title-section">
        <h1 class="page-title">IP Access</h1>
        <p class="page-subtitle">
          Allow or deny client IPs of tcp proxies on the server side. The real
          client IP of a tcp proxy is only visible here, the service behind frpc
          always sees the loopback address.
        </p>
      </div>
      <ActionButton variant="outline" size="small" @click="refreshAll">
        Refresh
      </ActionButton>
    </div>

    <el-alert
      v-if="settings && !settings.enabled"
      class="notice"
      type="warning"
      :closable="false"
      show-icon
      title="IP access control is disabled"
      description="Add an [ipAccessControl] section with enable = true to the frps configuration and restart frps to start filtering. Rules and records below are only used when it is enabled."
    />

    <div v-loading="loading" class="content">
      <el-card v-if="settings" class="section" shadow="never">
        <template #header>
          <div class="section-header">
            <span class="section-title">Mode</span>
            <span class="section-meta">
              {{ settings.ruleCount }} rules · {{ settings.recordCount }}
              records · store: {{ settings.storePath }}
            </span>
          </div>
        </template>

        <div class="mode-block">
          <el-radio-group
            v-model="globalMode"
            :disabled="!settings.enabled || savingMode"
            @change="onGlobalModeChange"
          >
            <el-radio-button value="off">Off</el-radio-button>
            <el-radio-button value="allowlist">Allowlist</el-radio-button>
            <el-radio-button value="denylist">Denylist</el-radio-button>
          </el-radio-group>
          <p class="hint">{{ modeHint }}</p>
        </div>

        <div class="proxy-mode-block">
          <div class="sub-title">Per proxy override</div>
          <p class="hint">
            Proxies without an override inherit the global mode.
          </p>
          <div class="inline-form">
            <el-select
              v-model="proxyModeForm.proxyName"
              class="proxy-select"
              placeholder="Proxy name"
              filterable
              allow-create
              default-first-option
            >
              <el-option
                v-for="name in proxyNames"
                :key="name"
                :label="name"
                :value="name"
              />
            </el-select>
            <el-select v-model="proxyModeForm.mode" class="mode-select">
              <el-option label="Inherit global" value="inherit" />
              <el-option label="Off" value="off" />
              <el-option label="Allowlist" value="allowlist" />
              <el-option label="Denylist" value="denylist" />
            </el-select>
            <ActionButton size="small" :loading="savingMode" @click="saveProxyMode">
              Save
            </ActionButton>
          </div>

          <el-table
            v-if="settings.proxyModes.length > 0"
            :data="settings.proxyModes"
            size="small"
            class="mode-table"
          >
            <el-table-column prop="proxyName" label="Proxy" min-width="200" />
            <el-table-column label="Mode" width="140">
              <template #default="{ row }">
                <el-tag size="small" :type="modeTagType(row.mode)">
                  {{ row.mode }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column label="Actions" width="110" align="right">
              <template #default="{ row }">
                <el-button link type="danger" @click="removeProxyMode(row.proxyName)">
                  Remove
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>
      </el-card>

      <el-card class="section" shadow="never">
        <template #header>
          <div class="section-header">
            <span class="section-title">Rules</span>
            <div class="section-actions">
              <span class="section-meta">
                The most specific rule wins, deny wins on equal prefix length.
              </span>
              <span v-if="currentIP" class="section-meta mono">
                Your IP: {{ currentIP }}{{ currentIPLoopback ? ' (local)' : '' }}
              </span>
              <el-dropdown
                trigger="click"
                :disabled="!settings?.enabled || allowingSelf"
                @command="allowMyIP"
              >
                <el-button size="small" :loading="allowingSelf">
                  Allow my IP
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item :command="1">For 1 hour</el-dropdown-item>
                    <el-dropdown-item :command="24">For 24 hours</el-dropdown-item>
                    <el-dropdown-item :command="168">For 7 days</el-dropdown-item>
                    <el-dropdown-item :command="0" divided>
                      Permanently
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </div>
        </template>

        <div class="inline-form rule-form">
          <el-select
            v-model="ruleForm.proxyName"
            class="proxy-select"
            filterable
            allow-create
            default-first-option
          >
            <el-option label="All proxies" :value="GLOBAL_PROXY" />
            <el-option
              v-for="name in proxyNames"
              :key="name"
              :label="name"
              :value="name"
            />
          </el-select>
          <el-select v-model="ruleForm.action" class="action-select">
            <el-option label="Allow" value="allow" />
            <el-option label="Deny" value="deny" />
          </el-select>
          <el-input
            v-model="ruleForm.ip"
            class="ip-input"
            placeholder="IP or CIDR, e.g. 1.2.3.4 or 10.0.0.0/8"
            @keyup.enter="addRule"
          />
          <el-input
            v-model="ruleForm.note"
            class="note-input"
            placeholder="Note (optional)"
            @keyup.enter="addRule"
          />
          <ActionButton size="small" :loading="addingRule" @click="addRule">
            Add
          </ActionButton>
        </div>

        <el-table
          v-loading="rulesLoading"
          :data="rules"
          size="small"
          class="data-table"
          empty-text="No rules yet"
        >
          <el-table-column label="Proxy" min-width="160">
            <template #default="{ row }">
              <span class="mono">{{ row.proxyName }}</span>
              <el-tag v-if="row.proxyName === GLOBAL_PROXY" size="small" class="tag-gap">
                all
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Action" width="100">
            <template #default="{ row }">
              <el-tag
                size="small"
                :type="row.action === 'allow' ? 'success' : 'danger'"
              >
                {{ row.action }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="IP / CIDR" min-width="180">
            <template #default="{ row }">
              <span class="mono">{{ row.ip }}</span>
            </template>
          </el-table-column>
          <el-table-column label="Expires" width="150">
            <template #default="{ row }">
              <span v-if="row.expiresAt > 0" class="mono">
                {{ formatUnixSeconds(row.expiresAt) }}
              </span>
              <span v-else class="muted">permanent</span>
            </template>
          </el-table-column>
          <el-table-column prop="note" label="Note" min-width="140" />
          <el-table-column label="Created" width="150">
            <template #default="{ row }">
              <span class="mono">{{ formatUnixSeconds(row.createdAt) }}</span>
              <el-tag
                v-if="row.source === 'api'"
                size="small"
                type="info"
                class="tag-gap"
              >
                api
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="Actions" width="100" align="right">
            <template #default="{ row }">
              <el-button link type="danger" @click="removeRule(row)">Delete</el-button>
            </template>
          </el-table-column>
        </el-table>

        <div v-if="rulesTotal > rulesPageSize" class="pagination">
          <el-pagination
            :current-page="rulesPage"
            :page-size="rulesPageSize"
            :total="rulesTotal"
            layout="total, prev, pager, next"
            @current-change="onRulesPageChange"
          />
        </div>
      </el-card>

      <el-card class="section" shadow="never">
        <template #header>
          <div class="section-header">
            <span class="section-title">Access records</span>
            <span class="section-meta">
              Memory only, cleared on restart. Kept for
              {{ settings?.retentionHours ?? 0 }}h after the last connection, at
              most {{ settings?.maxRecords ?? 0 }} records.
            </span>
          </div>
        </template>

        <div class="inline-form filter-form">
          <el-select
            v-model="recordFilter.proxyName"
            class="proxy-select"
            clearable
            placeholder="All proxies"
          >
            <el-option
              v-for="name in proxyNames"
              :key="name"
              :label="name"
              :value="name"
            />
          </el-select>
          <el-input
            v-model="recordFilter.ip"
            class="ip-input"
            placeholder="Filter by IP"
            clearable
            @keyup.enter="reloadRecords"
          />
          <el-select v-model="recordFilter.result" class="action-select">
            <el-option label="All results" value="all" />
            <el-option label="Allowed" value="allowed" />
            <el-option label="Denied" value="denied" />
          </el-select>
          <ActionButton variant="outline" size="small" @click="reloadRecords">
            Search
          </ActionButton>
          <ActionButton
            variant="outline"
            size="small"
            :disabled="selectedRecordIds.length === 0"
            danger
            @click="deleteSelectedRecords"
          >
            Delete selected
          </ActionButton>
          <ActionButton variant="outline" size="small" @click="pruneRecords">
            Prune expired
          </ActionButton>
        </div>

        <el-table
          v-loading="recordsLoading"
          :data="records"
          size="small"
          row-key="id"
          class="data-table"
          empty-text="No access records yet"
          @selection-change="onSelectionChange"
        >
          <el-table-column type="selection" width="44" />
          <el-table-column label="Proxy" min-width="140">
            <template #default="{ row }">
              <span class="mono">{{ row.proxyName }}</span>
            </template>
          </el-table-column>
          <el-table-column label="Client IP" min-width="150">
            <template #default="{ row }">
              <span class="mono">{{ row.ip }}</span>
            </template>
          </el-table-column>
          <el-table-column prop="connCount" label="Conns" width="80" />
          <el-table-column label="Last seen" width="150">
            <template #default="{ row }">
              {{ formatUnixSeconds(row.lastSeen) }}
            </template>
          </el-table-column>
          <el-table-column label="Result" width="110">
            <template #default="{ row }">
              <el-tooltip :content="row.reason || ''" placement="top">
                <el-tag size="small" :type="row.allowed ? 'success' : 'danger'">
                  {{ row.allowed ? 'allowed' : 'denied' }}
                </el-tag>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column label="Actions" width="150" align="right">
            <template #default="{ row }">
              <el-button link type="primary" @click="trustRecord(row, 'allow')">
                Allow
              </el-button>
              <el-button link type="danger" @click="trustRecord(row, 'deny')">
                Deny
              </el-button>
            </template>
          </el-table-column>
        </el-table>

        <div v-if="recordsTotal > recordsPageSize" class="pagination">
          <el-pagination
            :current-page="recordsPage"
            :page-size="recordsPageSize"
            :total="recordsTotal"
            layout="total, prev, pager, next"
            @current-change="onRecordsPageChange"
          />
        </div>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ActionButton from '@shared/components/ActionButton.vue'
import {
  allowIPAccessCurrentIP,
  createIPAccessRule,
  deleteIPAccessRecords,
  deleteIPAccessRule,
  getIPAccessCurrentIP,
  getIPAccessRecords,
  getIPAccessRules,
  getIPAccessSettings,
  pruneIPAccessRecords,
  updateIPAccessGlobalMode,
  updateIPAccessProxyMode,
} from '../api/ipaccess'
import { getProxiesV2 } from '../api/proxy'
import { formatUnixSeconds } from '../utils/format'
import type {
  IPAccessMode,
  IPAccessRecord,
  IPAccessRule,
  IPAccessSettings,
} from '../types/ipaccess'

const GLOBAL_PROXY = '*'

type TagType = 'primary' | 'success' | 'info' | 'warning' | 'danger'

const loading = ref(false)
const settings = ref<IPAccessSettings | null>(null)
const proxyNames = ref<string[]>([])

const globalMode = ref<string>('off')
const savingMode = ref(false)
const proxyModeForm = reactive<{ proxyName: string; mode: string }>({
  proxyName: '',
  mode: 'inherit',
})

const addingRule = ref(false)
const rulesLoading = ref(false)
const rules = ref<IPAccessRule[]>([])
const rulesTotal = ref(0)
const rulesPage = ref(1)
const rulesPageSize = 20
const ruleForm = reactive({
  proxyName: GLOBAL_PROXY,
  action: 'allow' as 'allow' | 'deny',
  ip: '',
  note: '',
})

const recordsLoading = ref(false)
const records = ref<IPAccessRecord[]>([])
const recordsTotal = ref(0)
const recordsPage = ref(1)
const recordsPageSize = 20
const recordFilter = reactive<{
  proxyName: string
  ip: string
  result: 'all' | 'allowed' | 'denied'
}>({
  proxyName: '',
  ip: '',
  result: 'all',
})
const selectedRecordIds = ref<number[]>([])

const currentIP = ref('')
const currentIPLoopback = ref(false)
const allowingSelf = ref(false)

const modeHint = computed(() => {
  switch (globalMode.value) {
    case 'allowlist':
      return 'Only the client IPs matched by an allow rule can connect, everything else is rejected.'
    case 'denylist':
      return 'The client IPs matched by a deny rule are rejected, everything else can connect.'
    default:
      return 'Access is not filtered, the client IPs are still recorded below.'
  }
})

const modeTagType = (mode: string): TagType => {
  if (mode === 'allowlist') return 'warning'
  if (mode === 'denylist') return 'danger'
  return 'info'
}

const ensureEnabled = () => {
  if (settings.value?.enabled) return true
  ElMessage.warning('IP access control is not enabled in the frps configuration')
  return false
}

const loadSettings = async () => {
  settings.value = await getIPAccessSettings()
  globalMode.value = settings.value.globalMode
}

const loadProxyNames = async () => {
  try {
    const page = await getProxiesV2({ type: 'tcp', pageSize: 200 })
    proxyNames.value = page.items.map((item) => item.name)
  } catch {
    proxyNames.value = []
  }
}

const loadRules = async () => {
  rulesLoading.value = true
  try {
    const page = await getIPAccessRules({
      page: rulesPage.value,
      pageSize: rulesPageSize,
    })
    rules.value = page.items
    rulesTotal.value = page.total
  } finally {
    rulesLoading.value = false
  }
}

const loadRecords = async () => {
  recordsLoading.value = true
  try {
    const page = await getIPAccessRecords({
      page: recordsPage.value,
      pageSize: recordsPageSize,
      proxyName: recordFilter.proxyName,
      ip: recordFilter.ip,
      result: recordFilter.result,
    })
    records.value = page.items
    recordsTotal.value = page.total
    selectedRecordIds.value = []
  } finally {
    recordsLoading.value = false
  }
}

const loadCurrentIP = async () => {
  try {
    const resp = await getIPAccessCurrentIP()
    currentIP.value = resp.ip
    currentIPLoopback.value = resp.loopback
  } catch {
    currentIP.value = ''
  }
}

// Allow the address this browser is talking from. 0 means the rule never
// expires, which is what "long lived" means here.
const allowMyIP = async (command: string | number | object) => {
  if (!ensureEnabled()) return
  const hours = Number(command)
  if (!Number.isFinite(hours) || hours < 0) return
  allowingSelf.value = true
  try {
    const resp = await allowIPAccessCurrentIP(hours)
    await Promise.all([loadRules(), loadSettings()])
    ElMessage.success(
      resp.expiresAt
        ? `${resp.ip} allowed for ${describeHours(hours)}`
        : `${resp.ip} allowed permanently`,
    )
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to allow this ip')
  } finally {
    allowingSelf.value = false
  }
}

const describeHours = (hours: number) => {
  if (hours % 24 === 0 && hours >= 24) return `${hours / 24} day(s)`
  return `${hours} hour(s)`
}

const refreshAll = async () => {
  loading.value = true
  try {
    await loadSettings()
    await Promise.all([
      loadProxyNames(),
      loadRules(),
      loadRecords(),
      loadCurrentIP(),
    ])
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to load data')
  } finally {
    loading.value = false
  }
}

const onGlobalModeChange = async (value: string | number | boolean | undefined) => {
  if (!ensureEnabled()) {
    globalMode.value = settings.value?.globalMode ?? 'off'
    return
  }
  savingMode.value = true
  try {
    settings.value = await updateIPAccessGlobalMode(value as IPAccessMode)
    globalMode.value = settings.value.globalMode
    ElMessage.success('Global mode updated')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to update mode')
    globalMode.value = settings.value?.globalMode ?? 'off'
  } finally {
    savingMode.value = false
  }
}

const saveProxyMode = async () => {
  if (!ensureEnabled()) return
  const proxyName = proxyModeForm.proxyName.trim()
  if (!proxyName) {
    ElMessage.warning('Proxy name is required')
    return
  }
  savingMode.value = true
  try {
    settings.value = await updateIPAccessProxyMode(
      proxyName,
      proxyModeForm.mode === 'inherit' ? '' : (proxyModeForm.mode as IPAccessMode),
    )
    proxyModeForm.proxyName = ''
    proxyModeForm.mode = 'inherit'
    ElMessage.success('Proxy mode updated')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to update mode')
  } finally {
    savingMode.value = false
  }
}

const removeProxyMode = async (proxyName: string) => {
  if (!ensureEnabled()) return
  try {
    settings.value = await updateIPAccessProxyMode(proxyName, '')
    ElMessage.success(`Proxy ${proxyName} inherits the global mode now`)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to update mode')
  }
}

const addRule = async () => {
  if (!ruleForm.ip.trim()) {
    ElMessage.warning('IP or CIDR is required')
    return
  }
  addingRule.value = true
  try {
    await createIPAccessRule({
      proxyName: ruleForm.proxyName || GLOBAL_PROXY,
      action: ruleForm.action,
      ip: ruleForm.ip.trim(),
      note: ruleForm.note.trim(),
    })
    ruleForm.ip = ''
    ruleForm.note = ''
    await Promise.all([loadRules(), loadSettings()])
    ElMessage.success('Rule added')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to add rule')
  } finally {
    addingRule.value = false
  }
}

const removeRule = async (rule: IPAccessRule) => {
  try {
    await ElMessageBox.confirm(
      `Delete the ${rule.action} rule for ${rule.ip}?`,
      'Delete rule',
      { type: 'warning', confirmButtonText: 'Delete', cancelButtonText: 'Cancel' },
    )
  } catch {
    return
  }
  try {
    await deleteIPAccessRule(rule.id)
    await Promise.all([loadRules(), loadSettings()])
    ElMessage.success('Rule deleted')
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to delete rule')
  }
}

const trustRecord = async (record: IPAccessRecord, action: 'allow' | 'deny') => {
  if (!ensureEnabled()) return
  try {
    await createIPAccessRule({
      proxyName: record.proxyName,
      action,
      ip: record.ip,
      note: 'from access records',
    })
    await Promise.all([loadRules(), loadSettings(), loadRecords()])
    ElMessage.success(`${record.ip} is now in the ${action} list of ${record.proxyName}`)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to create rule')
  }
}

const onSelectionChange = (rows: IPAccessRecord[]) => {
  selectedRecordIds.value = rows.map((row) => row.id)
}

const deleteSelectedRecords = async () => {
  if (selectedRecordIds.value.length === 0) return
  try {
    const resp = await deleteIPAccessRecords(selectedRecordIds.value)
    await Promise.all([loadRecords(), loadSettings()])
    ElMessage.success(`${resp.cleared} records deleted`)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to delete records')
  }
}

const pruneRecords = async () => {
  try {
    const resp = await pruneIPAccessRecords()
    await Promise.all([loadRecords(), loadSettings()])
    ElMessage.success(`${resp.cleared} expired records pruned`)
  } catch (err) {
    ElMessage.error(err instanceof Error ? err.message : 'Failed to prune records')
  }
}

const reloadRecords = () => {
  recordsPage.value = 1
  loadRecords()
}

const onRulesPageChange = (page: number) => {
  rulesPage.value = page
  loadRules()
}

const onRecordsPageChange = (page: number) => {
  recordsPage.value = page
  loadRecords()
}

onMounted(() => {
  refreshAll()
})
</script>

<style scoped lang="scss">
.ipaccess-page {
  display: flex;
  flex-direction: column;
  gap: $spacing-lg;
}

.page-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: $spacing-lg;
}

.title-section {
  min-width: 0;
}

.notice {
  border-radius: $radius-md;
}

.content {
  display: flex;
  flex-direction: column;
  gap: $spacing-lg;
}

.section {
  :deep(.el-card__header) {
    padding: $spacing-md $spacing-lg;
  }

  :deep(.el-card__body) {
    padding: $spacing-lg;
  }
}

.section-header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: $spacing-md;
  flex-wrap: wrap;
}

.section-title {
  font-size: $font-size-md;
  font-weight: $font-weight-semibold;
  color: $color-text-primary;
}

.section-meta {
  font-size: $font-size-sm;
  color: $color-text-muted;
}

.section-actions {
  display: flex;
  align-items: center;
  gap: $spacing-md;
  flex-wrap: wrap;
}

.sub-title {
  font-size: $font-size-sm;
  font-weight: $font-weight-medium;
  color: $color-text-secondary;
}

.hint {
  margin: $spacing-sm 0 0;
  font-size: $font-size-sm;
  color: $color-text-muted;
}

.mode-block {
  margin-bottom: $spacing-xl;
}

.proxy-mode-block {
  padding-top: $spacing-lg;
  border-top: 1px solid $color-border-lighter;
}

.inline-form {
  display: flex;
  align-items: center;
  gap: $spacing-sm;
  flex-wrap: wrap;
  margin: $spacing-md 0;
}

.proxy-select {
  width: 200px;
}

.mode-select,
.action-select {
  width: 140px;
}

.ip-input {
  width: 260px;
}

.note-input {
  width: 200px;
}

.mode-table,
.data-table {
  width: 100%;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: $font-size-sm;
}

.muted {
  color: $color-text-muted;
  font-size: $font-size-sm;
}

.tag-gap {
  margin-left: $spacing-sm;
}

.pagination {
  display: flex;
  justify-content: flex-end;
  margin-top: $spacing-md;
}

@media (max-width: $breakpoint-mobile) {
  .inline-form {
    flex-direction: column;
    align-items: stretch;
  }

  .proxy-select,
  .mode-select,
  .action-select,
  .ip-input,
  .note-input {
    width: 100%;
  }
}
</style>
