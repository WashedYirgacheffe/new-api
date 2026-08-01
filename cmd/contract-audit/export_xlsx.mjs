import fs from 'node:fs/promises'
import path from 'node:path'
import { SpreadsheetFile, Workbook } from '@oai/artifact-tool'

const root = process.argv[2] || process.cwd()
const outputDir = process.argv[3] || path.join(root, 'outputs')
const catalogDir = path.join(root, 'docs', 'catalog')

const channels = [
  { key: 'reapi', title: 'RE', file: 'reapi-published-contracts.json' },
  { key: 'deepwl', title: 'DeepWL', file: 'deepwl-published-contracts.json' },
  { key: 'nodyhub', title: 'NodyHub', file: 'nodyhub-published-contracts.json' },
]

const columns = [
  ['channel', '渠道'],
  ['model_name', '模型身份'],
  ['gateway_model_id', '上游模型 ID'],
  ['operation', '操作'],
  ['endpoint_type', '端点类型'],
  ['profile_key', 'Profile Key'],
  ['profile_version', 'Profile 版本'],
  ['contract_version', '合同版本'],
  ['contract_hash', '合同 Hash'],
  ['audit_status', '审计状态'],
  ['input_fields', '输入字段'],
  ['material_slots', '素材槽位'],
  ['request_adapter', '请求适配器'],
  ['dispatch_path', '提交路径'],
  ['poll_path', '轮询路径'],
  ['pricing_basis', '价格依据'],
  ['source_urls', '证据 URL'],
  ['difference_summary', '差异摘要'],
]

function jsonText(value) {
  if (value == null) return ''
  if (typeof value === 'string') return value
  return JSON.stringify(value)
}

function schemaFields(schema) {
  return Object.keys(schema?.properties || {}).sort().join(', ')
}

function materialSlots(schema) {
  return Object.keys(schema || {}).sort().join(', ')
}

function rowFor(channel, model) {
  return {
    channel: channel.title,
    model_name: model.model_name || '',
    gateway_model_id: model.gateway_model_id || '',
    operation: model.operation || '',
    endpoint_type: model.endpoint_type || '',
    profile_key: model.profile_key || '',
    profile_version: model.profile_version ?? '',
    contract_version: model.contract_version ?? '',
    contract_hash: model.contract_hash || '',
    audit_status: model.audit_status || '',
    input_fields: schemaFields(model.input_schema),
    material_slots: materialSlots(model.material_schema),
    request_adapter: model.request_contract?.adapter || '',
    dispatch_path: model.dispatch_path || '',
    poll_path: model.poll_path || '',
    pricing_basis: model.pricing_rule?.basis || model.pricing_rule?.pricing_basis || '',
    source_urls: (model.evidence_urls || []).join(' | '),
    difference_summary: (model.differences || []).join('；'),
  }
}

function columnName(index) {
  let value = index + 1
  let name = ''
  while (value > 0) {
    const remainder = (value - 1) % 26
    name = String.fromCharCode(65 + remainder) + name
    value = Math.floor((value - 1) / 26)
  }
  return name
}

function setColumnWidths(sheet, count) {
  for (let index = 0; index < count; index += 1) {
    const width = index === 17 ? 72 : index === 16 ? 52 : index === 10 || index === 11 ? 32 : 22
    sheet.getRange(`${columnName(index)}:${columnName(index)}`).format.columnWidth = width
  }
}

async function loadSnapshot(channel) {
  const source = await fs.readFile(path.join(catalogDir, channel.file), 'utf8')
  return JSON.parse(source)
}

