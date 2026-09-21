import Hero from '../components/Hero'
import BusScrollAnimation from '../components/BusScrollAnimation'
import FeaturedServices from '../components/FeaturedServices'
import Services from '../components/Services'
import Fleet from '../components/Fleet'
import ServiceValues from '../components/ServiceValues'
import FinalCTA from '../components/FinalCTA'
import About from '../components/About'

export default function Home() {
    return (
        <>
            <Hero />
            <BusScrollAnimation />
            <FeaturedServices />
            <Services />
            <Fleet />
            <ServiceValues />
            <FinalCTA />
            <About />
        </>
    )
}
