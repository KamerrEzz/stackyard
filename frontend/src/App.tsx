import {useState} from 'react'
import Sidebar, {type ViewKey} from './components/Sidebar'
import TopBar from './components/TopBar'
import EnvironmentManagerView from './modules/environment-manager/EnvironmentManagerView'
import StatusDashboard from './modules/environment-manager/StatusDashboard'
import DbClientView from './modules/db-client/DbClientView'
import SchemaDiagramView from './modules/schema-diagram/SchemaDiagramView'
import MigrationsView from './modules/migrations/MigrationsView'

const VIEW_TITLES: Record<ViewKey, string> = {
    environments: 'Environment Manager',
    'db-client': 'DB Client',
    status: 'Status Dashboard',
    'schema-diagram': 'Schema Diagram',
    migrations: 'Migrations',
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
                    {activeView === 'schema-diagram' && <SchemaDiagramView/>}
                    {activeView === 'migrations' && <MigrationsView/>}
                    {activeView === 'status' && <StatusDashboard/>}
                </main>
            </div>
        </div>
    )
}

export default App
