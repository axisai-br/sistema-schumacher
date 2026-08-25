import { Building2, CalendarCheck, Check, Compass, MapPin } from 'lucide-react'

const destinations = [
    {
        icon: <MapPin aria-hidden="true" size={24} strokeWidth={1.8} />,
        title: 'Lençóis Maranhenses',
        description: 'Viagem completa aos Lençóis Maranhenses com roteiro exclusivo. Lagoas cristalinas, dunas infinitas e paisagens de tirar o fôlego.',
        features: ['Roteiro completo', 'Hospedagem inclusa', 'Guia especializado'],
        highlight: true,
        badge: 'MAIS PROCURADO',
    },
    {
        icon: <Compass aria-hidden="true" size={24} strokeWidth={1.8} />,
        title: 'Santa Catarina',
        description: 'Balneário Camboriú, Beto Carrero World e praias incríveis. Diversão garantida para toda a família.',
        features: ['Beto Carrero', 'Balneário Camboriú', 'Praias paradisíacas'],
        highlight: false,
        badge: 'LAZER',
    },
]

const services = [
    {
        icon: <Building2 aria-hidden="true" className="text-brand-lynx" size={30} strokeWidth={1.7} />,
        title: 'Fretamento Empresarial',
        description: 'Transporte regular de colaboradores com rotas personalizadas e pontualidade garantida.',
        features: ['Rotas customizadas', 'Contratos flexíveis'],
    },
    {
        icon: <CalendarCheck aria-hidden="true" className="text-brand-lynx" size={30} strokeWidth={1.7} />,
        title: 'Eventos e Transfers',
        description: 'Transporte para eventos corporativos, casamentos, formaturas e ocasiões especiais.',
        features: ['Logística completa', 'Atendimento VIP'],
    },
]

function FeatureList({ features, compact = false }) {
    return (
        <ul className={compact ? 'mt-5 space-y-2' : 'mt-7 space-y-3'}>
            {features.map((feature) => (
                <li key={feature} className="flex items-center gap-3 text-sm font-medium text-brand-lynx/85">
                    <span className="flex h-5 w-5 shrink-0 items-center justify-center bg-brand-lynx text-brand-gunship" aria-hidden="true">
                        <Check size={13} strokeWidth={3} />
                    </span>
                    {feature}
                </li>
            ))}
        </ul>
    )
}

export default function Services() {
    return (
        <section id="servicos" className="bg-brand-gunship px-5 py-20 text-brand-lynx sm:px-8 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-7xl">
                <header className="mb-12 max-w-3xl sm:mb-16">
                    <h2 className="text-3xl font-bold leading-tight tracking-[-0.025em] text-brand-lynx sm:text-4xl lg:text-5xl">
                        Nossos <span className="border-b-4 border-brand-ember">Destinos</span>
                    </h2>
                    <p className="mt-5 text-base leading-7 text-brand-lynx/75 sm:text-lg">
                        Viagens inesquecíveis com conforto e segurança
                    </p>
                </header>

                <div className="grid gap-6 md:grid-cols-2 lg:gap-8">
                    {destinations.map(({ icon, title, description, features, highlight, badge }) => (
                        <article
                            key={title}
                            className={`flex h-full flex-col border bg-black/10 p-6 sm:p-8 ${highlight ? 'border-brand-ember' : 'border-brand-blue-grey/55'}`}
                        >
                            <div className="mb-8 flex items-start justify-between gap-5">
                                <span className={`flex h-12 w-12 shrink-0 items-center justify-center ${highlight ? 'bg-brand-ember text-black' : 'border border-brand-blue-grey/60 text-brand-lynx'}`}>
                                    {icon}
                                </span>
                                <span className={`px-3 py-2 text-[0.68rem] font-bold tracking-[0.14em] ${highlight ? 'bg-brand-ember text-black' : 'border border-brand-blue-grey/60 text-brand-lynx'}`}>
                                    {badge}
                                </span>
                            </div>

                            <h3 className="mb-4 text-2xl font-bold text-brand-lynx sm:text-3xl">{title}</h3>
                            <p className="text-base leading-7 text-brand-lynx/75">{description}</p>
                            <div className="mt-auto">
                                <FeatureList features={features} />
                            </div>
                        </article>
                    ))}
                </div>

                <div className="mt-16 border-t border-brand-blue-grey/40 pt-12 sm:mt-20 sm:pt-16">
                    <h3 className="mb-8 text-2xl font-bold text-brand-lynx sm:text-3xl">
                        Também oferecemos
                    </h3>

                    <div className="grid gap-x-12 gap-y-10 md:grid-cols-2">
                        {services.map(({ icon, title, description, features }) => (
                            <article key={title} className="grid gap-5 border-l-2 border-brand-blue-grey/60 pl-5 sm:grid-cols-[auto_1fr] sm:pl-6">
                                {icon}
                                <div>
                                    <h4 className="text-xl font-bold text-brand-lynx">{title}</h4>
                                    <p className="mt-3 text-sm leading-6 text-brand-lynx/75 sm:text-base">
                                        {description}
                                    </p>
                                    <FeatureList features={features} compact />
                                </div>
                            </article>
                        ))}
                    </div>
                </div>
            </div>
        </section>
    )
}
