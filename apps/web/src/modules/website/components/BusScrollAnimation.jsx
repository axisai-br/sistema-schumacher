import { useRef, useState, useSyncExternalStore } from 'react'
import { motion as Motion, useScroll, useTransform } from 'framer-motion'
import schumacherHorizontalDark from '../../../assets/brand/logos/schumacher-horizontal-dark.svg'

const BUS_SOURCE = '/assets/brand/schumacher-bus-cutout.webp'
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'
const MOBILE_QUERY = '(max-width: 767px)'

// React to preference and breakpoint changes; Motion owns scroll cleanup.
function subscribePresentation(onChange) {
    const queries = [REDUCED_MOTION_QUERY, MOBILE_QUERY].map(query => window.matchMedia?.(query))
    queries.forEach(query => query?.addEventListener('change', onChange))
    return () => queries.forEach(query => query?.removeEventListener('change', onChange))
}

function needsStaticPresentation() {
    return !window.matchMedia || window.matchMedia(REDUCED_MOTION_QUERY).matches
        || window.matchMedia(MOBILE_QUERY).matches
        || !window.CSS?.supports('position', 'sticky')
        || !window.CSS?.supports('height', '100svh')
}

function BusImage({ onError }) {
    return (
        <img
            src={BUS_SOURCE}
            alt=""
            width={1672}
            height={941}
            className="h-auto max-h-[55svh] w-[92vw] max-w-[1200px] object-contain"
            decoding="async"
            loading="lazy"
            onError={onError}
        />
    )
}

function Signature() {
    return (
        <img
            src={schumacherHorizontalDark}
            alt="Schumacher Tur"
            width={1920}
            height={1500}
            className="h-auto w-[92vw] max-w-[1000px] object-contain"
            decoding="async"
        />
    )
}

function JourneyHeading() {
    return (
        <div className="px-5 text-center">
            <h2 className="text-3xl font-bold leading-tight text-brand-lynx sm:text-4xl lg:text-5xl">
                Sua Jornada Começa Aqui
            </h2>
            <p className="mt-3 text-base font-medium text-brand-lynx sm:text-lg">↓ Continue descendo ↓</p>
        </div>
    )
}

function StaticBusSection({ busUnavailable, onBusError }) {
    return (
        <section data-bus-mode="static" className="border-t border-brand-blue-grey/30">
            <div className="flex flex-col items-center gap-8 bg-brand-gunship py-12 sm:py-16">
                <JourneyHeading />
                {!busUnavailable && (
                    <BusImage onError={onBusError} />
                )}
            </div>
            <div className="flex justify-center bg-brand-lynx py-8">
                <Signature />
            </div>
        </section>
    )
}

function AnimatedBusSection({ onBusError }) {
    const sectionRef = useRef(null)
    const { scrollYProgress } = useScroll({
        target: sectionRef,
        // Match the sticky interval below the shared 5rem header.
        offset: ['start 80px', 'end end'],
    })
    const busX = useTransform(scrollYProgress, [0, 0.25, 0.5, 0.75], ['0%', '0%', '35%', '110%'])
    const busOpacity = useTransform(scrollYProgress, [0, 0.25, 0.5, 0.75], [1, 1, 0.55, 0])
    const headingOpacity = useTransform(scrollYProgress, [0, 0.25, 0.45, 0.6], [1, 1, 1, 0])
    const backgroundOpacity = useTransform(scrollYProgress, [0.45, 0.75], [0, 1])
    const signatureOpacity = useTransform(scrollYProgress, [0.65, 0.75, 1], [0, 0.65, 1])
    const signatureScale = useTransform(scrollYProgress, [0.65, 0.75, 1], [0.96, 0.96, 1])

    return (
        <section ref={sectionRef} data-bus-mode="animated" className="relative h-[240svh] bg-brand-gunship">
            <div className="sticky top-20 flex h-[calc(100svh-5rem)] items-center justify-center overflow-hidden">
                <Motion.div
                    data-bus-layer="background"
                    className="pointer-events-none absolute inset-0 bg-brand-lynx"
                    style={{ opacity: backgroundOpacity }}
                    aria-hidden="true"
                />
                <div className="relative flex flex-col items-center gap-6 sm:gap-8">
                    <Motion.div data-bus-layer="heading" style={{ opacity: headingOpacity }}>
                        <JourneyHeading />
                    </Motion.div>
                    <Motion.div data-bus-layer="bus" style={{ x: busX, opacity: busOpacity }}>
                        <BusImage onError={onBusError} />
                    </Motion.div>
                </div>
                <Motion.div
                    data-bus-layer="signature"
                    className="pointer-events-none absolute inset-0 flex items-center justify-center"
                    style={{ opacity: signatureOpacity, scale: signatureScale }}
                >
                    <Signature />
                </Motion.div>
            </div>
        </section>
    )
}

export default function BusScrollAnimation() {
    const isStatic = useSyncExternalStore(subscribePresentation, needsStaticPresentation, () => true)
    const [busUnavailable, setBusUnavailable] = useState(false)
    const handleBusError = () => setBusUnavailable(true)

    if (isStatic || busUnavailable) {
        return <StaticBusSection busUnavailable={busUnavailable} onBusError={handleBusError} />
    }

    return <AnimatedBusSection onBusError={handleBusError} />
}
