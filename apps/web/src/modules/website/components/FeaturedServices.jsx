import { Building2, CalendarCheck, Check, MessageCircle } from 'lucide-react'
import { contacts, createWhatsAppUrl } from '../data/contacts'
import { featuredServices } from '../data/services'

const WHATSAPP_MESSAGE = 'Olá! Gostaria de solicitar um orçamento de viagem'

const featuredIcons = {
    'fretamento-empresarial': <Building2 size={28} strokeWidth={1.8} />,
    'eventos-transfers': <CalendarCheck size={28} strokeWidth={1.8} />,
}

export default function FeaturedServices() {
    const whatsappUrl = createWhatsAppUrl(contacts.commercial, WHATSAPP_MESSAGE)

    return (
        <section aria-labelledby="featured-services-title" className="bg-brand-lynx px-5 py-20 sm:px-8 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-7xl">
                <header className="mb-12 max-w-3xl sm:mb-16">
                    <h2 id="featured-services-title" className="text-3xl font-bold leading-tight tracking-[-0.025em] text-brand-gunship sm:text-4xl lg:text-5xl">
                        Serviços em <span className="border-b-4 border-brand-ember">destaque</span>
                    </h2>
                </header>

                <div className="grid gap-6 md:grid-cols-2 lg:gap-8">
                    {featuredServices.map(({ id, title, description, features }) => (
                        <article
                            key={id}
                            id={id}
                            className="flex flex-col border border-brand-blue-grey/55 bg-white/35 p-6 sm:p-8 lg:p-10"
                        >
                            <span className="mb-8 flex h-14 w-14 items-center justify-center bg-brand-ember text-black" aria-hidden="true">
                                {featuredIcons[id]}
                            </span>

                            <h3 className="text-2xl font-bold text-brand-gunship sm:text-3xl">{title}</h3>
                            <p className="mt-4 text-base leading-7 text-brand-gunship/80">{description}</p>

                            <ul className="mt-7 space-y-3">
                                {features.map(feature => (
                                    <li key={feature} className="flex items-center gap-3 text-sm font-medium text-brand-gunship/85 sm:text-base">
                                        <span className="flex h-5 w-5 shrink-0 items-center justify-center bg-brand-gunship text-brand-lynx" aria-hidden="true">
                                            <Check size={13} strokeWidth={3} />
                                        </span>
                                        {feature}
                                    </li>
                                ))}
                            </ul>

                            <a
                                href={whatsappUrl}
                                target="_blank"
                                rel="noopener noreferrer"
                                aria-label={`Solicitar orçamento para ${title} pelo WhatsApp`}
                                className="mt-9 inline-flex min-h-12 w-fit items-center justify-center gap-2 rounded-sm bg-brand-ember px-6 py-3 font-bold text-black transition-[filter] hover:brightness-95 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-brand-gunship focus-visible:ring-offset-2 focus-visible:ring-offset-brand-lynx motion-reduce:transition-none"
                            >
                                <MessageCircle aria-hidden="true" size={20} />
                                Solicitar orçamento
                            </a>
                        </article>
                    ))}
                </div>
            </div>
        </section>
    )
}
