import { Armchair, Clock3, MessageCircle, ShieldCheck } from 'lucide-react'

const features = [
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

export default function About() {
    return (
        <section id="sobre" className="border-b border-brand-blue-grey/35 bg-brand-lynx px-5 py-20 sm:px-8 sm:py-24 lg:px-10 lg:py-28">
            <div className="mx-auto max-w-7xl">
                <div className="grid gap-10 border-l-4 border-brand-ember pl-5 sm:pl-7 lg:grid-cols-12 lg:gap-14">
                    <div className="lg:col-span-5">
                        <h2 className="max-w-lg text-3xl font-bold leading-tight tracking-[-0.025em] text-brand-gunship sm:text-4xl lg:text-5xl">
                            Sobre a Schumacher Tur
                        </h2>
                    </div>

                    <div className="max-w-3xl lg:col-span-7 lg:pt-8">
                        <p className="text-base leading-8 text-brand-gunship/85 sm:text-lg">
                            A Schumacher Tur é uma empresa de turismo que atua com uma proposta diferenciada,
                            cujo foco principal não se restringe somente à venda de pacotes de viagem.
                            <span className="font-bold text-brand-gunship"> Focamos na sua experiência de viagem</span>,
                            oferecendo roteiros exclusivos para os Lençóis Maranhenses e destinos em Santa Catarina.
                        </p>
                        <p className="mt-5 text-sm font-medium text-brand-gunship/80">
                            Sede em Fraiburgo/SC • CNPJ: 17.246.217/0001-89
                        </p>
                    </div>
                </div>

                <ul className="mt-14 grid border-t border-brand-blue-grey/55 sm:grid-cols-2 lg:mt-20 lg:grid-cols-4">
                    {features.map(({ icon, title, description }) => (
                        <li
                            key={title}
                            className="border-b border-brand-blue-grey/55 py-8 sm:px-6 sm:first:pl-0 sm:[&:nth-child(odd)]:border-r lg:border-b-0 lg:border-r lg:last:border-r-0 lg:[&:nth-child(odd)]:border-r"
                        >
                            {icon}
                            <h3 className="mb-2 text-xl font-bold text-brand-gunship">{title}</h3>
                            <p className="max-w-xs text-sm leading-6 text-brand-gunship/80 sm:text-base">
                                {description}
                            </p>
                        </li>
                    ))}
                </ul>
            </div>
        </section>
    )
}
