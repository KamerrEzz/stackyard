import {useState} from 'react'
import Sidebar, {type ViewKey} from './components/Sidebar'
import TopBar from './components/TopBar'
import EnvironmentManagerView from './modules/environment-manager/EnvironmentManagerView'
import StatusDashboard from './modules/environment-manager/StatusDashboard'
import DbClientView from './modules/db-client/DbClientView'

const VIEW_TITLES: Record<ViewKey, string> = {
    environments: 'Environment Manager',
    'db-client': 'DB Client',
    status: 'Status Dashboard',
}

function App() {
    const [activeView, setActiveView] = useState<ViewKey>('environments')

    return (
        <div className="flex h-screen w-screen overflow-hidden">
            <Sidebar activeView={activeView} onSelectView={setActiveView}/>
            <div className="flex flex-1 flex-col overflow-hidden">
                <TopBar subtitle={VIEW_TITLES[activeView]}/>
                <main className="flex-1 overflow-auto p-6">
                    {activeView === 'environments' && <EnvironmentManagerView/>}
                    {activeView === 'db-client' && <DbClientView/>}
                    {activeView === 'status' && <StatusDashboard/>}
                </main>
            </div>
        </div>
    )
}

export default App
