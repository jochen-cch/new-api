import fs from 'node:fs/promises'

const zh = JSON.parse(await fs.readFile('src/i18n/locales/zh.json', 'utf8'))
  .translation
const keys = [
  'Billing',
  'Model Pricing',
  'Group Pricing',
  'Group Settings',
  'Group ratios',
  'Model ratios',
  'Special ratio rules',
  'Pricing groups',
  'Group visibility',
  'Auto group order',
  'Customer model ratios',
  'Customer group model ratios',
  'Billing group model ratios',
  'Visual editor',
  'JSON editor',
  'Settings',
]
for (const k of keys) console.log(`${k} => ${JSON.stringify(zh[k])}`)
