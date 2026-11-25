/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import {
  AlertTriangle,
  ChevronDown,
  ChevronUp,
  Plus,
  Trash2,
} from 'lucide-react'
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'

import { EmptyState } from '@/components/empty-state'
import { StatusBadge } from '@/components/status-badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Combobox } from '@/components/ui/combobox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

import { parseNestedRatioMap } from '../utils/json-parser'

const sectionCardClassName = 'min-w-0 shadow-none'
const sectionHeaderClassName = 'gap-2 border-b'

// Model keys are the billing model name; "*" matches every model.
const MODEL_RATIO_WILDCARD = '*'

export type ModelRatioSubjectKind = 'user_id' | 'group'

type ModelRatioRule = {
  _id: string
  subject: string
  model: string
  ratio: string
}

let modelRatioIdCounter = 0
function createModelRatioId() {
  modelRatioIdCounter += 1
  return `gmr_${modelRatioIdCounter}`
}

function parseRules(
  value: string,
  subjectKind: ModelRatioSubjectKind
): ModelRatioRule[] {
  const map = parseNestedRatioMap(value)
  const rules: ModelRatioRule[] = []
  for (const [subject, entries] of Object.entries(map)) {
    if (
      typeof entries !== 'object' ||
      entries === null ||
      Array.isArray(entries)
    ) {
      continue
    }
    for (const [model, ratio] of Object.entries(entries)) {
      const parsed = Number(ratio)
      if (!Number.isFinite(parsed) || parsed < 0) continue
      if (subjectKind === 'user_id' && !isUserId(subject)) continue
      rules.push({
        _id: createModelRatioId(),
        subject,
        model,
        ratio: String(parsed),
      })
    }
  }
  return rules
}

function serializeRules(rules: ModelRatioRule[]): string {
  const result: Record<string, Record<string, number>> = {}
  for (const rule of rules) {
    const subject = rule.subject.trim()
    const model = rule.model.trim()
    const ratio = Number.parseFloat(rule.ratio)
    if (!subject || !model) continue
    if (!Number.isFinite(ratio) || ratio < 0) continue
    if (!result[subject]) result[subject] = {}
    result[subject][model] = ratio
  }
  return Object.keys(result).length === 0
    ? '{}'
    : JSON.stringify(result, null, 2)
}

function isUserId(value: string): boolean {
  return /^\d+$/.test(value.trim()) && Number.parseInt(value, 10) > 0
}

function groupRules(rules: ModelRatioRule[]) {
  const map: Record<string, ModelRatioRule[]> = {}
  const order: string[] = []
  for (const rule of rules) {
    if (!rule.subject) continue
    if (!map[rule.subject]) {
      map[rule.subject] = []
      order.push(rule.subject)
    }
    map[rule.subject].push(rule)
  }
  return order.map((subject) => ({ subject, items: map[subject] }))
}

type SubjectModelSelectProps = {
  id: string
  subjectKind: ModelRatioSubjectKind
  groupOptions: string[]
  value: string
  placeholder: string
  onValueChange: (value: string) => void
}

// A subject is either a numeric customer id or a group name; group names may be
// typed by hand because new groups can be configured before they appear.
function SubjectModelSelect(props: SubjectModelSelectProps) {
  const options = useMemo(() => {
    if (props.subjectKind === 'user_id') return []
    if (props.value && !props.groupOptions.includes(props.value)) {
      return [props.value, ...props.groupOptions]
    }
    return props.groupOptions
  }, [props.groupOptions, props.subjectKind, props.value])

  if (props.subjectKind === 'user_id') {
    return (
      <Input
        id={props.id}
        className='min-w-0'
        type='number'
        min={1}
        step={1}
        value={props.value}
        aria-label={props.placeholder}
        placeholder={props.placeholder}
        onChange={(e) => props.onValueChange(e.target.value)}
      />
    )
  }

  return (
    <Combobox
      id={props.id}
      options={options.map((name) => ({ value: name, label: name }))}
      value={props.value}
      allowCustomValue
      onValueChange={(value) => {
        if (value) props.onValueChange(value)
      }}
      className='min-w-0'
      placeholder={props.placeholder}
      aria-label={props.placeholder}
    />
  )
}

