import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom'
import AppLayout from './components/AppLayout'
import BlastPage from './pages/BlastPage'
import TranscriptPage from './pages/TranscriptPage'
import SettingsPage from './pages/SettingsPage'

function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<AppLayout />}>
          <Route index element={<Navigate to="/blast" replace />} />
          <Route path="blast" element={<BlastPage />} />
          <Route path="transcript" element={<TranscriptPage />} />
          <Route path="settings" element={<SettingsPage />} />
          <Route path="*" element={<Navigate to="/blast" replace />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}

export default App
