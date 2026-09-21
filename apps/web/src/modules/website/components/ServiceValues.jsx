import { Armchair, Clock3, MessageCircle, ShieldCheck } from 'lucide-react'

const values = [
    {
        icon: <ShieldCheck aria-hidden="true" className="mb-5 text-brand-ember" size={30} strokeWidth={1.8} />,
        title: 'Segurança',
        description: 'Frota com manutenção rigorosa e motoristas qualificados',
    },
    {
        icon: <Clock3 aria-hidden="true" className="mb-5 text-brand-ember" size={30} strokeWidth={1.8} />,
        title: 'Pontualidade',
        description: 'Compromisso com horários e planejamento de rotas',
    },
    {
        icon: <Armchair aria-hidden="true" className="mb-5 text-brand-ember" size={30} strokeWidth={1.8} />,
        title: 'Conforto',
        description: 'Veículos modernos equipados para sua comodidade',
    },
    {
        icon: <MessageCircle aria-hidden="true" className="mb-5 text-brand-ember" size={30} strokeWidth={1.8} />,
        title: 'Atendimento',
        description: 'Suporte dedicado do planejamento à execução',
    },
]

export default function ServiceValues() {
    return (
        <section aria-labelledby="service-values-title" className="border-b border-brand-blue-grey/35 bg-brand-gunship px-5 py-20 sm:px-8 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-7xl">
                <h2 id="service-values-title" className="mb-12 text-3xl font-bold leading-tight tracking-[-0.025em] text-brand-lynx sm:mb-16 sm:text-4xl lg:text-5xl">
                    Nossos <span className="border-b-4 border-brand-ember">diferenciais</span>
                </h2>

                <ul className="grid border-t border-brand-blue-grey/55 sm:grid-cols-2 lg:grid-cols-4">
                    {values.map(({ icon, title, description }) => (
                        <li
                            key={title}
                            className="border-b border-brand-blue-grey/55 py-8 sm:px-6 sm:first:pl-0 sm:[&:nth-child(odd)]:border-r lg:border-b-0 lg:border-r lg:last:border-r-0 lg:[&:nth-child(odd)]:border-r"
                        >
                            {icon}
                            <h3 className="mb-2 text-xl font-bold text-brand-lynx">{title}</h3>
                            <p className="max-w-xs text-sm leading-6 text-brand-lynx/80 sm:text-base">
                                {description}
                            </p>
                        </li>
                    ))}
                </ul>
            </div>
        </section>
    )
}
