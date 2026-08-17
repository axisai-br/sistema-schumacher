import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { MotionConfig } from 'framer-motion'
import Layout from './layout/Layout'
import Home from './pages/Home'
import TripMaranhao from './pages/TripMaranhao'
import TripSantaCatarina from './pages/TripSantaCatarina'
import QuotePage from './pages/QuotePage'

function App() {
  return (
    <MotionConfig reducedMotion="user">
      <BrowserRouter>
        <Routes>
          <Route path="/" element={<Layout />}>
            <Route index element={<Home />} />
            <Route path="viagens">
              <Route path="maranhao" element={<TripMaranhao />} />
              <Route path="santa-catarina" element={<TripSantaCatarina />} />
            </Route>
            <Route path="orcamento" element={<QuotePage />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </MotionConfig>
  )
}

export default App
