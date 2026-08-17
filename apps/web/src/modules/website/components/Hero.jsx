import { Button } from '@heroui/react'
import { motion as Motion } from 'framer-motion'

const WHATSAPP_NUMBER = '5549999862222'
const WHATSAPP_MESSAGE = 'Olá! Gostaria de informações sobre viagens com a Schumacher Tur'

// Floating particles animation
const FloatingParticle = ({ delay, duration, x, y, size }) => (
    <Motion.div
        className="absolute rounded-full bg-gold-400/20"
        style={{ width: size, height: size, left: x, top: y }}
        animate={{
            y: [0, -30, 0],
            x: [0, 15, 0],
            opacity: [0.2, 0.5, 0.2],
            scale: [1, 1.2, 1],
        }}
        transition={{
            duration,
            delay,
            repeat: Infinity,
            ease: "easeInOut",
        }}
    />
)



export default function Hero() {
    const handleWhatsApp = () => {
        window.open(`https://wa.me/${WHATSAPP_NUMBER}?text=${encodeURIComponent(WHATSAPP_MESSAGE)}`, '_blank')
    }

    const scrollToFleet = () => {
        const behavior = window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'auto' : 'smooth'
        document.getElementById('frota')?.scrollIntoView({ behavior })
    }

    return (
        <section className="relative z-10 flex min-h-[100svh] items-center justify-center overflow-hidden bg-white pt-16 sm:pt-0">
            {/* Gradient background - ends in pure white for smooth transition */}
            <div className="absolute inset-0 bg-gradient-to-b from-gold-50 via-white to-white" />

            {/* Animated mesh gradient overlay */}
            <Motion.div
                className="absolute inset-0 opacity-50"
                style={{
                    background: 'radial-gradient(ellipse at 20% 30%, rgba(212,175,55,0.15) 0%, transparent 50%), radial-gradient(ellipse at 80% 70%, rgba(212,175,55,0.1) 0%, transparent 50%)',
                }}
                animate={{
                    scale: [1, 1.1, 1],
                    opacity: [0.5, 0.7, 0.5],
                }}
                transition={{ duration: 8, repeat: Infinity, ease: "easeInOut" }}
            />

            {/* Floating particles */}
            <FloatingParticle delay={0} duration={4} x="10%" y="20%" size={20} />
            <FloatingParticle delay={1} duration={5} x="85%" y="15%" size={30} />
            <FloatingParticle delay={2} duration={6} x="70%" y="60%" size={25} />
            <FloatingParticle delay={0.5} duration={4.5} x="15%" y="70%" size={35} />
            <FloatingParticle delay={1.5} duration={5.5} x="50%" y="80%" size={20} />
            <FloatingParticle delay={2.5} duration={7} x="30%" y="10%" size={15} />

            {/* Decorative rings */}
            <Motion.div
                className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[600px] h-[600px] border border-gold-200/30 rounded-full"
                animate={{ rotate: 360, scale: [1, 1.05, 1] }}
                transition={{ duration: 30, repeat: Infinity, ease: "linear" }}
            />
            <Motion.div
                className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[800px] h-[800px] border border-gold-200/20 rounded-full"
                animate={{ rotate: -360 }}
                transition={{ duration: 40, repeat: Infinity, ease: "linear" }}
            />

            {/* Glowing orbs */}
            <div className="absolute top-20 right-[15%] w-64 h-64 bg-gradient-to-br from-gold-300/30 to-gold-400/10 rounded-full blur-3xl" />
            <div className="absolute bottom-20 left-[10%] w-80 h-80 bg-gradient-to-tr from-gold-200/25 to-transparent rounded-full blur-3xl" />

            {/* Content */}
            <div className="relative z-10 mx-auto w-full max-w-6xl px-4 py-12 sm:px-6 sm:py-20 lg:px-8">
                <div className="text-center">
                    {/* Badge */}
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.6 }}
                        className="mb-6 inline-flex items-center gap-2 rounded-full border border-gold-200 bg-white/80 px-4 py-2 shadow-soft backdrop-blur-sm sm:mb-8"
                    >
                        <span className="w-2 h-2 bg-green-500 rounded-full animate-pulse" />
                        <span className="text-sm font-medium text-dark-600">Atendimento 24h via WhatsApp</span>
                    </Motion.div>

                    {/* Main heading */}
                    <Motion.h1
                        initial={{ opacity: 0, y: 30 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.8, delay: 0.2 }}
                        className="mb-6 text-4xl font-bold leading-[1.1] text-dark-900 sm:text-6xl lg:text-7xl xl:text-8xl"
                    >
                        Viagens com
                        <br />
                        <span className="relative inline-block">
                            <span className="text-gradient-gold">Conforto Premium</span>
                            <Motion.span
                                className="absolute -bottom-2 left-0 right-0 h-3 bg-gold-400/20 rounded-full -z-10"
                                initial={{ scaleX: 0 }}
                                animate={{ scaleX: 1 }}
                                transition={{ duration: 0.8, delay: 0.8 }}
                            />
                        </span>
                    </Motion.h1>

                    {/* Subtitle */}
                    <Motion.p
                        initial={{ opacity: 0, y: 20 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.8, delay: 0.4 }}
                        className="mx-auto mb-8 max-w-2xl text-lg leading-relaxed text-dark-500 sm:mb-10 sm:text-xl lg:text-2xl"
                    >
                        Fretamento executivo, turismo e eventos com
                        <span className="text-gold-500 font-semibold"> veículos modernos </span>
                        e atendimento personalizado
                    </Motion.p>

                    {/* CTA Buttons */}
                    <Motion.div
                        initial={{ opacity: 0, y: 20 }}
                        animate={{ opacity: 1, y: 0 }}
                        transition={{ duration: 0.8, delay: 0.6 }}
                        className="mb-10 flex flex-col items-center justify-center gap-4 sm:mb-12 sm:flex-row"
                    >
                        <Motion.div className="w-full max-w-sm sm:w-auto" whileHover={{ scale: 1.05 }} whileTap={{ scale: 0.98 }}>
                            <Button
                                onClick={handleWhatsApp}
                                size="lg"
                                className="flex w-full items-center gap-3 rounded-2xl bg-gradient-to-r from-gold-500 via-gold-400 to-gold-500 px-6 py-7 text-lg font-bold text-white shadow-gold-lg transition-shadow duration-300 hover:shadow-2xl sm:w-auto sm:px-10"
                            >
                                <span className="text-2xl">💬</span>
                                Pedir Cotação Grátis
                            </Button>
                        </Motion.div>

                        <Motion.div className="w-full max-w-sm sm:w-auto" whileHover={{ scale: 1.05 }} whileTap={{ scale: 0.98 }}>
                            <Button
                                onClick={scrollToFleet}
                                size="lg"
                                variant="bordered"
                                className="flex w-full items-center gap-3 rounded-2xl border-2 border-gold-300 bg-white/50 px-6 py-7 text-lg font-semibold text-gold-600 backdrop-blur-sm transition-all duration-300 hover:bg-gold-50 sm:w-auto sm:px-10"
                            >
                                <span className="text-2xl">🚌</span>
                                Conhecer Frota
                            </Button>
                        </Motion.div>
                    </Motion.div>

                    {/* Trust badges */}
                    <Motion.div
                        initial={{ opacity: 0 }}
                        animate={{ opacity: 1 }}
                        transition={{ duration: 0.8, delay: 0.8 }}
                        className="flex flex-wrap justify-center gap-x-6 gap-y-2 text-dark-400"
                    >
                        <div className="flex items-center gap-2">
                            <span className="text-gold-500">✓</span>
                            <span className="text-sm">+10 anos no mercado</span>
                        </div>
                        <div className="flex items-center gap-2">
                            <span className="text-gold-500">✓</span>
                            <span className="text-sm">Frota 100% revisada</span>
                        </div>
                        <div className="flex items-center gap-2">
                            <span className="text-gold-500">✓</span>
                            <span className="text-sm">Seguro incluso</span>
                        </div>
                    </Motion.div>
                </div>
            </div>

            {/* Scroll indicator */}
            <Motion.div
                aria-hidden="true"
                initial={{ opacity: 0 }}
                animate={{ opacity: 1 }}
                transition={{ delay: 1.2 }}
                className="absolute bottom-4 left-1/2 z-20 hidden -translate-x-1/2 sm:block xl:bottom-28"
            >
                <Motion.div
                    animate={{ y: [0, 12, 0] }}
                    transition={{ duration: 1.5, repeat: Infinity, ease: "easeInOut" }}
                    className="flex flex-col items-center gap-2"
                >
                    <span className="text-xs text-dark-400 uppercase tracking-widest">Scroll</span>
                    <div className="w-6 h-10 border-2 border-gold-300 rounded-full flex items-start justify-center p-1.5">
                        <Motion.div
                            animate={{ y: [0, 12, 0] }}
                            transition={{ duration: 1.5, repeat: Infinity, ease: "easeInOut" }}
                            className="w-1.5 h-3 bg-gold-400 rounded-full"
                        />
                    </div>
                </Motion.div>
            </Motion.div>

            {/* Smooth transition gradient to video section */}
            <div className="absolute bottom-0 left-0 right-0 h-24 bg-gradient-to-b from-transparent to-white pointer-events-none" />
        </section>
    )
}
