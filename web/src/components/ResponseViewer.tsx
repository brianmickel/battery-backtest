import JsonView from '@uiw/react-json-view';
import { useClipboard } from '../hooks/useClipboard';

interface ResponseViewerProps {
  data: object;
}

export function ResponseViewer({ data }: ResponseViewerProps) {
  const { copy, messageFor } = useClipboard();
  const json = JSON.stringify(data, null, 2);

  return (
    <div className="card">
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '1em' }}>
        <h2 style={{ margin: 0 }}>Response JSON</h2>
        <button type="button" onClick={() => void copy(json)} style={{ padding: '0.5em 1em' }}>
          Copy response JSON
        </button>
      </div>
      <p role="status">{messageFor(json)}</p>
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
