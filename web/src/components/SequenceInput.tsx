import { Input, Alert, Space } from 'antd'
import { useTranslation } from 'react-i18next'
import { MONO_STACK } from '../theme'

const { TextArea } = Input

interface Props {
  value: string
  onChange: (v: string) => void
  error?: string
}

export default function SequenceInput({ value, onChange, error }: Props) {
  const { t } = useTranslation()
  return (
    <Space orientation="vertical" style={{ width: '100%' }}>
      <TextArea
        rows={8}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={'>seq1 Description\nATGCGTACGTAGCTAGCTAGCTAGC\n>seq2 Description\nATGCGTACGTAGCTAGCTAGCTAGC'}
        style={{ fontFamily: MONO_STACK, fontSize: 13 }}
      />
      {error && <Alert title={error} type="error" showIcon />}
      {value && !error && (
        <Alert
          title={t('home.sequenceInput.fastaDetected')}
          type="success"
          showIcon
          style={{ padding: '4px 12px' }}
        />
      )}
    </Space>
  )
}
