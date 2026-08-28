import { Button } from '@heroui/react'
import { motion as Motion } from 'framer-motion'
import schumacherGraphicNeutral from '../../../assets/brand/graphics/schumacher-graphic-neutral.svg'

const WHATSAPP_NUMBER = '5549999862222'
const WHATSAPP_MESSAGE = 'Olá! Gostaria de solicitar um orçamento de viagem'

export default function FinalCTA() {
    const handleWhatsApp = () => {
        window.open(`https://wa.me/${WHATSAPP_NUMBER}?text=${encodeURIComponent(WHATSAPP_MESSAGE)}`, '_blank')
    }

    return (
        <section className="section-padding overflow-hidden border-y border-brand-blue-grey/40 bg-brand-lynx">
            <div className="container-max grid items-center gap-10 lg:grid-cols-4 lg:gap-14">
                <Motion.div
                    initial={{ opacity: 0, y: 20 }}
                    whileInView={{ opacity: 1, y: 0 }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.6 }}
                    className="text-center lg:col-span-2 lg:text-left"
                >
                    <h2 className="mb-6 text-4xl font-bold text-brand-gunship sm:text-5xl">
                        Pronto para Embarcar?
                    </h2>
                    <p className="mx-auto mb-10 max-w-2xl text-lg leading-relaxed text-brand-gunship/85 sm:text-xl lg:mx-0">
                        Solicite um orçamento agora mesmo e descubra como podemos tornar sua viagem inesquecível.
                    </p>

                    <Motion.div
                        initial={{ opacity: 0, y: 12 }}
                        whileInView={{ opacity: 1, y: 0 }}
                        viewport={{ once: true }}
                        transition={{ duration: 0.4, delay: 0.2 }}
                    >
                        <Button
                            onClick={handleWhatsApp}
                            size="lg"
                            className="h-14 rounded-sm bg-brand-ember px-8 text-base font-bold text-black transition-[filter] hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-gunship focus-visible:ring-offset-2 focus-visible:ring-offset-brand-lynx sm:px-10 sm:text-lg"
                        >
                            Falar no WhatsApp
                        </Button>
                    </Motion.div>
                </Motion.div>

                <Motion.div
                    initial={{ opacity: 0, x: 20 }}
                    whileInView={{ opacity: 1, x: 0 }}
                    viewport={{ once: true }}
                    transition={{ duration: 0.55, delay: 0.1 }}
                    className="flex justify-center lg:col-span-2 lg:justify-end"
                    aria-hidden="true"
                >
                    <img
                        src={schumacherGraphicNeutral}
                        alt=""
                        className="h-auto w-full max-w-md object-contain"
                        loading="lazy"
                        decoding="async"
                    />
                </Motion.div>
            </div>
        </section>
    )
}
