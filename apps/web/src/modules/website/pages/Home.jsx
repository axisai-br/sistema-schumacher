import Hero from '../components/Hero'
import BusScrollAnimation from '../components/BusScrollAnimation'
import About from '../components/About'
import Services from '../components/Services'
import Fleet from '../components/Fleet'
import FinalCTA from '../components/FinalCTA'

export default function Home() {
    return (
        <>
            <Hero />
            <BusScrollAnimation />
            <About />
            <Services />
            <Fleet />
            <FinalCTA />
        </>
    )
}
