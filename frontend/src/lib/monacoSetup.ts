import {loader} from '@monaco-editor/react'
import type {Environment} from 'monaco-editor'
import * as monaco from 'monaco-editor'
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker'

declare global {
    interface Window {
        MonacoEnvironment?: Environment
    }
}

window.MonacoEnvironment = {
    getWorker() {
        return new EditorWorker()
    },
}

loader.config({monaco})
