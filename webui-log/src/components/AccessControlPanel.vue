<script setup>
import { onMounted, ref } from 'vue'
import { getJSON, postJSON } from '../api/http'
import { clearTopNotice, setError, setSuccess } from '../utils/notice'

const loading = ref(false)
const saving = ref(false)
const mode = ref('hours')
const controlHours = ref('22:00-07:00')
const status = ref(null)
const statusText = ref('未加载')
const apps = ref([])
const usingApps = ref(false)
const appsLoaded = ref(false)

async function load() {
  loading.value = true
  try {
    const s = await getJSON('/plugins/access_control/')
    status.value = s
    mode.value = s.mode || 'hours'
    controlHours.value = s.control_hours || '22:00-07:00'
    statusText.value = '已加载'
  } catch (e) {
    statusText.value = '加载失败: ' + (e?.message || e)
  } finally {
    loading.value = false
  }
}

async function loadApps() {
  if (appsLoaded.value) return
  try {
    const s = await getJSON('/plugins/access_control/apps')
    apps.value = s.apps || []
    usingApps.value = s.using_apps || false
    appsLoaded.value = true
  } catch (e) {
    console.error('apps load failed', e)
  }
}

function appCats() {
  const cats = []
  for (const a of apps.value) {
    if (!cats.includes(a.cat)) cats.push(a.cat)
  }
  return cats
}

function catApps(cat) {
  return apps.value.filter(a => a.cat === cat)
}

async function save() {
  saving.value = true
  try {
    const enabled = apps.value.filter(a => a.enabled).map(a => a.key)
    await postJSON('/plugins/access_control/apps', { enabled_apps: enabled })
    await postJSON('/plugins/access_control/', { mode: mode.value, control_hours: controlHours.value })
    await load()
    setSuccess('访问控制配置已保存并生效')
  } catch (e) {
    setError('保存失败: ' + (e?.message || e))
  } finally {
    saving.value = false
  }
}

const modeOptions = [
  { value: 'hours', label: '定时控制', desc: '仅在下方时段内屏蔽受控应用（推荐）' },
  { value: 'always', label: '全天控制', desc: '任何时候都屏蔽受控应用' },
  { value: 'off', label: '关闭', desc: '不屏蔽受控应用（仅保留系统防误解析规则）' }
]

onMounted(() => { load(); loadApps() })
</script>

<template>
  <div class="page-shell">
    <div class="panel">
      <div class="panel-header">
        <h2>访问控制（家长控制）</h2>
        <span class="panel-status">{{ statusText }}</span>
      </div>

      <div class="form-row" v-if="usingApps">
        <label>受控应用（勾选 = 禁止访问，点击「保存并应用」生效）</label>
        <div v-for="cat in appCats()" :key="cat" class="app-cat">
          <div class="cat-name">{{ cat }}</div>
          <label v-for="a in catApps(cat)" :key="a.key" class="app-item">
            <input type="checkbox" v-model="a.enabled" />
            <span class="app-name">{{ a.name }}</span>
            <span class="app-count">{{ a.count }} 域名</span>
          </label>
        </div>
      </div>

      <div class="form-row">
        <label>控制模式</label>
        <div class="mode-list">
          <label v-for="opt in modeOptions" :key="opt.value" class="mode-item">
            <input type="radio" :value="opt.value" v-model="mode" />
            <div>
              <strong>{{ opt.label }}</strong>
              <span class="mode-desc">{{ opt.desc }}</span>
            </div>
          </label>
        </div>
      </div>

      <div class="form-row" v-if="mode === 'hours'">
        <label>控制时段（跨天可用）</label>
        <input type="text" v-model="controlHours" placeholder="如 22:00-07:00" class="text-input" />
        <span class="hint">格式: 开始-结束，24小时制，例如 22:00-07:00 表示夜间控制</span>
      </div>

      <div class="form-row">
        <label>当前状态</label>
        <div class="status-grid" v-if="status">
          <div class="status-item">
            <span class="k">当前时间</span>
            <span class="v">{{ status.now }}</span>
          </div>
          <div class="status-item">
            <span class="k">处于控制时段</span>
            <span class="v" :class="status.in_hours ? 'ok' : 'off'">{{ status.in_hours ? '是' : '否' }}</span>
          </div>
          <div class="status-item">
            <span class="k">控制生效中</span>
            <span class="v" :class="status.applying ? 'ok' : 'off'">{{ status.applying ? '是' : '否' }}</span>
          </div>
        </div>
      </div>

      <div class="panel-actions">
        <button class="btn-primary" :disabled="saving" @click="save">{{ saving ? '保存中...' : '保存并应用' }}</button>
        <button class="btn-plain" :disabled="loading" @click="load">刷新</button>
      </div>

      <div class="panel-note">
        <p>勾选受控应用后其域名将加入黑名单（随控制模式/时段生效）；自定义域名可在「本地规则 → 黑名单」中添加。</p>
        <p>命令行方式：<code>manage.sh on / off / status</code>，配置文件 <code>/usr/data/recon/manage.conf</code>。</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.panel { padding: 20px; max-width: 720px; }
.panel-header { display: flex; align-items: center; gap: 12px; margin-bottom: 18px; }
.panel-header h2 { margin: 0; font-size: 18px; }
.panel-status { font-size: 12px; color: #888; }
.form-row { margin-bottom: 16px; }
.form-row label { display: block; font-weight: 600; margin-bottom: 6px; font-size: 13px; }
.app-cat { margin-bottom: 10px; }
.cat-name { font-size: 13px; font-weight: 700; color: #555; margin: 8px 0 4px; }
.app-item { display: flex; align-items: center; gap: 8px; padding: 6px 8px; cursor: pointer; font-weight: 400; font-size: 13px; }
.app-item:hover { background: #f7f9fc; }
.app-item input { accent-color: #4a90d9; }
.app-name { min-width: 90px; }
.app-count { font-size: 11px; color: #999; }
.mode-list { display: flex; flex-direction: column; gap: 8px; }
.mode-item { display: flex; gap: 8px; align-items: flex-start; cursor: pointer; padding: 8px 10px; border: 1px solid #ddd; border-radius: 6px; }
.mode-item:has(input:checked) { border-color: #4a90d9; background: #f0f6ff; }
.mode-desc { display: block; font-size: 12px; color: #888; margin-top: 2px; }
.text-input { padding: 8px 10px; border: 1px solid #ccc; border-radius: 6px; width: 180px; font-size: 14px; }
.hint { display: block; font-size: 12px; color: #999; margin-top: 4px; }
.status-grid { display: flex; gap: 24px; }
.status-item .k { font-size: 12px; color: #888; display: block; }
.status-item .v { font-size: 16px; font-weight: 600; }
.status-item .v.ok { color: #2e8b57; }
.status-item .v.off { color: #c0392b; }
.panel-actions { margin-top: 18px; display: flex; gap: 10px; }
.btn-primary { padding: 8px 18px; background: #4a90d9; color: #fff; border: none; border-radius: 6px; cursor: pointer; }
.btn-plain { padding: 8px 14px; background: #eee; border: none; border-radius: 6px; cursor: pointer; }
.panel-note { margin-top: 24px; font-size: 12px; color: #999; border-top: 1px solid #eee; padding-top: 12px; }
.panel-note code { background: #f5f5f5; padding: 1px 5px; border-radius: 3px; }
</style>
