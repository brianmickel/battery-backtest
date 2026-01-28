import { useState } from 'react';
import JsonView from '@uiw/react-json-view';

interface ResponseViewerProps {
  data: any;
}

export function ResponseViewer({ data }: ResponseViewerProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = () => {
    navigator.clipboard.writeText(JSON.stringify(data, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="card">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1em' }}>
        <h2 style={{ margin: 0 }}>Response JSON</h2>
        <button onClick={handleCopy} style={{ padding: '0.5em 1em' }}>
          {copied ? '✓ Copied!' : 'Copy to Clipboard'}
        </button>
      </div>
      <div style={{ 
        backgroundColor: '#0a0a0a', 
        padding: '1em', 
        borderRadius: '4px',
        overflow: 'auto',
        maxHeight: '600px'
      }}>
        <JsonView
          value={data}
          style={{ 
            fontFamily: 'monospace', 
            fontSize: '0.9em',
            backgroundColor: '#0a0a0a',
            color: '#f8f8f2'
          }}
          collapsed={2}
          displayDataTypes={false}
          displayObjectSize={false}
        />
      </div>
    </div>
  );
}
