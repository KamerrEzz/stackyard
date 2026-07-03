import {useState} from 'react'
import {Ping} from '../../wailsjs/go/main/App'

function PingCheck() {
    const [result, setResult] = useState<string | null>(null)

    const handlePing = async () => {
        const response = await Ping()
        setResult(response)
    }

    return (
        <button
            type="button"
            onClick={handlePing}
            className="rounded border border-ink-700 px-2 py-1 font-mono text-xs text-ink-300 transition-colors hover:border-brass-500 hover:text-brass-400"
        >
            {result ? `pong: ${result}` : 'Ping backend'}
        </button>
    )
}

export default PingCheck