type ModelRatioSectionProps = {
  subject: string
  subjectKind: ModelRatioSubjectKind
  items: ModelRatioRule[]
  modelOptions: string[]
  groupOptions: string[]
  onUpdate: (id: string, field: 'model' | 'ratio', value: string) => void
  onRemove: (id: string) => void
  onRemoveSubject: (subject: string) => void
  onAddRule: (subject: string) => void
}

function ModelRatioSubjectSection(props: ModelRatioSectionProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(true)
  const isKnownSubject =
    props.subjectKind === 'user_id'
      ? isUserId(props.subject)
      : props.groupOptions.includes(props.subject)

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className='rounded-lg border'>
        <div className='flex items-center justify-between gap-2 p-3'>
          <CollapsibleTrigger
            render={
              <Button
                variant='ghost'
                className='h-auto min-w-0 justify-start whitespace-normal'
              />
            }
            aria-label={t('Rules for {{subject}}', { subject: props.subject })}
          >
            {open ? (
              <ChevronUp className='h-4 w-4' />
            ) : (
              <ChevronDown className='h-4 w-4' />
            )}
            <span className='min-w-0 truncate font-semibold'>
              {props.subject}
            </span>
            {!isKnownSubject && (
              <StatusBadge variant='danger' copyable={false}>
                <AlertTriangle className='mr-1 h-3 w-3' />
                {props.subjectKind === 'user_id'
                  ? t('Invalid user ID')
                  : t('Not in pricing table')}
              </StatusBadge>
            )}
            <StatusBadge variant='neutral' copyable={false}>
              {props.items.length} {t('rules')}
            </StatusBadge>
          </CollapsibleTrigger>
          <div className='flex shrink-0 items-center gap-1'>
            <Button
              variant='ghost'
              size='sm'
              className='h-7 w-7 p-0'
              aria-label={t('Add model ratio rule')}
              onClick={() => {
                props.onAddRule(props.subject)
                setOpen(true)
              }}
            >
              <Plus className='h-4 w-4' />
            </Button>
            <Button
              variant='ghost'
              size='sm'
              className='text-destructive h-7 w-7 p-0'
              aria-label={t('Remove {{subject}}', { subject: props.subject })}
              onClick={() => props.onRemoveSubject(props.subject)}
            >
              <Trash2 className='h-4 w-4' />
            </Button>
          </div>
        </div>
        <CollapsibleContent>
          <div className='space-y-2 border-t p-3'>
            {props.items.length === 0 && (
              <p className='text-muted-foreground text-sm'>
                {t('No rules yet. Add a model to override the group ratio.')}
              </p>
            )}
            {props.items.map((rule) => (
              <div
                key={rule._id}
                className='grid min-w-0 grid-cols-[minmax(0,1fr)_auto] items-center gap-2 rounded-lg border p-3 sm:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto] sm:border-0 sm:p-0'
              >
                <Combobox
                  options={[
                    {
                      value: MODEL_RATIO_WILDCARD,
                      label: MODEL_RATIO_WILDCARD,
                      description: t('Matches every model in this scope'),
                    },
                    ...props.modelOptions.map((name) => ({
                      value: name,
                      label: name,
                    })),
                  ]}
                  value={rule.model}
                  allowCustomValue
                  onValueChange={(value) =>
                    value && props.onUpdate(rule._id, 'model', value)
                  }
                  className='min-w-0'
                  placeholder={t('Model name')}
                  aria-label={t('Model name')}
                />
                <div className='col-start-1 row-start-2 flex min-w-0 items-center gap-1.5 sm:col-start-auto sm:row-start-auto'>
                  <Input
                    className='min-w-0'
                    inputMode='decimal'
                    aria-label={t('Group ratio')}
                    placeholder='0.8'
                    value={rule.ratio}
                    onChange={(e) => {
                      const next = e.target.value
                      if (next === '' || /^\d*\.?\d*$/.test(next)) {
                        props.onUpdate(rule._id, 'ratio', next)
                      }
                    }}
                  />
                  {rule.model && rule.model !== MODEL_RATIO_WILDCARD && (
                    <StatusBadge variant='neutral' copyable={false}>
                      {t('Exact match')}
                    </StatusBadge>
                  )}
                </div>
                <Button
                  variant='ghost'
                  size='sm'
                  className='text-destructive col-start-2 row-start-1 size-8 p-0 sm:col-start-auto sm:row-start-auto'
                  aria-label={t('Delete rule')}
                  onClick={() => props.onRemove(rule._id)}
                >
                  <Trash2 className='h-4 w-4' />
                </Button>
              </div>
            ))}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

type ModelRatioLevelProps = {
  field: string
  title: string
  description: string
  subjectLabel: string
  addLabel: string
  subjectPlaceholder: string
  subjectKind: ModelRatioSubjectKind
  value: string
  groupOptions: string[]
  modelOptions: string[]
  onChange: (field: string, value: string) => void
}

function ModelRatioLevel(props: ModelRatioLevelProps) {
  const { t } = useTranslation()
  const [rules, setRules] = useState<ModelRatioRule[]>(() =>
    parseRules(props.value, props.subjectKind)
  )
  const [newSubject, setNewSubject] = useState('')

  const { field, onChange, subjectKind } = props
  const emitChange = useCallback(
    (next: ModelRatioRule[]) => {
      setRules(next)
      onChange(field, serializeRules(next))
    },
    [field, onChange]
  )

  // The same value is editable from the JSON tab, so re-sync whenever the
  // incoming configuration no longer matches what this editor produced.
  useEffect(() => {
    const next = parseRules(props.value, subjectKind)
    setRules((prev) =>
      serializeRules(prev) === serializeRules(next) ? prev : next
    )
  }, [props.value, subjectKind])

  const updateRule = useCallback(
    (id: string, key: 'model' | 'ratio', value: string) =>
      emitChange(
        rules.map((rule) =>
          rule._id === id ? { ...rule, [key]: value } : rule
        )
      ),
    [rules, emitChange]
  )

  const removeRule = useCallback(
    (id: string) => emitChange(rules.filter((rule) => rule._id !== id)),
    [rules, emitChange]
  )

  const removeSubject = useCallback(
    (subject: string) =>
      emitChange(rules.filter((rule) => rule.subject !== subject)),
    [rules, emitChange]
  )

  const addRule = useCallback(
    (subject: string) =>
      emitChange([
        ...rules,
        { _id: createModelRatioId(), subject, model: '', ratio: '' },
      ]),
    [rules, emitChange]
  )

  const grouped = useMemo(() => groupRules(rules), [rules])

  const subjectCandidates = useMemo(() => {
    if (subjectKind === 'user_id') return []
    const used = new Set(grouped.map((group) => group.subject))
    return props.groupOptions.filter((name) => !used.has(name))
  }, [grouped, props.groupOptions, subjectKind])

  const canAddSubject =
    subjectKind === 'user_id'
      ? isUserId(newSubject) &&
        !grouped.some((group) => group.subject === newSubject.trim())
      : newSubject.trim() !== '' &&
        !grouped.some((group) => group.subject === newSubject.trim())

  const handleAddSubject = useCallback(() => {
    const subject = newSubject.trim()
    if (!subject || !canAddSubject) return
    emitChange([
      ...rules,
      { _id: createModelRatioId(), subject, model: '', ratio: '' },
    ])
    setNewSubject('')
  }, [canAddSubject, emitChange, newSubject, rules])

  return (
    <Card className={sectionCardClassName}>
      <CardHeader className={sectionHeaderClassName}>
        <CardTitle>{props.title}</CardTitle>
        <CardDescription>{props.description}</CardDescription>
      </CardHeader>
      <CardContent>
        <div className='space-y-3'>
          {grouped.length === 0 ? (
            <EmptyState
              className='min-h-32'
              title={t('No rules yet. Add a {{label}} below to get started.', {
                label: props.subjectLabel.toLowerCase(),
              })}
            />
          ) : (
            grouped.map((group) => (
              <ModelRatioSubjectSection
                key={group.subject}
                subject={group.subject}
                subjectKind={subjectKind}
                items={group.items}
                modelOptions={props.modelOptions}
                groupOptions={props.groupOptions}
                onUpdate={updateRule}
                onRemove={removeRule}
                onRemoveSubject={removeSubject}
                onAddRule={addRule}
              />
            ))
          )}

          <div className='flex flex-col gap-2 pt-2 sm:flex-row sm:items-end'>
            <div className='min-w-0 flex-1 space-y-1.5'>
              <Label htmlFor={`${field}-subject`}>{props.subjectLabel}</Label>
              <SubjectModelSelect
                id={`${field}-subject`}
                subjectKind={subjectKind}
                groupOptions={subjectCandidates}
                value={newSubject}
                placeholder={props.subjectPlaceholder}
                onValueChange={setNewSubject}
              />
            </div>
            <Button
              className='shrink-0'
              variant='outline'
              size='sm'
              aria-label={props.addLabel}
              disabled={!canAddSubject}
              onClick={handleAddSubject}
            >
              <Plus className='mr-2 h-4 w-4' />
              {t('Add')}
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  )
}

type GroupModelRatioRulesEditorProps = {
  userModelRatio: string
  userGroupModelRatio: string
  groupModelRatio: string
  groupOptions: string[]
  modelOptions: string[]
  onChange: (field: string, value: string) => void
}

export function GroupModelRatioRulesEditor(
  props: GroupModelRatioRulesEditorProps
) {
  const { t } = useTranslation()
  const modelOptions = useMemo(
    () =>
      props.modelOptions.filter((name) => name !== MODEL_RATIO_WILDCARD).sort(),
    [props.modelOptions]
  )

  return (
    <div className='space-y-4'>
      <Card className={sectionCardClassName}>
        <CardContent className='pt-6'>
          <p className='text-muted-foreground text-sm'>
            {t(
              'A matched value becomes the effective group ratio for that model, replacing the group ratio. The first table that matches wins: customer, then customer group, then billing group.'
            )}
          </p>
        </CardContent>
      </Card>
      <ModelRatioLevel
        field='UserModelRatio'
        title={t('Customer model ratios')}
        description={t(
          'Ratio overrides for a single customer, identified by user ID.'
        )}
        subjectLabel={t('User ID')}
        addLabel={t('Add customer')}
        subjectPlaceholder={t('Enter a user ID')}
        subjectKind='user_id'
        value={props.userModelRatio}
        groupOptions={props.groupOptions}
        modelOptions={modelOptions}
        onChange={props.onChange}
      />
      <ModelRatioLevel
        field='UserGroupModelRatio'
        title={t('Customer group model ratios')}
        description={t(
          'Ratio overrides for every customer inside a user group.'
        )}
        subjectLabel={t('User group')}
        addLabel={t('Add customer group')}
        subjectPlaceholder={t('Select or type a user group')}
        subjectKind='group'
        value={props.userGroupModelRatio}
        groupOptions={props.groupOptions}
        modelOptions={modelOptions}
        onChange={props.onChange}
      />
      <ModelRatioLevel
        field='GroupModelRatio'
        title={t('Billing group model ratios')}
        description={t(
          'Ratio overrides for a billing group, applying to every customer using it.'
        )}
        subjectLabel={t('Billing group')}
        addLabel={t('Add billing group')}
        subjectPlaceholder={t('Select or type a billing group')}
        subjectKind='group'
        value={props.groupModelRatio}
        groupOptions={props.groupOptions}
        modelOptions={modelOptions}
        onChange={props.onChange}
      />
    </div>
  )
}
