import { useRef, useState, useSyncExternalStore } from 'react'
import { ChevronLeft, ChevronRight, Quote, Star, UserRound } from 'lucide-react'
import { motion as Motion } from 'framer-motion'
import { testimonials } from '../data/testimonials'

const SWIPE_THRESHOLD = 48
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

function subscribeReducedMotion(onChange) {
    const query = window.matchMedia?.(REDUCED_MOTION_QUERY)
    query?.addEventListener('change', onChange)
    return () => query?.removeEventListener('change', onChange)
}

function getReducedMotionSnapshot() {
    return window.matchMedia?.(REDUCED_MOTION_QUERY).matches ?? false
}

export default function Testimonials() {
    const [activeIndex, setActiveIndex] = useState(0)
    const pointerStart = useRef(null)
    const prefersReducedMotion = useSyncExternalStore(subscribeReducedMotion, getReducedMotionSnapshot, () => true)

    if (testimonials.length === 0) return null

    const showPrevious = () => {
        setActiveIndex(current => (current - 1 + testimonials.length) % testimonials.length)
    }

    const showNext = () => {
        setActiveIndex(current => (current + 1) % testimonials.length)
    }

    const handleKeyDown = (event) => {
        if (event.key === 'ArrowLeft') {
            event.preventDefault()
            showPrevious()
        } else if (event.key === 'ArrowRight') {
            event.preventDefault()
            showNext()
        }
    }

    const handlePointerDown = (event) => {
        if (event.pointerType === 'mouse' && event.button !== 0) return
        pointerStart.current = { x: event.clientX, y: event.clientY }
    }

    const handlePointerUp = (event) => {
        const start = pointerStart.current
        pointerStart.current = null
        if (!start) return

        const deltaX = event.clientX - start.x
        const deltaY = event.clientY - start.y
        if (Math.abs(deltaX) < SWIPE_THRESHOLD || Math.abs(deltaX) <= Math.abs(deltaY)) return

        if (deltaX < 0) showNext()
        else showPrevious()
    }

    return (
        <section
            id="depoimentos"
            aria-labelledby="testimonials-title"
            aria-roledescription="carrossel"
            tabIndex={0}
            onKeyDown={handleKeyDown}
            className="w-full max-w-3xl rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-4 focus-visible:ring-offset-brand-gunship"
        >
            <header className="mb-6 sm:mb-8">
                <p className="text-xs font-bold uppercase tracking-[0.2em] text-brand-ember">Depoimentos</p>
                <h2 id="testimonials-title" className="mt-3 text-2xl font-bold leading-tight text-brand-lynx sm:text-3xl">
                    Quem Viaja, Recomenda
                </h2>
                <p className="mt-3 text-sm leading-6 text-brand-lynx/70 sm:text-base">
                    Veja o que nossos clientes dizem sobre nós
                </p>
            </header>

            <div
                className="grid touch-pan-y"
                onPointerDown={handlePointerDown}
                onPointerUp={handlePointerUp}
                onPointerCancel={() => { pointerStart.current = null }}
            >
                {testimonials.map((testimonial, index) => {
                    const isActive = index === activeIndex
                    const offset = prefersReducedMotion ? 0 : index < activeIndex ? -16 : 16

                    return (
                        <Motion.article
                            key={testimonial.id}
                            aria-hidden={!isActive}
                            initial={false}
                            animate={{
                                opacity: isActive ? 1 : 0,
                                x: isActive ? 0 : offset,
                            }}
                            transition={{ duration: prefersReducedMotion ? 0 : 0.25, ease: 'easeOut' }}
                            className={`[grid-area:1/1] flex h-full flex-col border border-brand-blue-grey/50 bg-black/10 p-5 sm:p-7 ${isActive ? 'pointer-events-auto' : 'pointer-events-none select-none'}`}
                        >
                            <div className="mb-5 flex items-start justify-between gap-4">
                                <div className="flex gap-1" role="img" aria-label={`${testimonial.rating} de 5 estrelas`}>
                                    {Array.from({ length: testimonial.rating }, (_, starIndex) => (
                                        <Star
                                            key={starIndex}
                                            aria-hidden="true"
                                            className="fill-brand-wasp text-brand-wasp"
                                            size={17}
                                            strokeWidth={1.5}
                                        />
                                    ))}
                                </div>
                                <Quote aria-hidden="true" className="shrink-0 text-brand-lynx/40" size={28} strokeWidth={1.5} />
                            </div>

                            <blockquote className="mb-6 flex-1 text-sm italic leading-7 text-brand-lynx/90 sm:text-base">
                                “{testimonial.text}”
                            </blockquote>

                            <div className="flex items-center gap-4 border-t border-brand-blue-grey/35 pt-5">
                                <span className="flex h-11 w-11 shrink-0 items-center justify-center bg-brand-lynx text-brand-gunship" aria-hidden="true">
                                    <UserRound size={22} strokeWidth={1.8} />
                                </span>
                                <div className="min-w-0">
                                    <p className="font-bold text-brand-lynx">{testimonial.name}</p>
                                    <p className="mt-0.5 text-sm text-brand-lynx/65">
                                        {testimonial.destination} · {testimonial.date}
                                    </p>
                                </div>
                            </div>
                        </Motion.article>
                    )
                })}
            </div>

            <p className="sr-only" aria-live="polite" aria-atomic="true">
                Depoimento {activeIndex + 1} de {testimonials.length}: {testimonials[activeIndex].name}
            </p>

            <div className="mt-6 flex flex-wrap items-center justify-between gap-4">
                <div className="flex items-center gap-2" role="group" aria-label="Navegar pelos depoimentos">
                    <button
                        type="button"
                        onClick={showPrevious}
                        aria-label="Depoimento anterior"
                        className="flex h-11 w-11 items-center justify-center border border-brand-blue-grey/60 text-brand-lynx transition-colors hover:border-brand-lynx hover:bg-brand-lynx/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx"
                    >
                        <ChevronLeft aria-hidden="true" size={20} />
                    </button>
                    <button
                        type="button"
                        onClick={showNext}
                        aria-label="Próximo depoimento"
                        className="flex h-11 w-11 items-center justify-center border border-brand-blue-grey/60 text-brand-lynx transition-colors hover:border-brand-lynx hover:bg-brand-lynx/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx"
                    >
                        <ChevronRight aria-hidden="true" size={20} />
                    </button>
                </div>

                <div className="flex items-center gap-2" role="group" aria-label="Selecionar depoimento">
                    {testimonials.map((testimonial, index) => {
                        const isActive = index === activeIndex
                        return (
                            <button
                                key={testimonial.id}
                                type="button"
                                onClick={() => setActiveIndex(index)}
                                aria-label={`Ir para depoimento ${index + 1} de ${testimonials.length}: ${testimonial.name}`}
                                aria-current={isActive ? 'true' : undefined}
                                className="group flex h-6 w-8 items-center justify-center rounded-full focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-2 focus-visible:ring-offset-brand-gunship"
                            >
                                <span
                                    aria-hidden="true"
                                    className={`h-3 rounded-full transition-[width,background-color] motion-reduce:transition-none ${isActive ? 'w-8 bg-brand-ember' : 'w-3 bg-brand-blue-grey/65 group-hover:bg-brand-lynx'}`}
                                />
                            </button>
                        )
                    })}
                </div>
            </div>
        </section>
    )
}
