import React, { useState } from 'react';
import Box from '@mui/material/Box';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import { useTheme } from '@mui/material/styles';
import CodeMirror from '@uiw/react-codemirror';
import { json as jsonMode } from '@codemirror/lang-json';
import { oneDark } from '@codemirror/theme-one-dark';
import { RateLimitDefinition } from '@authproxy/api';
import RateLimitDefinitionForm from './RateLimitDefinitionForm';

interface Props {
    value: RateLimitDefinition;
    onChange: (next: RateLimitDefinition) => void;
}

// Tabbed editor: structured form (default) + read-only JSON preview using
// the same CodeMirror + one-dark setup that ConnectorVersionDetail uses.
// Keeping the JSON view read-only sidesteps the round-trip headaches of
// parsing arbitrary edits back into the structured form.
export default function RateLimitDefinitionEditor({ value, onChange }: Props) {
    const theme = useTheme();
    const [tab, setTab] = useState<'form' | 'json'>('form');

    return (
        <Box>
            <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ mb: 2 }}>
                <Tab value="form" label="Form" />
                <Tab value="json" label="JSON preview" />
            </Tabs>

            {tab === 'form' && (
                <RateLimitDefinitionForm value={value} onChange={onChange} />
            )}

            {tab === 'json' && (
                <Box sx={{
                    border: '1px solid',
                    borderColor: 'divider',
                    borderRadius: 1,
                    overflow: 'hidden',
                }}>
                    <CodeMirror
                        value={JSON.stringify(value, null, 2)}
                        theme={theme.palette.mode === 'dark' ? oneDark : undefined}
                        extensions={[jsonMode()]}
                        editable={false}
                        basicSetup={{ lineNumbers: true, foldGutter: true, highlightActiveLine: false }}
                    />
                </Box>
            )}
        </Box>
    );
}
