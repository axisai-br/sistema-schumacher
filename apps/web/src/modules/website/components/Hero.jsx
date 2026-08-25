import { Button } from '@heroui/react'
import { ArrowDown, BusFront, Check, MessageCircle } from 'lucide-react'
import { motion as Motion } from 'framer-motion'

const WHATSAPP_NUMBER = '5549999862222'
const WHATSAPP_MESSAGE = 'Olá! Gostaria de informações sobre viagens com a Schumacher Tur'

const trustItems = [
    '+10 anos no mercado',
    'Frota 100% revisada',
    'Seguro incluso',
]

export default function Hero() {
    const handleWhatsApp = () => {
        window.open(`https://wa.me/${WHATSAPP_NUMBER}?text=${encodeURIComponent(WHATSAPP_MESSAGE)}`, '_blank')
    }

    const scrollToFleet = () => {
        const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
        document.getElementById('frota')?.scrollIntoView({ behavior })
    }

    return (
        <section className="relative z-10 min-h-[calc(100svh-5rem)] overflow-hidden bg-brand-gunship">
            <div className="mx-auto grid min-h-[calc(100svh-5rem)] w-full max-w-[90rem] lg:grid-cols-12">
                <div className="flex min-w-0 items-center px-5 py-12 sm:px-8 sm:py-16 lg:col-span-5 lg:px-10 xl:px-16">
                    <div className="w-full max-w-2xl">
                        <Motion.div
                            initial={{ opacity: 0, y: 16 }}
                            animate={{ opacity: 1, y: 0 }}
                            transition={{ duration: 0.45 }}
                            className="mb-6 inline-flex items-center gap-3 border-l-4 border-brand-ember bg-black/10 px-4 py-3 sm:mb-8"
                        >
                            <span className="h-2 w-2 rounded-full bg-brand-ember" aria-hidden="true" />
                            <span className="text-sm font-semibold tracking-wide text-brand-lynx">
                                Atendimento 24h via WhatsApp
                            </span>
                        </Motion.div>

                        <Motion.h1
                            initial={{ opacity: 0, y: 20 }}
                            animate={{ opacity: 1, y: 0 }}
                            transition={{ duration: 0.55, delay: 0.1 }}
                            className="mb-6 text-4xl font-bold leading-[1.03] tracking-[-0.035em] text-brand-lynx sm:text-5xl lg:text-6xl xl:text-7xl"
                        >
                            Viagens com
                            <span className="mt-1 block text-brand-lynx">
                                <span className="border-b-[0.12em] border-brand-ember">Conforto Premium</span>
                            </span>
                        </Motion.h1>

                        <Motion.p
                            initial={{ opacity: 0, y: 18 }}
                            animate={{ opacity: 1, y: 0 }}
                            transition={{ duration: 0.55, delay: 0.2 }}
                            className="mb-8 max-w-xl text-base leading-relaxed text-brand-lynx/85 sm:text-lg lg:text-xl"
                        >
                            Fretamento executivo, turismo e eventos com
                            <span className="font-semibold text-brand-lynx"> veículos modernos </span>
                            e atendimento personalizado
                        </Motion.p>

                        <Motion.div
                            initial={{ opacity: 0, y: 18 }}
                            animate={{ opacity: 1, y: 0 }}
                            transition={{ duration: 0.55, delay: 0.3 }}
                            className="mb-9 flex flex-col gap-3 sm:flex-row"
                        >
                            <Button
                                onClick={handleWhatsApp}
                                size="lg"
                                className="h-14 w-full rounded-md bg-brand-ember px-6 text-base font-bold text-black transition-[filter] hover:brightness-95 focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-2 focus-visible:ring-offset-brand-gunship sm:w-auto"
                                startContent={<MessageCircle aria-hidden="true" size={20} />}
                            >
                                Pedir Cotação Grátis
                            </Button>

                            <Button
                                onClick={scrollToFleet}
                                size="lg"
                                variant="bordered"
                                className="h-14 w-full rounded-md border border-brand-blue-grey bg-transparent px-6 text-base font-semibold text-brand-lynx transition-colors hover:border-brand-lynx hover:bg-brand-lynx/10 focus-visible:ring-2 focus-visible:ring-brand-lynx focus-visible:ring-offset-2 focus-visible:ring-offset-brand-gunship sm:w-auto"
                                startContent={<BusFront aria-hidden="true" size={20} />}
                            >
                                Conhecer Frota
                            </Button>
                        </Motion.div>

                        <Motion.ul
                            initial={{ opacity: 0 }}
                            animate={{ opacity: 1 }}
                            transition={{ duration: 0.45, delay: 0.4 }}
                            className="grid gap-3 border-t border-brand-blue-grey/35 pt-6 text-sm text-brand-lynx/85 sm:grid-cols-3 lg:grid-cols-1 xl:grid-cols-3"
                        >
                            {trustItems.map((item) => (
                                <li key={item} className="flex items-center gap-2">
                                    <Check aria-hidden="true" className="shrink-0 text-brand-ember" size={17} strokeWidth={2.5} />
                                    <span>{item}</span>
                                </li>
                            ))}
                        </Motion.ul>
                    </div>
                </div>

                <Motion.figure
                    initial={{ opacity: 0 }}
                    animate={{ opacity: 1 }}
                    transition={{ duration: 0.65, delay: 0.15 }}
                    className="relative min-h-[22rem] overflow-hidden border-t border-brand-blue-grey/30 lg:col-span-7 lg:min-h-0 lg:border-l lg:border-t-0"
                >
                    <img
                        src="/assets/bus-static.webp"
                        alt="Ônibus Schumacher Tur em uma estrada"
                        className="absolute inset-0 h-full w-full object-cover object-center"
                        fetchPriority="high"
                    />
                    <div className="absolute inset-0 bg-gradient-to-t from-brand-gunship/70 via-transparent to-transparent" aria-hidden="true" />
                    <div className="absolute inset-x-0 top-0 h-2 bg-brand-ember" aria-hidden="true" />

                    <div className="absolute bottom-6 right-6 hidden items-center gap-2 text-xs font-semibold uppercase tracking-[0.2em] text-brand-lynx sm:flex">
                        <span>Scroll</span>
                        <ArrowDown aria-hidden="true" size={16} />
                    </div>
                </Motion.figure>
            </div>
        </section>
    )
}