const snapshots = await Promise.all(channels.map(loadSnapshot))
const workbook = Workbook.create()
const summary = workbook.worksheets.add('Summary')
summary.showGridLines = false
summary.getRange('A1:D1').merge()
summary.getRange('A1').values = [['CarLab 上游合同全量审计（文档证据版）']]
summary.getRange('A1:D1').format = { fill: '#1F2937', font: { bold: true, color: '#FFFFFF', size: 14 } }
summary.getRange('A3:D3').values = [['渠道', '已发布计数', '审计行数', '状态说明']]
summary.getRange('A3:D3').format = { fill: '#E5E7EB', font: { bold: true, color: '#111827' } }
summary.getRange('A4:D6').values = snapshots.map((snapshot) => [
  snapshot.channel,
  snapshot.published_model_count,
  snapshot.models.length,
  snapshot.channel === 're'
    ? 'RE 88 个合同与价格 SKU 已有模型级证据'
    : snapshot.channel === 'deepwl'
      ? 'DeepWL 公开价格候选已抓取；CarLab 67 个发布身份待 token-scoped 导出映射'
      : 'NodyHub 权威 Markdown 已覆盖 19 个模型章节；剩余 707 个模型逐项合同/价格待导出',
])
summary.getRange('A8:B8').values = [['审计规则', '具体模型 API 文档 > OpenAPI > 渠道通用文档 > CarLab 合同 > TapLater 覆盖']]
summary.getRange('A9:B9').values = [['烟测边界', '本轮不执行付费调用；新合同只标记 documented 或 fail-closed 状态']]
summary.getRange('A8:B9').format.wrapText = true
summary.getRange('A1:D9').format.borders = { preset: 'outside', style: 'thin', color: '#D1D5DB' }
summary.getRange('A1:D9').format.rowHeight = 24
summary.getRange('A1:D9').format.columnWidth = 30
summary.getRange('A1:D1').format.rowHeight = 30
summary.freezePanes.freezeRows(3)

for (let index = 0; index < channels.length; index += 1) {
  const channel = channels[index]
  const snapshot = snapshots[index]
  const sheet = workbook.worksheets.add(channel.title)
  sheet.showGridLines = false
  const headerValues = columns.map(([, label]) => label)
  const rows = snapshot.models.map((model) => {
    const row = rowFor(channel, model)
    return columns.map(([key]) => row[key] ?? '')
  })
  const endColumn = columnName(columns.length - 1)
  sheet.getRange(`A1:${endColumn}1`).values = [headerValues]
  if (rows.length > 0) sheet.getRange(`A2:${endColumn}${rows.length + 1}`).values = rows
  sheet.getRange(`A1:${endColumn}1`).format = { fill: '#111827', font: { bold: true, color: '#FFFFFF' }, wrapText: true }
  if (rows.length > 0) sheet.getRange(`A2:${endColumn}${rows.length + 1}`).format.wrapText = true
  sheet.getRange(`A1:${endColumn}${rows.length + 1}`).format.borders = { preset: 'inside', style: 'thin', color: '#E5E7EB' }
  sheet.getRange(`A1:${endColumn}${rows.length + 1}`).format.rowHeight = 32
  setColumnWidths(sheet, columns.length)
  sheet.tables.add(`A1:${endColumn}${rows.length + 1}`, true)
  sheet.freezePanes.freezeRows(1)
  sheet.getRange(`A1:${endColumn}${rows.length + 1}`).conditionalFormats.add('containsText', {
    text: 'missing_documentation',
    format: { fill: '#FEF3C7', font: { color: '#92400E' } },
  })
}

await fs.mkdir(outputDir, { recursive: true })
const check = await workbook.inspect({ kind: 'table', range: 'Summary!A1:D9', include: 'values,formulas', tableMaxRows: 12, tableMaxCols: 6 })
console.log(check.ndjson)
const errors = await workbook.inspect({ kind: 'match', searchTerm: '#REF!|#DIV/0!|#VALUE!|#NAME\\?|#N/A', options: { useRegex: true, maxResults: 300 }, summary: 'formula error scan' })
console.log(errors.ndjson)
for (const channel of channels) {
  const preview = await workbook.render({ sheetName: channel.title, range: 'A1:R12', scale: 1, format: 'png' })
  const bytes = new Uint8Array(await preview.arrayBuffer())
  await fs.writeFile(path.join(outputDir, `${channel.key}-audit-preview.png`), bytes)
}
const output = await SpreadsheetFile.exportXlsx(workbook)
await output.save(path.join(outputDir, 'carlab-upstream-contract-audit-2026-08-01.xlsx'))
console.log(`saved ${path.join(outputDir, 'carlab-upstream-contract-audit-2026-08-01.xlsx')}`)
